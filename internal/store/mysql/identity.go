package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/bottle-note/mfds-crawler/internal/usecase/identity"
)

// ListMissingIdentitySources pages normalized declarations that have no product identity key yet.
func (s *Store) ListMissingIdentitySources(ctx context.Context, afterID int64, limit int) ([]identity.Source, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, normalized_at,
		       COALESCE(name_search_key_ko, ''), COALESCE(name_search_key_en, ''),
		       abv_percent, age_years, COALESCE(strength_type, '')
		FROM mfds_declarations
		WHERE normalized_at IS NOT NULL AND product_identity_key_sha256 IS NULL AND id > ?
		ORDER BY id
		LIMIT ?
	`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("제품 동일성 키 대상 조회 실패: %w", err)
	}
	defer closeRows(rows, "제품 동일성 키 대상")
	sources := make([]identity.Source, 0, limit)
	for rows.Next() {
		var source identity.Source
		var age sql.NullInt64
		var abv sql.NullFloat64
		value := &source.Identity
		if err := rows.Scan(
			&source.DeclarationID, &source.NormalizedAt,
			&value.NameSearchKeyKO, &value.NameSearchKeyEN, &abv, &age, &value.StrengthType,
		); err != nil {
			return nil, fmt.Errorf("제품 동일성 키 대상 scan 실패: %w", err)
		}
		value.ABVPercent = nullFloatPointer(abv)
		value.AgeYears = nullIntPointer(age)
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("제품 동일성 키 대상 rows 실패: %w", err)
	}
	return sources, nil
}

// SaveProductIdentityKeys writes one batch with a single UPDATE joined to the batch rows, so a remote database costs
// one round trip per batch. Each row is fenced on normalized_at and a still-missing key, so a key a concurrent
// normalization already wrote is never overwritten.
func (s *Store) SaveProductIdentityKeys(ctx context.Context, updates []identity.Update) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	rows := make([]string, 0, len(updates))
	args := make([]any, 0, len(updates)*3)
	for _, update := range updates {
		// 파생 테이블의 값은 문자 집합 변환을 거치므로 키를 hex 문자열로 넘기고 UNHEX로 되돌린다.
		if _, err := nullableSHA256(update.Key); err != nil {
			return 0, err
		}
		rows = append(rows, "ROW(?, ?, ?)")
		args = append(args, update.DeclarationID, update.NormalizedAt, update.Key)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE mfds_declarations AS d
		JOIN (VALUES `+strings.Join(rows, ", ")+`) AS batch (id, normalized_at, identity_key) ON batch.id = d.id
		SET d.product_identity_key_sha256 = UNHEX(batch.identity_key)
		WHERE d.normalized_at = batch.normalized_at AND d.product_identity_key_sha256 IS NULL
	`, args...)
	if err != nil {
		return 0, fmt.Errorf("제품 동일성 키 저장 실패: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("제품 동일성 키 저장 결과 확인 실패: %w", err)
	}
	return int(affected), nil
}

func nullIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullFloatPointer(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	converted := value.Float64
	return &converted
}

var _ identity.Store = (*Store)(nil)
