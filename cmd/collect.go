package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type RunWebListJobFunc func(
	context.Context,
	config.Config,
	weblist.JobCommand,
) (weblist.JobResult, error)

type RunNormalizationFunc func(
	context.Context,
	config.Config,
	normalization.Command,
) (normalization.Summary, error)

func newCollectCommand(
	getConfig func() config.Config,
	run RunWebListJobFunc,
	out io.Writer,
) (*cobra.Command, error) {
	var fromDate string
	var toDate string
	var workers int

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "고정 4개 품목의 날짜 범위를 수집합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := getConfig()
			effectiveWorkers := workers
			if !cmd.Flags().Changed("workers") {
				effectiveWorkers = cfg.Web.ListWorkers
			}
			result, err := executeCollection(cmd.Context(), cfg, run, weblist.JobCommand{
				FromDate: fromDate,
				ToDate:   toDate,
				Workers:  effectiveWorkers,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(
				out,
				"collect 완료: job_id=%d items=%d tasks=%d fetches=%d parsed_rows=%d unique_rcno=%d new_rcno=%d job_status=%s\n",
				result.RunID, len(cfg.Targets), result.TotalPartitions, result.FetchedPages,
				result.ParsedRows, result.UniqueRCNOCount, result.NewRCNOCount, result.Status,
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromDate, "from", "", "수집 시작일 (YYYY-MM-DD)")
	cmd.Flags().StringVar(&toDate, "to", "", "수집 종료일 (YYYY-MM-DD)")
	cmd.Flags().IntVar(&workers, "workers", 0, "동시 DB task worker 수 (기본값: web.list_workers)")
	for _, flag := range []string{"from", "to"} {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			return nil, fmt.Errorf("%s flag 필수 설정 실패: %w", flag, err)
		}
	}
	return cmd, nil
}

func newCollectRecentCommand(
	getConfig func() config.Config,
	run RunWebListJobFunc,
	out io.Writer,
	now func() time.Time,
) *cobra.Command {
	return &cobra.Command{
		Use:   "collect-recent",
		Short: "KST 오늘을 포함한 최근 7일을 중첩 수집합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := getConfig()
			fromDate, toDate, err := recentCollectionRange(now())
			if err != nil {
				return err
			}
			result, err := executeCollection(cmd.Context(), cfg, run, weblist.JobCommand{
				FromDate: fromDate,
				ToDate:   toDate,
				Workers:  cfg.Web.ListWorkers,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(
				out,
				"collect-recent 완료: from=%s to=%s job_id=%d items=%d tasks=%d fetches=%d parsed_rows=%d unique_rcno=%d new_rcno=%d job_status=%s\n",
				fromDate, toDate, result.RunID, len(cfg.Targets), result.TotalPartitions,
				result.FetchedPages, result.ParsedRows, result.UniqueRCNOCount,
				result.NewRCNOCount, result.Status,
			)
			return nil
		},
	}
}

func recentCollectionRange(now time.Time) (string, string, error) {
	location, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return "", "", fmt.Errorf("KST timezone 읽기 실패: %w", err)
	}
	today := now.In(location)
	return today.AddDate(0, 0, -6).Format(time.DateOnly), today.Format(time.DateOnly), nil
}

func executeCollection(
	ctx context.Context,
	cfg config.Config,
	run RunWebListJobFunc,
	command weblist.JobCommand,
) (weblist.JobResult, error) {
	result, err := run(ctx, cfg, command)
	if err != nil {
		return weblist.JobResult{}, err
	}
	if result.Status != weblist.RunStatusCompleted {
		return weblist.JobResult{}, fmt.Errorf("웹 목록 Job 결과가 terminal success가 아닙니다: run_status=%s", result.Status)
	}
	for _, unit := range result.Units {
		if err := validateWebListResult(unit); err != nil {
			return weblist.JobResult{}, err
		}
	}
	return result, nil
}

func validateWebListResult(result weblist.Result) error {
	if result.RunStatus != weblist.RunStatusCompleted ||
		result.PartitionStatus != weblist.PartitionStatusDone {
		return fmt.Errorf(
			"웹 목록 수집 결과가 terminal success가 아닙니다: run_status=%s partition_status=%s",
			result.RunStatus,
			result.PartitionStatus,
		)
	}
	return nil
}
