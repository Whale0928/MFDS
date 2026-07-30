//go:build legacy

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bottle-note/mfds-crawler/internal/usecase/overview"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

const tuiPartitionLimit = 50

func (s *Store) LoadRunDetail(
	ctx context.Context,
	runID uint64,
) (detail overview.RunDetail, err error) {
	if runID == 0 {
		return overview.RunDetail{}, fmt.Errorf("run ID는 양수여야 합니다")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return overview.RunDetail{}, fmt.Errorf("run 상세 transaction 시작 실패: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("run 상세 rollback 실패: %w", rollbackErr))
			detail = overview.RunDetail{}
		}
	}()

	queries := sqlcgen.New(s.db).WithTx(tx)
	run, err := queries.GetCollectorDashboard(ctx, runID)
	if err != nil {
		return overview.RunDetail{}, fmt.Errorf("run 요약 조회 실패: %w", err)
	}
	partitionRows, err := queries.ListPartitionDashboard(ctx, sqlcgen.ListPartitionDashboardParams{
		RunID: runID,
		Limit: tuiPartitionLimit,
	})
	if err != nil {
		return overview.RunDetail{}, fmt.Errorf("run partition 조회 실패: %w", err)
	}
	pageRows, err := queries.ListTUIRunPages(ctx, runID)
	if err != nil {
		return overview.RunDetail{}, fmt.Errorf("run page 조회 실패: %w", err)
	}

	partitions := make([]overview.Partition, 0, len(partitionRows))
	for _, row := range partitionRows {
		partition := overview.Partition{
			ID:              row.ID,
			ItemName:        row.ItemName,
			ProcessDate:     row.ProcessDate,
			Status:          row.Status,
			ParsedRows:      row.ParsedRows,
			UniqueRCNOCount: row.UniqueRcnoCount,
			Attempts:        row.Attempts,
		}
		if row.ExpectedTotal.Valid {
			value := row.ExpectedTotal.Int64
			partition.ExpectedTotal = &value
		}
		if row.ExpectedPages.Valid {
			value := row.ExpectedPages.Int32
			partition.ExpectedPages = &value
		}
		if row.LastError.Valid {
			partition.LastError = row.LastError.String
		}
		partitions = append(partitions, partition)
	}

	pages := make([]overview.Page, 0, len(pageRows))
	for _, row := range pageRows {
		page := overview.Page{
			ID:          row.ID,
			PartitionID: row.PartitionID,
			ItemName:    row.ItemName,
			ItemCode:    row.ItemCode,
			ProcessDate: row.ProcessDate,
			PageNo:      row.PageNo,
			Status:      row.Status,
			WorkerID:    row.WorkerID.String,
			Attempts:    row.Attempts,
		}
		if row.TotalSnapshot.Valid {
			value := row.TotalSnapshot.Int64
			page.TotalSnapshot = &value
		}
		if row.RowCount.Valid {
			value := row.RowCount.Int32
			page.RowCount = &value
		}
		if row.UniqueRcnoCount.Valid {
			value := row.UniqueRcnoCount.Int32
			page.UniqueRCNOCount = &value
		}
		if row.LastError.Valid {
			page.LastError = row.LastError.String
		}
		pages = append(pages, page)
	}

	if err := tx.Commit(); err != nil {
		return overview.RunDetail{}, fmt.Errorf("run 상세 transaction commit 실패: %w", err)
	}
	committed = true

	return overview.RunDetail{
		RunID:               run.ID,
		Status:              run.Status,
		TotalPartitions:     run.TotalPartitions,
		CompletedPartitions: run.CompletedPartitions,
		FetchedRequests:     run.FetchedRequests,
		ParsedRows:          run.ParsedRows,
		DirtyPartitions:     run.DirtyPartitions,
		PendingPages:        run.PendingPages,
		Partitions:          partitions,
		Pages:               pages,
	}, nil
}
