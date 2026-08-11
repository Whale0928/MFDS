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
	usecase "github.com/bottle-note/mfds-crawler/internal/usecase/matching"
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
	parts := append(append(alcoholParts, distilleryParts...), regionParts...)
	sort.Strings(parts)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	version := domain.DefaultMatchingVersion(hex.EncodeToString(digest[:]))
	return domain.NewReferenceSnapshot(alcohols, distilleries, regions, version)
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

func (s *Store) ListMatchingSources(ctx context.Context, query usecase.Query) ([]usecase.Source, error) {
	statement := `
		SELECT id, rcno, source_item_id, normalization_version, normalized_at,
		       COALESCE(matching_version, ''),
		       COALESCE(base_product_name_ko, ''), COALESCE(base_product_name_en, ''),
		       COALESCE(name_search_key_ko, ''), COALESCE(name_search_key_en, ''),
		       abv_percent, COALESCE(age_raw, ''), age_years, COALESCE(cask_candidate, ''),
		       COALESCE(manufacture_country_name_en, '')
		FROM declarations
		WHERE normalization_status IN ('NORMALIZED', 'PARTIAL', 'REVIEW_REQUIRED', 'UNPARSED')
		  AND normalized_at IS NOT NULL
		  AND (? = '' OR rcno = ?)
		  AND (? OR COALESCE(matching_version, '') <> ?)
		ORDER BY id`
	args := []any{query.RCNO, query.RCNO, query.Force, query.Version}
	if query.Limit > 0 {
		statement += " LIMIT ?"
		args = append(args, query.Limit)
	}
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("matching 대상 조회 실패: %w", err)
	}
	defer closeRows(rows, "matching 대상")
	var sources []usecase.Source
	for rows.Next() {
		var source usecase.Source
		var abv sql.NullFloat64
		var age sql.NullInt64
		if err := rows.Scan(
			&source.DeclarationID, &source.RCNO, &source.SourceItemID,
			&source.NormalizationVersion, &source.NormalizedAt, &source.MatchingVersion,
			&source.BaseProductNameKO, &source.BaseProductNameEN,
			&source.NameSearchKeyKO, &source.NameSearchKeyEN,
			&abv, &source.AgeRaw, &age, &source.CaskCandidate, &source.ManufactureCountryName,
		); err != nil {
			return nil, fmt.Errorf("matching 대상 scan 실패: %w", err)
		}
		if abv.Valid {
			value := abv.Float64
			source.ABVPercent = &value
		}
		if age.Valid {
			value := int(age.Int64)
			source.AgeYears = &value
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("matching 대상 rows 실패: %w", err)
	}
	return sources, nil
}

func (s *Store) SaveMatchingResult(ctx context.Context, completion usecase.Completion) error {
	assign := newColumnAssignments(14)
	setStoredCandidates(assign, "distillery", storedMatchingCandidates(completion.Result.Distilleries))
	setStoredCandidates(assign, "region", storedMatchingCandidates(completion.Result.Regions))
	assign.set("matching_version", completion.Version)
	assign.set("matched_at", completion.MatchedAt)
	result, err := s.db.ExecContext(ctx, `
		UPDATE declarations
		SET `+assign.clause()+`
		WHERE id = ? AND rcno = ? AND source_item_id = ?
		  AND normalization_version = ? AND normalized_at = ?
		  AND COALESCE(matching_version, '') = ?
	`, assign.arguments(
		completion.Source.DeclarationID, completion.Source.RCNO, completion.Source.SourceItemID,
		completion.Source.NormalizationVersion, completion.Source.NormalizedAt, completion.Source.MatchingVersion,
	)...)
	if err != nil {
		return fmt.Errorf("matching 결과 저장 실패: %w", err)
	}
	return requireOne(result, "matching 결과 저장")
}

func (s *Store) MatchingRemaining(ctx context.Context, version string) (int, error) {
	var remaining int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM declarations
		WHERE normalization_status IN ('NORMALIZED', 'PARTIAL', 'REVIEW_REQUIRED', 'UNPARSED')
		  AND normalized_at IS NOT NULL
		  AND COALESCE(matching_version, '') <> ?
	`, version).Scan(&remaining)
	if err != nil {
		return 0, fmt.Errorf("남은 matching 대상 조회 실패: %w", err)
	}
	return remaining, nil
}

type storedCandidate struct {
	id    int64
	score float64
}

func setStoredCandidates(assign *columnAssignments, prefix string, candidates []storedCandidate) {
	for index := 0; index < 3; index++ {
		idColumn := fmt.Sprintf("%s_candidate_%d_id", prefix, index+1)
		scoreColumn := fmt.Sprintf("%s_candidate_%d_score", prefix, index+1)
		if index >= len(candidates) {
			assign.set(idColumn, nil)
			assign.set(scoreColumn, nil)
			continue
		}
		assign.set(idColumn, candidates[index].id)
		assign.set(scoreColumn, candidates[index].score)
	}
}

func storedMatchingCandidates(candidates []domain.Candidate) []storedCandidate {
	stored := make([]storedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		stored = append(stored, storedCandidate{id: candidate.ID, score: normalizedCandidateScore(candidate.Score)})
	}
	return stored
}

func normalizedCandidateScore(score int) float64 {
	if score <= 0 {
		return 0
	}
	if score >= 100 {
		return 1
	}
	return float64(score) / 100
}

func nullableReferenceTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

var _ usecase.Store = (*Store)(nil)
