//go:build legacy

package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func (s *Store) StartWebListJob(
	ctx context.Context,
	params weblist.JobStartParams,
) (record weblist.JobStartRecord, err error) {
	if len(params.Partitions) == 0 {
		return weblist.JobStartRecord{}, fmt.Errorf("웹 목록 Job partition이 필요합니다")
	}
	if params.StartedAt.IsZero() {
		params.StartedAt = time.Now()
	}
	seen := make(map[string]struct{}, len(params.Partitions))
	for _, partition := range params.Partitions {
		key := partition.ItemCode + "\x00" + partition.ProcessDate.Format("2006-01-02")
		if _, duplicated := seen[key]; duplicated {
			return weblist.JobStartRecord{}, fmt.Errorf(
				"웹 목록 Job partition이 중복되었습니다: %s/%s",
				partition.ItemCode, partition.ProcessDate.Format("2006-01-02"),
			)
		}
		seen[key] = struct{}{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return weblist.JobStartRecord{}, fmt.Errorf("웹 목록 Job transaction 시작 실패: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("웹 목록 Job rollback 실패: %w", rollbackErr))
			record = weblist.JobStartRecord{}
		}
	}()

	queries := sqlcgen.New(s.db).WithTx(tx)
	runID, err := queries.CreateCrawlRun(ctx, sqlcgen.CreateCrawlRunParams{
		RunType: string(CrawlRunTypeBackfill), RequestedFromDate: params.FromDate,
		RequestedToDate: params.ToDate, ConfigJson: params.ConfigJSON,
	})
	if err != nil || runID <= 0 {
		return weblist.JobStartRecord{}, fmt.Errorf("crawl run 생성 실패: id=%d: %w", runID, err)
	}
	units := make([]weblist.JobUnit, 0, len(params.Partitions))
	for _, partition := range params.Partitions {
		partitionID, createErr := queries.CreateCrawlPartition(ctx, sqlcgen.CreateCrawlPartitionParams{
			RunID: uint64(runID), ItemCode: partition.ItemCode, ItemName: partition.ItemName,
			ProcessDate: partition.ProcessDate, PageSize: params.PageSize,
		})
		if createErr != nil || partitionID <= 0 {
			return weblist.JobStartRecord{}, fmt.Errorf(
				"crawl partition %s/%s 생성 실패: id=%d: %w",
				partition.ItemName, partition.ProcessDate.Format("2006-01-02"), partitionID, createErr,
			)
		}
		units = append(units, weblist.JobUnit{
			PartitionID: uint64(partitionID),
			Target:      weblist.Target{Name: partition.ItemName, Code: partition.ItemCode},
			ProcessDate: partition.ProcessDate,
		})
	}
	if err := one("crawl run partition 수 설정", func() (int64, error) {
		return queries.SetCrawlRunPartitionTotal(ctx, sqlcgen.SetCrawlRunPartitionTotalParams{
			TotalPartitions: uint32(len(params.Partitions)), ID: uint64(runID),
		})
	}); err != nil {
		return weblist.JobStartRecord{}, err
	}
	metadata, err := json.Marshal(map[string]any{
		"version": 1, "status": string(CrawlRunStatusCreated),
		"partition_count": len(params.Partitions),
	})
	if err != nil {
		return weblist.JobStartRecord{}, fmt.Errorf("JOB_CREATED metadata 생성 실패: %w", err)
	}
	if err := appendEvent(ctx, queries, weblist.EventParams{
		RunID: uint64(runID), Level: "INFO", Phase: "JOB_CREATED",
		Message: "웹 목록 Job 생성", MetadataJSON: metadata,
	}); err != nil {
		return weblist.JobStartRecord{}, err
	}
	if err := one("crawl run LISTING 전이", func() (int64, error) {
		return queries.EnsureCrawlRunListing(ctx, sqlcgen.EnsureCrawlRunListingParams{
			StartedAt: nullTime(params.StartedAt), ID: uint64(runID),
		})
	}); err != nil {
		return weblist.JobStartRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return weblist.JobStartRecord{}, fmt.Errorf("웹 목록 Job transaction commit 실패: %w", err)
	}
	committed = true
	return weblist.JobStartRecord{RunID: uint64(runID), Units: units}, nil
}

func (s *Store) StartWebListBackfill(
	ctx context.Context,
	params weblist.StartParams,
) (record weblist.StartRecord, err error) {
	job, err := s.StartWebListJob(ctx, weblist.JobStartParams{
		FromDate: params.ProcessDate, ToDate: params.ProcessDate, PageSize: params.PageSize,
		ConfigJSON: params.ConfigJSON,
		Partitions: []weblist.JobPartition{{
			ItemName: params.ItemName, ItemCode: params.ItemCode, ProcessDate: params.ProcessDate,
		}},
	})
	if err != nil {
		return weblist.StartRecord{}, err
	}
	return weblist.StartRecord{RunID: job.RunID, PartitionID: job.Units[0].PartitionID}, nil
}
