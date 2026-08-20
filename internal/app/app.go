package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"
	_ "time/tzdata"

	"github.com/bottle-note/mfds-crawler/cmd"
	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdsweb"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	"github.com/bottle-note/mfds-crawler/internal/usecase/importerresolution"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func Run(ctx context.Context, out, errOut io.Writer) error {
	root, err := cmd.NewRootCommand(cmd.Dependencies{
		Loader: config.NewLoader(),
		OpenDatabase: func(cfg config.DatabaseConfig) (cmd.Database, error) {
			return storemysql.Open(cfg)
		},
		RunWebListJob: func(
			ctx context.Context,
			cfg config.Config,
			command weblist.JobCommand,
		) (jobResult weblist.JobResult, runErr error) {
			location, err := time.LoadLocation(cfg.Timezone)
			if err != nil {
				return weblist.JobResult{}, fmt.Errorf("timezone 읽기 실패: %w", err)
			}
			targets := make([]weblist.Target, 0, len(cfg.Targets))
			for _, target := range cfg.Targets {
				targets = append(targets, weblist.Target{Name: target.Name, Code: target.Code})
			}
			store, err := storemysql.Open(cfg.Database)
			if err != nil {
				return weblist.JobResult{}, err
			}
			defer func() {
				runErr = errors.Join(runErr, store.Close())
			}()
			baseURL, err := url.Parse(cfg.Web.BaseURL)
			if err != nil {
				return weblist.JobResult{}, fmt.Errorf("웹 base URL 읽기 실패: %w", err)
			}
			webClient, err := mfdsweb.NewClient(mfdsweb.ClientOptions{BaseURL: baseURL})
			if err != nil {
				return weblist.JobResult{}, err
			}
			service, err := weblist.NewService(store, mfdsweb.NewUsecaseAdapter(webClient), weblist.Options{
				Targets: targets, PageSize: cfg.Web.ListPageSize, QPS: cfg.Web.QPS,
				MaxAttempts: cfg.Retry.MaxAttempts, RetryDelays: cfg.Retry.Delays, Location: location,
				WebBaseURL: cfg.Web.BaseURL,
			})
			if err != nil {
				return weblist.JobResult{}, err
			}
			result, err := service.ExecuteJob(ctx, command)
			if err != nil || result.Status != weblist.RunStatusCompleted {
				return result, err
			}
			companyScraper, err := mfdscompany.NewScraper(mfdscompany.Options{BaseURL: baseURL})
			if err != nil {
				return result, err
			}
			requestDelay := time.Duration(0)
			if cfg.Web.QPS > 0 {
				requestDelay = time.Duration(float64(time.Second) / cfg.Web.QPS)
			}
			importerService, err := importerresolution.NewService(store, companyScraper, importerresolution.Options{
				PageSize: mfdscompany.MaximumPageSize, Delay: requestDelay,
				Industry: "141", State: "ING",
			})
			if err != nil {
				return result, err
			}
			if _, err := importerService.SyncJob(ctx, result.RunID); err != nil {
				return result, err
			}
			return result, nil
		},
		RunNormalization: runNormalization,
		RunMatching:      runMatching,
		Out:              out,
		ErrOut:           errOut,
	})
	if err != nil {
		return err
	}
	return root.ExecuteContext(ctx)
}
