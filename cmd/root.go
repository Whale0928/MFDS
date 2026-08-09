package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
)

type Database interface {
	Ping(context.Context) error
	Migrate(context.Context) error
	Close() error
}

type Dependencies struct {
	Loader             *config.Loader
	OpenDatabase       func(config.DatabaseConfig) (Database, error)
	RunWebListJob      RunWebListJobFunc
	RunCompanyRegistry RunCompanyRegistryCollectionFunc
	RunNormalization   RunNormalizationFunc
	Out                io.Writer
	ErrOut             io.Writer
}

func NewRootCommand(deps Dependencies) (*cobra.Command, error) {
	if deps.Loader == nil ||
		deps.OpenDatabase == nil ||
		deps.RunWebListJob == nil ||
		deps.RunCompanyRegistry == nil ||
		deps.RunNormalization == nil ||
		deps.Out == nil ||
		deps.ErrOut == nil {
		return nil, fmt.Errorf("CLI 의존성이 모두 필요합니다")
	}

	var cfg config.Config

	root := &cobra.Command{
		Use:           "mfds-crawler",
		Short:         "MFDS 수입주류 원장 수집기",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			loaded, err := deps.Loader.Load(config.DefaultConfigFile, config.DefaultEnvFile)
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
	root.CompletionOptions.DisableDefaultCmd = true

	getConfig := func() config.Config { return cfg }
	root.AddCommand(
		newHealthCommand(getConfig, deps.OpenDatabase, deps.Out),
		newMigrateCommand(getConfig, deps.OpenDatabase, deps.Out),
	)
	if err := addCollectorContract(root, getConfig, deps.RunWebListJob); err != nil {
		return nil, err
	}
	root.AddCommand(newNormalizeCommand(getConfig, deps.RunNormalization, deps.Out))
	root.AddCommand(newCollectCompanyRegistryCommand(getConfig, deps.RunCompanyRegistry, deps.Out))
	return root, nil
}

func newHealthCommand(
	getConfig func() config.Config,
	openDatabase func(config.DatabaseConfig) (Database, error),
	out io.Writer,
) *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "설정과 MySQL 연결을 확인합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := getConfig()
			db, err := openDatabase(cfg.Database)
			if err != nil {
				return err
			}
			defer db.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if err := db.Ping(ctx); err != nil {
				return err
			}
			fmt.Fprintf(out, "health 정상: config=ok mysql=ok targets=%d\n", len(cfg.Targets))
			return nil
		},
	}
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
	return migrateCmd
}
