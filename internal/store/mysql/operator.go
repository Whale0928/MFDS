//go:build legacy

package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/operator"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func (s *Store) LoadEvents(
	ctx context.Context,
	runID uint64,
	workerID string,
	afterID uint64,
	limit int32,
) (operator.EventPage, error) {
	queries := sqlcgen.New(s.db)
	rows, err := queries.ListCrawlEventsAfter(ctx, sqlcgen.ListCrawlEventsAfterParams{
		RunID: runID, WorkerID: nullString(workerID), AfterID: afterID, Limit: limit,
	})
	if err != nil {
		return operator.EventPage{}, err
	}
	events := make([]operator.Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, operator.Event{
			ID: row.ID, RunID: row.RunID, PartitionID: nullableUnsigned(row.PartitionID),
			PageID: nullableUnsigned(row.PageID), WorkerID: row.WorkerID.String,
			Level: row.Level, Phase: row.Phase, Message: row.Message,
			MetadataJSON: string(row.MetadataJson), CreatedAt: row.CreatedAt,
		})
	}
	nextAfterID := afterID
	if len(events) > 0 {
		nextAfterID = events[len(events)-1].ID
	}
	return operator.EventPage{
		Events: events, NextAfterID: nextAfterID, HasMore: len(events) == int(limit),
	}, nil
}

func (s *Store) LoadPageItems(ctx context.Context, pageID uint64) ([]operator.Item, error) {
	rows, err := sqlcgen.New(s.db).ListTUIPageItems(ctx, pageID)
	if err != nil {
		return nil, err
	}
	items := make([]operator.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, pageItem(row))
	}
	return items, nil
}

func pageItem(row sqlcgen.ListTUIPageItemsRow) operator.Item {
	return operator.Item{
		ID: row.ID, RunID: row.RunID, PartitionID: row.PartitionID, PageID: row.PageID,
		FetchID: row.FetchID, RowNo: row.RowNo, RCNO: row.Rcno,
		ItemCode: row.QueriedItemCode, ItemName: row.QueriedItemName,
		ProductNameKO: row.ProductNameKo.String, ProductNameEN: row.ProductNameEn.String,
		ImporterName: row.ImporterName.String, CountryName: row.ManufactureCountryName.String,
		ProcessedDate: nullableDatePointer(row.ProcessedDate), ObservedAt: row.ObservedAt,
	}
}

func nullableUnsigned(value sql.NullInt64) uint64 {
	if !value.Valid || value.Int64 < 0 {
		return 0
	}
	return uint64(value.Int64)
}

func nullableDatePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	date := value.Time
	return &date
}
