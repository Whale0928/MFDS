package app

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/cli"
	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdsweb"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func Run(ctx context.Context, out, errOut io.Writer) error {
	root, err := cli.NewRootCommand(cli.Dependencies{
		Loader: config.NewLoader(),
		OpenDatabase: func(cfg config.DatabaseConfig) (cli.Database, error) {
			return storemysql.Open(cfg)
		},
		RunWebListJob: func(
			ctx context.Context,
			cfg config.Config,
			command weblist.JobCommand,
		) (weblist.JobResult, error) {
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
			defer store.Close()
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
				WebBaseURL: cfg.Web.BaseURL, ProxyMode: cfg.Proxy.Mode, ProxyLabel: cfg.Proxy.Label,
			})
			if err != nil {
				return weblist.JobResult{}, err
			}
			return service.ExecuteJob(ctx, command)
		},
		Out:    out,
		ErrOut: errOut,
	})
	if err != nil {
		return err
	}
	return root.ExecuteContext(ctx)
}
