//go:build legacy

package mysql

import (
	"context"
	"fmt"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func (s *Store) LoadJobResult(ctx context.Context, runID uint64) (weblist.JobResult, error) {
	row, err := sqlcgen.New(s.db).GetCrawlJobResult(ctx, runID)
	if err != nil {
		return weblist.JobResult{}, fmt.Errorf("job 결과 집계 조회 실패: %w", err)
	}
	return weblist.JobResult{
		RunID:               row.ID,
		Status:              row.Status,
		TotalPartitions:     row.TotalPartitions,
		CompletedPartitions: row.CompletedPartitions,
		FailedPartitions:    uint32(row.FailedPartitions),
		FetchedPages:        uint32(row.FetchedPages),
		ParsedRows:          row.ParsedRows,
		UniqueRCNOCount:     uint64(row.UniqueRcnoCount),
		NewRCNOCount:        row.NewRcnoCount,
	}, nil
}
