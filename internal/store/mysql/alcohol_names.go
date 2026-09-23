package mysql

import (
	"context"
	"fmt"
)

// matchedAlcoholNamesJoin selects declarations whose alcohol names differ from the live alcohol they are matched to.
// An empty alcohol name leaves that language unchanged.
const matchedAlcoholNamesJoin = `
	mfds_declarations AS d
	JOIN alcohols AS a ON a.id = d.selected_alcohol_id AND a.deleted_at IS NULL`

const matchedAlcoholNamesDiffer = `
	NOT (d.alcohol_name_ko <=> COALESCE(NULLIF(TRIM(a.kor_name), ''), d.alcohol_name_ko))
	OR NOT (d.alcohol_name_en <=> COALESCE(NULLIF(TRIM(a.eng_name), ''), d.alcohol_name_en))`

// ApplyMatchedAlcoholNames overwrites alcohol_name_ko/en, the name the public site shows, with the matched alcohol's
// names for every selection, whether automatic, administrator or inherited. Base product names, search keys and SKU
// display names stay source-based, and nothing in MFDS reads alcohol_name back, so grouping and matching are
// unchanged. A dry run only counts the rows that would change.
func (s *Store) ApplyMatchedAlcoholNames(ctx context.Context, dryRun bool) (int, error) {
	if dryRun {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+matchedAlcoholNamesJoin+` WHERE `+matchedAlcoholNamesDiffer).Scan(&count); err != nil {
			return 0, fmt.Errorf("매칭 알코올 이름 대상 조회 실패: %w", err)
		}
		return count, nil
	}
	result, err := s.db.ExecContext(ctx, `UPDATE `+matchedAlcoholNamesJoin+`
		SET d.alcohol_name_ko = COALESCE(NULLIF(TRIM(a.kor_name), ''), d.alcohol_name_ko),
		    d.alcohol_name_en = COALESCE(NULLIF(TRIM(a.eng_name), ''), d.alcohol_name_en)
		WHERE `+matchedAlcoholNamesDiffer)
	if err != nil {
		return 0, fmt.Errorf("매칭 알코올 이름 저장 실패: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("매칭 알코올 이름 저장 결과 확인 실패: %w", err)
	}
	return int(affected), nil
}
