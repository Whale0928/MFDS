package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	usecase "github.com/bottle-note/mfds-crawler/internal/usecase/matching"
)

type RunMatchingFunc func(context.Context, config.Config, usecase.Command) (usecase.Summary, error)

func newMatchCommand(getConfig func() config.Config, run RunMatchingFunc, out io.Writer) *cobra.Command {
	var limit int
	var rcno string
	var all bool
	var dryRun bool
	var force bool
	command := &cobra.Command{
		Use:   "match",
		Short: "정제 결과의 알코올·증류소·리전 후보를 계산합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("limit") && limit <= 0 {
				return fmt.Errorf("limit은 0보다 커야 합니다")
			}
			if cmd.Flags().Changed("rcno") && strings.TrimSpace(rcno) == "" {
				return fmt.Errorf("rcno는 비어 있을 수 없습니다")
			}
			summary, err := run(cmd.Context(), getConfig(), usecase.Command{
				Limit: limit, RCNO: rcno, All: all, DryRun: dryRun, Force: force,
			})
			fmt.Fprintf(out,
				"match 결과: processed=%d alcohol_matched=%d distillery_matched=%d region_matched=%d no_match=%d remaining=%d version=%s dry_run=%t\n",
				summary.Processed, summary.AlcoholMatched, summary.DistilleryMatched, summary.RegionMatched,
				summary.NoMatch, summary.Remaining, summary.MatchingVersion, dryRun,
			)
			return err
		},
	}
	command.Flags().IntVar(&limit, "limit", 0, "처리할 declaration 수")
	command.Flags().StringVar(&rcno, "rcno", "", "한 RCNO를 강제 재매칭")
	command.Flags().BoolVar(&all, "all", false, "버전이 다른 전체 declaration 처리")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "저장하지 않고 후보 분포만 검증")
	command.Flags().BoolVar(&force, "force", false, "현재 버전 결과도 다시 계산")
	return command
}
