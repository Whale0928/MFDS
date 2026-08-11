package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bottle-note/mfds-crawler/internal/reference"
)

// SyncReferences copies BottleNote reference rows from a read-only source DSN into this MFDS store.
func (s *Store) SyncReferences(ctx context.Context, sourceDSN string) (reference.Result, error) {
	source, err := sql.Open("mysql", sourceDSN)
	if err != nil {
		return reference.Result{}, fmt.Errorf("reference source 연결 초기화 실패: %w", err)
	}
	defer source.Close()
	if err := source.PingContext(ctx); err != nil {
		return reference.Result{}, fmt.Errorf("reference source 연결 실패: %w", err)
	}
	return reference.Sync(ctx, source, s.db)
}
