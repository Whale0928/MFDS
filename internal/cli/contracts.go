package cli

import (
	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
)

func addCollectorContract(
	root *cobra.Command,
	getConfig func() config.Config,
	runWebListJob RunWebListJobFunc,
) error {
	collect, err := newCollectCommand(getConfig, runWebListJob, root.OutOrStdout())
	if err != nil {
		return err
	}
	root.AddCommand(collect)
	return nil
}
