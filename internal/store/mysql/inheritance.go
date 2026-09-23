package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bottle-note/mfds-crawler/internal/usecase/inheritance"
)

const (
	inheritedAlcoholReason = "INHERITED_FROM_DECLARATION"
	inheritedSelectedBy    = "declaration:"
)

// LoadInheritanceRows reads every keyed declaration and every INHERITED row, whose key may have disappeared.
func (s *Store) LoadInheritanceRows(ctx context.Context) ([]inheritance.Row, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.rcno, LOWER(COALESCE(HEX(d.product_identity_key_sha256), '')), d.normalization_status,
		       d.normalization_reasons, COALESCE(d.alcohol_category_en, ''), COALESCE(d.manufacture_country_alpha2, ''),
		       COALESCE(d.alcohol_match_decision, ''), COALESCE(d.selected_alcohol_id, 0),
		       COALESCE(d.selected_distillery_id, 0), COALESCE(d.selected_region_id, 0),
		       COALESCE(d.distillery_match_source, ''), COALESCE(d.region_match_source, ''),
		       COALESCE(d.inherited_from_declaration_id, 0), COALESCE(d.alcohol_candidate_1_id, 0),
		       COALESCE((
		           SELECT s.action
		           FROM mfds_matching_selections AS s
		           WHERE s.declaration_id = d.id AND s.target_type = 'ALCOHOL' AND s.selection_source = 'ADMIN'
		           ORDER BY s.selected_at DESC, s.id DESC
		           LIMIT 1
		       ), '')
		FROM mfds_declarations AS d
		WHERE d.product_identity_key_sha256 IS NOT NULL OR d.alcohol_match_decision = 'INHERITED'
		ORDER BY d.id
	`)
	if err != nil {
		return nil, fmt.Errorf("상속 대상 조회 실패: %w", err)
	}
	defer closeRows(rows, "상속 대상")
	var result []inheritance.Row
	for rows.Next() {
		var row inheritance.Row
		var reasons []byte
		var adminAction string
		if err := rows.Scan(
			&row.DeclarationID, &row.RCNO, &row.IdentityKey, &row.NormalizationStatus,
			&reasons, &row.AlcoholCategoryEN, &row.ManufactureCountryAlpha2,
			&row.Decision, &row.SelectedAlcoholID, &row.SelectedDistilleryID, &row.SelectedRegionID,
			&row.DistillerySource, &row.RegionSource, &row.InheritedFromDeclarationID, &row.TopAlcoholCandidateID,
			&adminAction,
		); err != nil {
			return nil, fmt.Errorf("상속 대상 scan 실패: %w", err)
		}
		if err := json.Unmarshal(reasons, &row.Reasons); err != nil {
			return nil, fmt.Errorf("rcno=%s 정제 사유 JSON 해석 실패: %w", row.RCNO, err)
		}
		row.AdminReleased = adminAction == "REVOKE"
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("상속 대상 rows 실패: %w", err)
	}
	return result, nil
}

func (s *Store) LoadInheritanceAlcohols(ctx context.Context) (map[int64]inheritance.Alcohol, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, deleted_at IS NOT NULL, COALESCE(distillery_id, 0), COALESCE(region_id, 0),
		       COALESCE(eng_name, ''), COALESCE(abv, ''), COALESCE(age, ''), COALESCE(volume, '')
		FROM alcohols
	`)
	if err != nil {
		return nil, fmt.Errorf("상속 기준 알코올 조회 실패: %w", err)
	}
	defer closeRows(rows, "상속 기준 알코올")
	alcohols := map[int64]inheritance.Alcohol{}
	for rows.Next() {
		var alcohol inheritance.Alcohol
		if err := rows.Scan(
			&alcohol.ID, &alcohol.Deleted, &alcohol.DistilleryID, &alcohol.RegionID,
			&alcohol.EngName, &alcohol.ABV, &alcohol.Age, &alcohol.Volume,
		); err != nil {
			return nil, fmt.Errorf("상속 기준 알코올 scan 실패: %w", err)
		}
		alcohols[alcohol.ID] = alcohol
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("상속 기준 알코올 rows 실패: %w", err)
	}
	return alcohols, nil
}

// ApplyInheritanceGroup locks the seeds with FOR SHARE first. An api-server confirmation or release on a seed then waits
// for this transaction, and a seed changed after the plan was read makes the whole group wait for the next run.
func (s *Store) ApplyInheritanceGroup(ctx context.Context, group inheritance.Group) (inheritance.GroupResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return inheritance.GroupResult{}, fmt.Errorf("상속 transaction 시작 실패: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	valid, err := seedsUnchanged(ctx, tx, group)
	if err != nil {
		return inheritance.GroupResult{}, err
	}
	if !valid {
		return inheritance.GroupResult{Skipped: len(group.Fills) + len(group.Updates)}, nil
	}
	result := inheritance.GroupResult{}
	for _, fill := range group.Fills {
		// 관리자 값이 자동 매칭보다 우선하므로 자동 확정 행도 덮어쓴다. 읽은 뒤 관리자가 확정한 행은 건드리지 않는다.
		applied, err := writeInheritedSelection(ctx, tx, fill, `COALESCE(alcohol_match_decision, '') NOT IN ('CANDIDATE', 'MANUAL', 'INHERITED')
			  AND selected_alcohol_id <=> ?`, nullablePositiveID(fill.Current.SelectedAlcoholID))
		if err != nil {
			return inheritance.GroupResult{}, err
		}
		if applied {
			result.Filled++
		} else {
			result.Skipped++
		}
	}
	for _, update := range group.Updates {
		applied, err := writeInheritedSelection(ctx, tx, update, `alcohol_match_decision = 'INHERITED'
			  AND inherited_from_declaration_id <=> ?
			  AND selected_alcohol_id <=> ?`,
			nullablePositiveID(update.Current.InheritedFromDeclarationID), nullablePositiveID(update.Current.SelectedAlcoholID))
		if err != nil {
			return inheritance.GroupResult{}, err
		}
		if applied {
			result.Updated++
		} else {
			result.Skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		return inheritance.GroupResult{}, fmt.Errorf("상속 transaction commit 실패: %w", err)
	}
	return result, nil
}

func seedsUnchanged(ctx context.Context, tx *sql.Tx, group inheritance.Group) (bool, error) {
	key, err := nullableSHA256(group.IdentityKey)
	if err != nil {
		return false, err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(group.Seeds)), ", ")
	args := make([]any, 0, len(group.Seeds)+1)
	for _, seed := range group.Seeds {
		args = append(args, seed.DeclarationID)
	}
	args = append(args, key)
	rows, err := tx.QueryContext(ctx, `
		SELECT d.selected_alcohol_id
		FROM mfds_declarations AS d
		JOIN alcohols AS a ON a.id = d.selected_alcohol_id AND a.deleted_at IS NULL
		WHERE d.id IN (`+placeholders+`)
		  AND d.alcohol_match_decision IN ('CANDIDATE', 'MANUAL')
		  AND d.normalization_status IN ('NORMALIZED', 'PARTIAL')
		  AND d.product_identity_key_sha256 = ?
		FOR SHARE OF d
	`, args...)
	if err != nil {
		return false, fmt.Errorf("상속 시드 확인 실패: %w", err)
	}
	defer closeRows(rows, "상속 시드")
	matched := 0
	for rows.Next() {
		var alcoholID int64
		if err := rows.Scan(&alcoholID); err != nil {
			return false, fmt.Errorf("상속 시드 scan 실패: %w", err)
		}
		// 다른 알코올로 재확정된 시드가 하나라도 있으면 그룹 전체를 다음 실행으로 미룬다.
		if alcoholID != group.Selection.AlcoholID {
			return false, nil
		}
		matched++
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("상속 시드 rows 실패: %w", err)
	}
	return matched == len(group.Seeds), nil
}

// writeInheritedSelection fences on the identity key and a normalized status in addition to the caller's condition,
// so a row renormalized or confirmed by an administrator after the read is left alone.
func writeInheritedSelection(ctx context.Context, tx *sql.Tx, action inheritance.Action, condition string, conditionArgs ...any) (bool, error) {
	key, err := nullableSHA256(action.Current.IdentityKey)
	if err != nil {
		return false, err
	}
	desired := action.Desired
	args := []any{
		desired.AlcoholID, desired.SeedDeclarationID,
		nullablePositiveID(desired.DistilleryID), nullableString(desired.DistillerySource),
		nullablePositiveID(desired.RegionID), nullableString(desired.RegionSource),
		action.Current.DeclarationID, key,
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE mfds_declarations
		SET selected_alcohol_id = ?, alcohol_match_decision = 'INHERITED', inherited_from_declaration_id = ?,
		    selected_distillery_id = ?, distillery_match_source = ?,
		    selected_region_id = ?, region_match_source = ?
		WHERE id = ? AND product_identity_key_sha256 = ?
		  AND normalization_status IN ('NORMALIZED', 'PARTIAL')
		  AND `+condition, append(args, conditionArgs...)...)
	if err != nil {
		return false, fmt.Errorf("rcno=%s 상속 저장 실패: %w", action.Current.RCNO, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rcno=%s 상속 저장 결과 확인 실패: %w", action.Current.RCNO, err)
	}
	if affected == 0 {
		return false, nil
	}
	selectedBy := inheritedSelectedBy + strconv.FormatInt(desired.SeedDeclarationID, 10)
	for _, selection := range []struct {
		targetType string
		targetID   int64
		reason     string
	}{
		{"ALCOHOL", desired.AlcoholID, inheritedAlcoholReason},
		{"DISTILLERY", desired.DistilleryID, desired.DistillerySource},
		{"REGION", desired.RegionID, desired.RegionSource},
	} {
		if selection.targetID <= 0 {
			continue
		}
		if err := insertInheritanceSelection(ctx, tx, action.Current.DeclarationID, selection.targetType, selection.targetID, "SELECT", selection.reason, selectedBy); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ReleaseInheritance restores the decision and selections the latest matcher run recorded, which is what normalization
// would have written had the row never been inherited.
func (s *Store) ReleaseInheritance(ctx context.Context, action inheritance.Action) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("상속 해제 transaction 시작 실패: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current := action.Current
	result, err := tx.ExecContext(ctx, `
		UPDATE mfds_declarations AS d
		LEFT JOIN mfds_alcohol_match_records AS a ON a.run_id = d.matching_run_id AND a.declaration_id = d.id
		LEFT JOIN mfds_reference_match_records AS r ON r.run_id = d.matching_run_id AND r.declaration_id = d.id
		SET d.alcohol_match_decision = a.decision,
		    d.selected_alcohol_id = CASE WHEN a.decision = 'AUTO_SELECTED' THEN a.top_alcohol_id END,
		    d.selected_distillery_id = r.selected_distillery_id, d.distillery_match_source = r.distillery_source,
		    d.selected_region_id = r.selected_region_id, d.region_match_source = r.region_source,
		    d.inherited_from_declaration_id = NULL
		WHERE d.id = ? AND d.alcohol_match_decision = 'INHERITED'
		  AND d.inherited_from_declaration_id <=> ? AND d.selected_alcohol_id <=> ?
	`, current.DeclarationID, nullablePositiveID(current.InheritedFromDeclarationID), nullablePositiveID(current.SelectedAlcoholID))
	if err != nil {
		return false, fmt.Errorf("상속 해제 저장 실패: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("상속 해제 결과 확인 실패: %w", err)
	}
	if affected == 0 {
		return false, nil
	}
	if current.SelectedAlcoholID > 0 {
		selectedBy := inheritedSelectedBy + strconv.FormatInt(current.InheritedFromDeclarationID, 10)
		if err := insertInheritanceSelection(ctx, tx, current.DeclarationID, "ALCOHOL", current.SelectedAlcoholID, "REVOKE", action.Reason, selectedBy); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("상속 해제 transaction commit 실패: %w", err)
	}
	return true, nil
}

// insertInheritanceSelection follows the api-server ADMIN rows: no matcher run, one row per target and action.
// selected_by names the seed declaration, never an administrator ID, so the two sources cannot be confused.
func insertInheritanceSelection(ctx context.Context, tx *sql.Tx, declarationID int64, targetType string, targetID int64, action, reason, selectedBy string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO mfds_matching_selections (
			run_id, declaration_id, target_type, target_id, action,
			selection_source, reason_code, selected_by, selected_at
		) VALUES (NULL, ?, ?, ?, ?, 'INHERITED', ?, ?, NOW(6))
	`, declarationID, targetType, targetID, action, reason, selectedBy)
	if err != nil {
		return fmt.Errorf("상속 선택 이력 저장 실패: %w", err)
	}
	return nil
}

var _ inheritance.Store = (*Store)(nil)
