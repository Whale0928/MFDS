package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
)

type Database interface {
	Ping(context.Context) error
	Migrate(context.Context) error
	MigrationStatus(context.Context) ([]storemysql.MigrationStatus, error)
	Close() error
}

type Dependencies struct {
	Loader             *config.Loader
	OpenDatabase       func(config.DatabaseConfig) (Database, error)
	RunWebListBackfill RunWebListBackfillFunc
	RunWebListJob      RunWebListJobFunc
	Out                io.Writer
	ErrOut             io.Writer
}

func NewRootCommand(deps Dependencies) (*cobra.Command, error) {
	if deps.Loader == nil ||
		deps.OpenDatabase == nil ||
		deps.RunWebListBackfill == nil ||
		deps.RunWebListJob == nil ||
		deps.Out == nil ||
		deps.ErrOut == nil {
		return nil, fmt.Errorf("CLI 의존성이 모두 필요합니다")
	}

	var configFile string
	var envFile string
	var cfg config.Config

	root := &cobra.Command{
		Use:           "mfds-crawler",
		Short:         "MFDS 수입주류 원장 수집기",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := deps.Loader.Load(configFile, envFile)
			if err != nil {
				return err
			}
			cfg = loaded
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(deps.Out)
	root.SetErr(deps.ErrOut)

	flags := root.PersistentFlags()
	flags.StringVar(
		&configFile,
		"config",
		"secrets/project/mfds/config.yaml",
		"YAML 설정 파일",
	)
	flags.StringVar(
		&envFile,
		"env-file",
		"secrets/project/mfds/.env",
		"환경 변수 파일",
	)
	flags.String("mysql-dsn", "", "MySQL DSN 재정의")
	flags.Float64("api-qps", 0, "API QPS 재정의")
	flags.Float64("web-qps", 0, "웹 QPS 재정의")
	flags.String("proxy-mode", "", "웹 프록시 모드 재정의")

	for key, name := range map[string]string{
		"database.dsn": "mysql-dsn",
		"api.qps":      "api-qps",
		"web.qps":      "web-qps",
		"proxy.mode":   "proxy-mode",
	} {
		if err := deps.Loader.BindFlag(key, flags.Lookup(name)); err != nil {
			return nil, fmt.Errorf("%s flag 연결 실패: %w", name, err)
		}
	}

	getConfig := func() config.Config { return cfg }
	root.AddCommand(
		newConfigCommand(getConfig, deps.Out),
		newDBCommand(getConfig, deps.OpenDatabase, deps.Out),
		newMigrateCommand(getConfig, deps.OpenDatabase, deps.Out),
	)
	if err := addCollectorContracts(root, getConfig, deps.RunWebListBackfill, deps.RunWebListJob); err != nil {
		return nil, err
	}
	return root, nil
}

func newConfigCommand(getConfig func() config.Config, out io.Writer) *cobra.Command {
	configCmd := &cobra.Command{Use: "config", Short: "설정을 관리합니다"}
	configCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "설정 파일과 환경 변수를 검증합니다",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := getConfig()
			fmt.Fprintf(out, "설정 정상: targets=%d api=%s web=%s proxy=%s\n",
				len(cfg.Targets), cfg.API.BaseURL, cfg.Web.BaseURL, cfg.Proxy.Mode)
			return nil
		},
	})
	return configCmd
}

func newDBCommand(
	getConfig func() config.Config,
	openDatabase func(config.DatabaseConfig) (Database, error),
	out io.Writer,
) *cobra.Command {
	dbCmd := &cobra.Command{Use: "db", Short: "데이터베이스 상태를 확인합니다"}
	dbCmd.AddCommand(&cobra.Command{
		Use:   "ping",
		Short: "MySQL 연결을 확인합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDatabase(getConfig().Database)
			if err != nil {
				return err
			}
			defer db.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if err := db.Ping(ctx); err != nil {
				return err
			}
			fmt.Fprintln(out, "MySQL 연결 정상")
			return nil
		},
	})
	return dbCmd
}

func newMigrateCommand(
	getConfig func() config.Config,
	openDatabase func(config.DatabaseConfig) (Database, error),
	out io.Writer,
) *cobra.Command {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "MySQL migration을 적용합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDatabase(getConfig().Database)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Ping(cmd.Context()); err != nil {
				return err
			}
			if err := db.Migrate(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(out, "migration 적용 완료")
			return nil
		},
	}
	migrateCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "MySQL migration 상태를 확인합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := openDatabase(getConfig().Database)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Ping(cmd.Context()); err != nil {
				return err
			}
			statuses, err := db.MigrationStatus(cmd.Context())
			if err != nil {
				return err
			}
			if len(statuses) == 0 {
				fmt.Fprintln(out, "적용된 migration 없음")
				return nil
			}
			for _, status := range statuses {
				fmt.Fprintf(out, "version=%d applied=%t at=%s\n",
					status.Version, status.Applied, status.AppliedAt.Format(time.RFC3339))
			}
			return nil
		},
	})
	return migrateCmd
}
