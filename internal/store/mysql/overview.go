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

func (s *Store) LoadOverview(
	ctx context.Context,
	limit int32,
) (snapshot overview.Snapshot, err error) {
	if limit <= 0 {
		return overview.Snapshot{}, fmt.Errorf("최근 실행 조회 개수는 양수여야 합니다: %d", limit)
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("운영 현황 transaction 시작 실패: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("운영 현황 rollback 실패: %w", rollbackErr))
			snapshot = overview.Snapshot{}
		}
	}()

	queries := sqlcgen.New(s.db).WithTx(tx)
	summary, err := queries.GetTUIOverview(ctx)
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("운영 현황 요약 조회 실패: %w", err)
	}
	rows, err := queries.ListTUIRecentRuns(ctx, limit)
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("최근 수집 실행 조회 실패: %w", err)
	}
	globalPendingDetails, err := queries.GlobalPendingDetails(ctx)
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("전역 상세 대기 수 조회 실패: %w", err)
	}

	runs := make([]overview.Run, 0, len(rows))
	for _, row := range rows {
		run := overview.Run{
			ID:                  row.ID,
			RunType:             row.RunType,
			RequestedFromDate:   row.RequestedFromDate,
			RequestedToDate:     row.RequestedToDate,
			Status:              row.Status,
			TotalPartitions:     row.TotalPartitions,
			CompletedPartitions: row.CompletedPartitions,
			ParsedRows:          row.ParsedRows,
			NewRCNOCount:        row.NewRcnoCount,
			CreatedAt:           row.CreatedAt,
		}
		if row.LastError.Valid {
			run.LastError = row.LastError.String
		}
		if row.FinishedAt.Valid {
			finishedAt := row.FinishedAt.Time
			run.FinishedAt = &finishedAt
		}
		runs = append(runs, run)
	}

	if err := tx.Commit(); err != nil {
		return overview.Snapshot{}, fmt.Errorf("운영 현황 transaction commit 실패: %w", err)
	}
	committed = true

	return overview.Snapshot{
		TotalRuns:            summary.TotalRuns,
		ActiveRuns:           summary.ActiveRuns,
		CompletedRuns:        summary.CompletedRuns,
		FailedRuns:           summary.FailedRuns,
		DirtyPartitions:      summary.DirtyPartitions,
		ListRawRows:          summary.ListRawRows,
		UniqueRCNO:           summary.UniqueRcno,
		DetailQueued:         summary.DetailQueued,
		DetailWorking:        summary.DetailWorking,
		GlobalPendingDetails: globalPendingDetails,
		DetailStored:         summary.DetailStored,
		DetailDead:           summary.DetailDead,
		DetailRawRows:        summary.DetailRawRows,
		RecentRuns:           runs,
	}, nil
}
