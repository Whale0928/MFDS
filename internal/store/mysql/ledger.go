//go:build legacy

package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/ledger"
	"github.com/bottle-note/mfds-crawler/secrets/project/mfds/sqlcgen"
)

func (s *Store) LoadObservations(
	ctx context.Context,
	filters ledger.Filters,
	beforeID uint64,
	limit int32,
) (ledger.Page, error) {
	rows, err := sqlcgen.New(s.db).ListWebListObservations(ctx, sqlcgen.ListWebListObservationsParams{
		ItemCode: optionalString(filters.ItemCode),
		FromDate: optionalTime(filters.FromDate),
		ToDate:   optionalTime(filters.ToDate),
		Rcno:     optionalString(filters.RCNO),
		BeforeID: beforeID,
		Limit:    limit,
	})
	if err != nil {
		return ledger.Page{}, err
	}
	items := make([]ledger.Observation, 0, len(rows))
	for _, row := range rows {
		items = append(items, ledger.Observation{
			ID: row.ID, RunID: row.RunID, PartitionID: row.PartitionID,
			PageID: row.PageID, FetchID: row.FetchID, RCNO: row.Rcno,
			ItemCode: row.QueriedItemCode, ItemName: row.QueriedItemName,
			ProductNameKO: row.ProductNameKo.String, ProductNameEN: row.ProductNameEn.String,
			ImporterName: row.ImporterName.String, CountryName: row.ManufactureCountryName.String,
			ProcessedDate:  nullableDatePointer(row.ProcessedDate),
			SemanticSHA256: append([]byte(nil), row.ListSemanticSha256...),
			ParserVersion:  row.ParserVersion, ParserWarning: row.ParserWarning.String,
			ObservedAt: row.ObservedAt,
		})
	}
	nextBeforeID := beforeID
	if len(items) > 0 {
		nextBeforeID = items[len(items)-1].ID
	}
	return ledger.Page{
		Items: items, NextBeforeID: nextBeforeID, HasMore: len(items) == int(limit),
	}, nil
}

func optionalString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func optionalTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}
