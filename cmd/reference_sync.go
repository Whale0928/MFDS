package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/reference"
)

type RunReferenceSyncFunc func(context.Context, config.Config) (reference.Result, error)

func newReferenceSyncCommand(
	getConfig func() config.Config,
	run RunReferenceSyncFunc,
	out io.Writer,
) *cobra.Command {
	return &cobra.Command{
		Use:   "reference-sync",
		Short: "BottleNote 기준 데이터를 로컬 DB에 동기화합니다",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := run(command.Context(), getConfig())
			if err != nil {
				return err
			}
			fmt.Fprintf(out,
				"reference-sync 완료: alcohols=%d distilleries=%d regions=%d alcohols_hash=%s distilleries_hash=%s regions_hash=%s\n",
				result.Alcohols.Count, result.Distilleries.Count, result.Regions.Count,
				result.Alcohols.Hash, result.Distilleries.Hash, result.Regions.Hash,
			)
			return nil
		},
	}
}
