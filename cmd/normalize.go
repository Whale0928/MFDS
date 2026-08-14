package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

func newNormalizeCommand(
	getConfig func() config.Config,
	run RunNormalizationFunc,
	out io.Writer,
) *cobra.Command {
	var limit int
	var rcno string
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "normalize",
		Short: "RCNO 원장을 비파괴 정제합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("limit") && limit <= 0 {
				return fmt.Errorf("limit은 0보다 커야 합니다")
			}
			if cmd.Flags().Changed("rcno") && strings.TrimSpace(rcno) == "" {
				return fmt.Errorf("rcno는 비어 있을 수 없습니다")
			}
			if cmd.Flags().Changed("limit") && strings.TrimSpace(rcno) != "" {
				return fmt.Errorf("limit과 rcno는 함께 사용할 수 없습니다")
			}
			if force && strings.TrimSpace(rcno) != "" {
				return fmt.Errorf("force와 rcno는 함께 사용할 수 없습니다")
			}
			if force && dryRun {
				return fmt.Errorf("force와 dry-run은 함께 사용할 수 없습니다")
			}

			summary, runErr := run(cmd.Context(), getConfig(), normalization.Command{
				Limit: limit, RCNO: rcno, DryRun: dryRun, Force: force,
			})
			fmt.Fprintf(
				out,
				"normalize 결과: normalized=%d partial=%d review_required=%d unparsed=%d system_failures=%d remaining_pending=%d remaining_stale=%d dry_run=%t\n",
				summary.Processed[normalization.StatusNormalized],
				summary.Processed[normalization.StatusPartial],
				summary.Processed[normalization.StatusReviewRequired],
				summary.Processed[normalization.StatusUnparsed],
				summary.SystemFailures,
				summary.RemainingPending,
				summary.RemainingStale,
				dryRun,
			)
			return runErr
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "처리할 RCNO 수 (기본값: normalization.run_limit)")
	cmd.Flags().StringVar(&rcno, "rcno", "", "상태와 관계없이 강제 재정제할 RCNO")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "DB를 변경하지 않고 정제 결과를 미리 봅니다")
	cmd.Flags().BoolVar(&force, "force", false, "기존 terminal 정제 결과를 한 번 STALE로 되돌려 재정제합니다")
	return cmd
}
