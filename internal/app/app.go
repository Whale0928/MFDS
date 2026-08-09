package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/bottle-note/mfds-crawler/cmd"
	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/source/foodsafetykorea"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdsweb"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
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
			return service.ExecuteJob(ctx, command)
		},
		RunCompanyRegistry: func(
			ctx context.Context,
			cfg config.Config,
		) (summary companyregistry.Summary, runErr error) {
			if err := cfg.FoodSafety.Validate(); err != nil {
				return companyregistry.Summary{}, err
			}
			store, err := storemysql.Open(cfg.Database)
			if err != nil {
				return companyregistry.Summary{}, err
			}
			defer func() {
				runErr = errors.Join(runErr, store.Close())
			}()
			client, err := foodsafetykorea.NewClient(foodsafetykorea.ClientOptions{
				APIKey: cfg.FoodSafety.APIKey,
				HTTPClient: &http.Client{
					Timeout: cfg.FoodSafety.RequestTimeout,
				},
			})
			if err != nil {
				return companyregistry.Summary{}, err
			}
			service, err := companyregistry.NewService(
				store,
				foodsafetykorea.NewUsecaseAdapter(client),
				companyregistry.Options{
					PageSize:       cfg.FoodSafety.PageSize,
					MaxPages:       cfg.FoodSafety.MaxPages,
					MaxRequests:    cfg.FoodSafety.MaxRequests,
					QPS:            cfg.FoodSafety.QPS,
					MaxAttempts:    cfg.Retry.MaxAttempts,
					RetryDelays:    cfg.Retry.Delays,
					MatcherVersion: "importer-license-match-v1",
				},
			)
			if err != nil {
				return companyregistry.Summary{}, err
			}
			return service.Collect(ctx)
		},
		RunNormalization: runNormalization,
		Out:              out,
		ErrOut:           errOut,
	})
	if err != nil {
		return err
	}
	return root.ExecuteContext(ctx)
}
