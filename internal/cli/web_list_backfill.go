package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type RunWebListBackfillFunc func(
	context.Context,
	config.Config,
	weblist.Command,
) (weblist.Result, error)

type RunWebListJobFunc func(
	context.Context,
	config.Config,
	weblist.JobCommand,
) (weblist.JobResult, error)

func newWebListBackfillCommand(
	getConfig func() config.Config,
	run RunWebListBackfillFunc,
	out io.Writer,
) (*cobra.Command, error) {
	var itemName string
	var processDate string

	cmd := &cobra.Command{
		Use:   "list-backfill",
		Short: "한 품목과 처리일의 웹 목록을 수집합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := run(cmd.Context(), getConfig(), weblist.Command{
				ItemName:    itemName,
				ProcessDate: processDate,
			})
			if err != nil {
				return err
			}
			if err := validateWebListResult(result); err != nil {
				return err
			}
			fmt.Fprintf(
				out,
				"web list-backfill 완료: run_id=%d partition_id=%d item=%s item_code=%s process_date=%s total=%d expected_pages=%d fetched_pages=%d parsed_rows=%d unique_rcno=%d new_rcno=%d run_status=%s partition_status=%s\n",
				result.RunID,
				result.PartitionID,
				result.ItemName,
				result.ItemCode,
				result.ProcessDate.Format("2006-01-02"),
				result.ExpectedTotal,
				result.ExpectedPages,
				result.FetchedPages,
				result.ParsedRows,
				result.UniqueRCNOCount,
				result.NewRCNOCount,
				result.RunStatus,
				result.PartitionStatus,
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&itemName, "item", "", "설정에 등록된 대상 품목명")
	cmd.Flags().StringVar(&processDate, "date", "", "신고 처리일 (YYYY-MM-DD)")
	if err := cmd.MarkFlagRequired("item"); err != nil {
		return nil, fmt.Errorf("item flag 필수 설정 실패: %w", err)
	}
	if err := cmd.MarkFlagRequired("date"); err != nil {
		return nil, fmt.Errorf("date flag 필수 설정 실패: %w", err)
	}
	return cmd, nil
}

func newWebListJobCommand(
	getConfig func() config.Config,
	run RunWebListJobFunc,
	out io.Writer,
) (*cobra.Command, error) {
	var fromDate string
	var toDate string
	var workers int

	cmd := &cobra.Command{
		Use:   "list-job",
		Short: "고정 4개 품목과 날짜 범위를 하나의 웹 목록 Job으로 수집합니다",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := getConfig()
			effectiveWorkers := workers
			if !cmd.Flags().Changed("workers") {
				effectiveWorkers = cfg.Web.ListWorkers
			}
			result, err := run(cmd.Context(), cfg, weblist.JobCommand{
				FromDate: fromDate,
				ToDate:   toDate,
				Workers:  effectiveWorkers,
			})
			if err != nil {
				return err
			}
			if result.Status != weblist.RunStatusCompleted {
				return fmt.Errorf("웹 목록 Job 결과가 terminal success가 아닙니다: run_status=%s", result.Status)
			}
			for _, unit := range result.Units {
				if err := validateWebListResult(unit); err != nil {
					return err
				}
			}
			fmt.Fprintf(
				out,
				"web list-job 완료: job_id=%d items=%d tasks=%d fetches=%d parsed_rows=%d unique_rcno=%d new_rcno=%d job_status=%s\n",
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
