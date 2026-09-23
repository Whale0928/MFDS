package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	domain "github.com/bottle-note/mfds-crawler/internal/matching"
)

type matchingRecordExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// storedSelection is the selection a declaration holds before a new matcher result is written.
type storedSelection struct {
	decision                          domain.DecisionStatus
	alcoholID, distilleryID, regionID int64
}

// lockStoredSelection reads the current selection under a row lock so the write decision and the UPDATE see the same
// state even while api-server confirms or releases the same declaration.
func lockStoredSelection(ctx context.Context, tx *sql.Tx, declarationID int64) (storedSelection, error) {
	var selection storedSelection
	var decision string
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(alcohol_match_decision, ''), COALESCE(selected_alcohol_id, 0),
		       COALESCE(selected_distillery_id, 0), COALESCE(selected_region_id, 0)
		FROM mfds_declarations
		WHERE id = ?
		FOR UPDATE
	`, declarationID).Scan(&decision, &selection.alcoholID, &selection.distilleryID, &selection.regionID)
	if err != nil {
		return storedSelection{}, fmt.Errorf("기존 매칭 선택 조회 실패: %w", err)
	}
	selection.decision = domain.DecisionStatus(decision)
	return selection, nil
}

// assignMatcherDecision writes the matcher decision only when no administrator or inheritance decision owns the row.
// Candidate slots and match records always follow the latest matcher; the preserved decision stays authoritative.
func (s storedSelection) assignMatcherDecision(assign *columnAssignments, result domain.MatchResult) {
	if s.decision.PreservesSelection() {
		return
	}
	assign.set("alcohol_match_decision", nullableString(string(result.AlcoholDecision.Status)))
	assign.set("distillery_match_source", nullableString(result.DistilleryDecision.Source))
	assign.set("region_match_source", nullableString(result.RegionDecision.Source))
	assign.setExpression("selected_alcohol_id = COALESCE(selected_alcohol_id, ?)", nullablePositiveID(result.AlcoholDecision.SelectedID))
	assign.setExpression("selected_distillery_id = COALESCE(selected_distillery_id, ?)", nullablePositiveID(result.DistilleryDecision.SelectedID))
	assign.setExpression("selected_region_id = COALESCE(selected_region_id, ?)", nullablePositiveID(result.RegionDecision.SelectedID))
}

// recordsAutoSelection reports whether the column really holds the automatic choice after the write. A preserved
// decision or an earlier different selection kept by COALESCE must not get an AUTO SELECT history row.
func (s storedSelection) recordsAutoSelection(targetType string, selectedID int64) bool {
	if s.decision.PreservesSelection() {
		return false
	}
	current := map[string]int64{"ALCOHOL": s.alcoholID, "DISTILLERY": s.distilleryID, "REGION": s.regionID}[targetType]
	return current == 0 || current == selectedID
}

func (s *Store) StartMatchingRun(
	ctx context.Context,
	version domain.MatchingVersion,
	normalizationVersion string,
	scope string,
) (int64, error) {
	weights, err := json.Marshal(domain.RuleWeights())
	if err != nil {
		return 0, fmt.Errorf("matching 가중치 JSON 인코딩 실패: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO mfds_matching_runs (
			matcher_version, reference_hash, normalization_version, scope, status,
			weights_json, stats_json, started_at
		) VALUES (?, ?, NULLIF(?, ''), ?, 'RUNNING', ?, JSON_OBJECT(), NOW(6))
	`, version.RuleVersion, version.ReferenceHash, normalizationVersion, scope, weights)
	if err != nil {
		return 0, fmt.Errorf("matching run 시작 실패: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("matching run ID 조회 실패: %w", err)
	}
	return id, nil
}

func (s *Store) FinishMatchingRun(ctx context.Context, runID int64, stats any, runErr error) error {
	if runID <= 0 {
		return nil
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("matching run 통계 JSON 인코딩 실패: %w", err)
	}
	status := "DONE"
	if runErr != nil {
		status = "FAILED"
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE mfds_matching_runs
		SET status = ?, stats_json = ?, finished_at = NOW(6)
		WHERE id = ? AND status = 'RUNNING'
	`, status, encoded, runID)
	if err != nil {
		return fmt.Errorf("matching run 종료 실패: %w", err)
	}
	return nil
}

func saveMatchingRecords(
	ctx context.Context,
	executor matchingRecordExecutor,
	runID int64,
	declarationID int64,
	result domain.MatchResult,
	matchedAt time.Time,
	stored storedSelection,
) error {
	if runID <= 0 {
		return fmt.Errorf("matching run ID가 필요합니다")
	}
	topAlcoholID, topAlcoholScore := topCandidate(result.Alcohols)
	_, err := executor.ExecContext(ctx, `
		INSERT INTO mfds_alcohol_match_records (
			run_id, declaration_id, decision, stop_reason, top_alcohol_id, top_score,
			margin_score, competitive_count, consensus_distillery_id, consensus_region_id, matched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			decision = VALUES(decision), stop_reason = VALUES(stop_reason),
			top_alcohol_id = VALUES(top_alcohol_id), top_score = VALUES(top_score),
			margin_score = VALUES(margin_score), competitive_count = VALUES(competitive_count),
			consensus_distillery_id = VALUES(consensus_distillery_id),
			consensus_region_id = VALUES(consensus_region_id), matched_at = VALUES(matched_at)
	`, runID, declarationID, result.AlcoholDecision.Status, result.AlcoholDecision.StopReason,
		nullablePositiveID(topAlcoholID), nullableScore(topAlcoholScore), nullableScore(result.AlcoholDecision.Margin),
		result.AlcoholDecision.CompetitiveCount, nullablePositiveID(result.AlcoholConsensus.DistilleryID),
		nullablePositiveID(result.AlcoholConsensus.RegionID), matchedAt)
	if err != nil {
		return fmt.Errorf("Stage A 매칭 레코드 저장 실패: %w", err)
	}

	topDistilleryID, topDistilleryScore := topCandidate(result.Distilleries)
	topRegionID, topRegionScore := topCandidate(result.Regions)
	_, err = executor.ExecContext(ctx, `
		INSERT INTO mfds_reference_match_records (
			run_id, declaration_id, triggered_by,
			distillery_decision, distillery_source, top_distillery_id, distillery_score, selected_distillery_id,
			region_decision, region_source, top_region_id, region_score, selected_region_id, matched_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			triggered_by = VALUES(triggered_by),
			distillery_decision = VALUES(distillery_decision), distillery_source = VALUES(distillery_source),
			top_distillery_id = VALUES(top_distillery_id), distillery_score = VALUES(distillery_score),
			selected_distillery_id = VALUES(selected_distillery_id),
			region_decision = VALUES(region_decision), region_source = VALUES(region_source),
			top_region_id = VALUES(top_region_id), region_score = VALUES(region_score),
			selected_region_id = VALUES(selected_region_id), matched_at = VALUES(matched_at)
	`, runID, declarationID, result.AlcoholDecision.StopReason,
		result.DistilleryDecision.Status, result.DistilleryDecision.Source,
		nullablePositiveID(topDistilleryID), nullableScore(topDistilleryScore), nullablePositiveID(result.DistilleryDecision.SelectedID),
		result.RegionDecision.Status, result.RegionDecision.Source,
		nullablePositiveID(topRegionID), nullableScore(topRegionScore), nullablePositiveID(result.RegionDecision.SelectedID), matchedAt)
	if err != nil {
		return fmt.Errorf("Stage B 매칭 레코드 저장 실패: %w", err)
	}

	for _, group := range []struct {
		stage      string
		targetType string
		candidates []domain.Candidate
	}{
		{"A", "ALCOHOL", result.Alcohols},
		{"B", "DISTILLERY", result.Distilleries},
		{"B", "REGION", result.Regions},
	} {
		for index, candidate := range group.candidates {
			candidateID, insertErr := saveMatchingCandidate(ctx, executor, runID, declarationID, group.stage, group.targetType, index+1, candidate)
			if insertErr != nil {
				return insertErr
			}
			for _, evidence := range candidate.Evidence {
				if evidenceErr := saveMatchingEvidence(ctx, executor, candidateID, evidence); evidenceErr != nil {
					return evidenceErr
				}
			}
		}
	}
	for _, selection := range []struct {
		targetType string
		decision   domain.MatchDecision
	}{
		{"ALCOHOL", result.AlcoholDecision},
		{"DISTILLERY", result.DistilleryDecision},
		{"REGION", result.RegionDecision},
	} {
		if selection.decision.Status != domain.DecisionAutoSelected || selection.decision.SelectedID <= 0 ||
			!stored.recordsAutoSelection(selection.targetType, selection.decision.SelectedID) {
			continue
		}
		_, err = executor.ExecContext(ctx, `
			INSERT INTO mfds_matching_selections (
				run_id, declaration_id, target_type, target_id, action,
				selection_source, reason_code, selected_by, selected_at
			) VALUES (?, ?, ?, ?, 'SELECT', 'AUTO', ?, 'matching-v4', ?)
			ON DUPLICATE KEY UPDATE target_id = VALUES(target_id), reason_code = VALUES(reason_code), selected_at = VALUES(selected_at)
		`, runID, declarationID, selection.targetType, selection.decision.SelectedID, selection.decision.StopReason, matchedAt)
		if err != nil {
			return fmt.Errorf("matching 자동 선택 이력 저장 실패: %w", err)
		}
	}
	return nil
}

func saveMatchingCandidate(
	ctx context.Context,
	executor matchingRecordExecutor,
	runID int64,
	declarationID int64,
	stage string,
	targetType string,
	rank int,
	candidate domain.Candidate,
) (int64, error) {
	result, err := executor.ExecContext(ctx, `
		INSERT INTO mfds_matching_candidates (
			run_id, declaration_id, stage, target_type, target_id, rank_no,
			raw_score, evidence_strength, target_name_ko, target_name_en
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))
		ON DUPLICATE KEY UPDATE
			id = LAST_INSERT_ID(id), rank_no = VALUES(rank_no), raw_score = VALUES(raw_score),
			evidence_strength = VALUES(evidence_strength), target_name_ko = VALUES(target_name_ko),
			target_name_en = VALUES(target_name_en)
	`, runID, declarationID, stage, targetType, candidate.ID, rank, candidate.Score,
		candidate.EvidenceStrength, candidate.NameKO, candidate.NameEN)
	if err != nil {
		return 0, fmt.Errorf("matching 후보 저장 실패: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("matching 후보 ID 조회 실패: %w", err)
	}
	return id, nil
}

func saveMatchingEvidence(ctx context.Context, executor matchingRecordExecutor, candidateID int64, evidence domain.Evidence) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO mfds_matching_evidence (
			candidate_id, feature_code, evidence_source, input_value, reference_value,
			rule_code, weight, upstream_target_id
		) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			evidence_source = VALUES(evidence_source), input_value = VALUES(input_value),
			reference_value = VALUES(reference_value), rule_code = VALUES(rule_code),
			weight = VALUES(weight), upstream_target_id = VALUES(upstream_target_id)
	`, candidateID, evidence.Kind, evidence.Source, evidence.InputValue, evidence.ReferenceValue,
		evidence.RuleCode, evidence.Weight, nullablePositiveID(evidence.UpstreamCandidateID))
	if err != nil {
		return fmt.Errorf("matching 후보 근거 저장 실패: %w", err)
	}
	return nil
}

func topCandidate(candidates []domain.Candidate) (int64, float64) {
	if len(candidates) == 0 {
		return 0, 0
	}
	return candidates[0].ID, candidates[0].Score
}

func nullablePositiveID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableScore(value float64) any {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return nil
	}
	return value
}
