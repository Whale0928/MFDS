package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	domain "github.com/bottle-note/mfds-crawler/internal/matching"
	"github.com/spf13/cast"
)

// LoadMatchingSnapshot reads one immutable in-process reference snapshot.
func (s *Store) LoadMatchingSnapshot(ctx context.Context) (*domain.ReferenceSnapshot, error) {
	alcohols, alcoholParts, err := s.loadAlcoholReferences(ctx)
	if err != nil {
		return nil, err
	}
	distilleries, distilleryParts, err := s.loadDistilleryReferences(ctx)
	if err != nil {
		return nil, err
	}
	regions, regionParts, err := s.loadRegionReferences(ctx)
	if err != nil {
		return nil, err
	}
	if len(alcohols) == 0 || len(distilleries) == 0 || len(regions) == 0 {
		return nil, fmt.Errorf("matching 기준 데이터가 비어 있습니다: alcohols=%d distilleries=%d regions=%d", len(alcohols), len(distilleries), len(regions))
	}
	aliases, aliasParts, err := s.loadReferenceAliases(ctx)
	if err != nil {
		return nil, err
	}
	parts := append(append(append(alcoholParts, distilleryParts...), regionParts...), aliasParts...)
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	version := domain.DefaultMatchingVersion(hex.EncodeToString(digest[:]))
	return domain.NewReferenceSnapshotWithAliases(alcohols, distilleries, regions, aliases, version)
}

func (s *Store) loadAlcoholReferences(ctx context.Context) ([]domain.AlcoholReference, []string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kor_name, eng_name, COALESCE(abv, ''), type,
		       CONCAT_WS('|', kor_category, eng_category, category_group),
		       COALESCE(region_id, 0), COALESCE(distillery_id, 0), COALESCE(age, ''),
		       COALESCE(cask, ''), COALESCE(volume, ''), deleted_at
		FROM alcohols
		ORDER BY id
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("alcohol matching 기준 조회 실패: %w", err)
	}
	defer closeRows(rows, "alcohol matching 기준")
	var references []domain.AlcoholReference
	var parts []string
	for rows.Next() {
		var reference domain.AlcoholReference
		var abvRaw string
		var deletedAt sql.NullTime
		if err := rows.Scan(
			&reference.ID, &reference.KorName, &reference.EngName, &abvRaw,
			&reference.Type, &reference.Category, &reference.RegionID, &reference.DistilleryID,
			&reference.Age, &reference.Cask, &reference.Volume, &deletedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("alcohol matching 기준 scan 실패: %w", err)
		}
		if parsed, parseErr := cast.ToFloat64E(strings.TrimSuffix(strings.TrimSpace(abvRaw), "%")); parseErr == nil {
			reference.ABVPercent = &parsed
		}
		if deletedAt.Valid {
			value := deletedAt.Time
			reference.DeletedAt = &value
		}
		references = append(references, reference)
		parts = append(parts, fmt.Sprintf("a|%d|%s|%s|%s|%s|%s|%d|%d|%s|%s|%s|%s",
			reference.ID, reference.KorName, reference.EngName, abvRaw, reference.Type,
			reference.Category, reference.RegionID, reference.DistilleryID, reference.Age,
			reference.Cask, reference.Volume, nullableReferenceTime(reference.DeletedAt)))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("alcohol matching 기준 rows 실패: %w", err)
	}
	return references, parts, nil
}

func (s *Store) loadDistilleryReferences(ctx context.Context) ([]domain.DistilleryReference, []string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kor_name, eng_name FROM distilleries ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("distillery matching 기준 조회 실패: %w", err)
	}
	defer closeRows(rows, "distillery matching 기준")
	var references []domain.DistilleryReference
	var parts []string
	for rows.Next() {
		var reference domain.DistilleryReference
		if err := rows.Scan(&reference.ID, &reference.KorName, &reference.EngName); err != nil {
			return nil, nil, fmt.Errorf("distillery matching 기준 scan 실패: %w", err)
		}
		references = append(references, reference)
		parts = append(parts, fmt.Sprintf("d|%d|%s|%s", reference.ID, reference.KorName, reference.EngName))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("distillery matching 기준 rows 실패: %w", err)
	}
	return references, parts, nil
}

func (s *Store) loadRegionReferences(ctx context.Context) ([]domain.RegionReference, []string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kor_name, eng_name, COALESCE(parent_id, 0) FROM regions ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("region matching 기준 조회 실패: %w", err)
	}
	defer closeRows(rows, "region matching 기준")
	var references []domain.RegionReference
	var parts []string
	for rows.Next() {
		var reference domain.RegionReference
		if err := rows.Scan(&reference.ID, &reference.KorName, &reference.EngName, &reference.ParentID); err != nil {
			return nil, nil, fmt.Errorf("region matching 기준 scan 실패: %w", err)
		}
		references = append(references, reference)
		parts = append(parts, fmt.Sprintf("r|%d|%s|%s|%d", reference.ID, reference.KorName, reference.EngName, reference.ParentID))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("region matching 기준 rows 실패: %w", err)
	}
	return references, parts, nil
}

func (s *Store) loadReferenceAliases(ctx context.Context) ([]domain.ReferenceAlias, []string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT entity_type, entity_id, alias_norm, language, source
		FROM mfds_reference_aliases
		ORDER BY entity_type, entity_id, alias_norm, language
	`)
	if err != nil {
		return nil, nil, fmt.Errorf("matching 별칭 조회 실패: %w", err)
	}
	defer closeRows(rows, "matching 별칭")
	var aliases []domain.ReferenceAlias
	var parts []string
	for rows.Next() {
		var alias domain.ReferenceAlias
		if err := rows.Scan(&alias.EntityType, &alias.EntityID, &alias.Alias, &alias.Language, &alias.Source); err != nil {
			return nil, nil, fmt.Errorf("matching 별칭 scan 실패: %w", err)
		}
		aliases = append(aliases, alias)
		parts = append(parts, fmt.Sprintf("x|%s|%d|%s|%s|%s", alias.EntityType, alias.EntityID, alias.Alias, alias.Language, alias.Source))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("matching 별칭 rows 실패: %w", err)
	}
	return aliases, parts, nil
}

func nullableReferenceTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
