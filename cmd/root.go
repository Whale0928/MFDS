package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
)

type Dependencies struct {
	Loader           *config.Loader
	RunWebListJob    RunWebListJobFunc
	RunNormalization RunNormalizationFunc
	Out              io.Writer
	ErrOut           io.Writer
}

func NewRootCommand(deps Dependencies) (*cobra.Command, error) {
	if deps.Loader == nil ||
		deps.RunWebListJob == nil ||
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
		newCollectRecentCommand(getConfig, deps.RunWebListJob, deps.Out, time.Now),
		newNormalizeCommand(getConfig, deps.RunNormalization, deps.Out),
	)
	return root, nil
}
