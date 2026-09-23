package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	matchingdomain "github.com/bottle-note/mfds-crawler/internal/matching"
	parser "github.com/bottle-note/mfds-crawler/internal/normalization"
	"github.com/bottle-note/mfds-crawler/internal/usecase/identity"
	matchingusecase "github.com/bottle-note/mfds-crawler/internal/usecase/matching"
	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

type selectionState struct {
	decision, distillerySource, regionSource sql.NullString
	alcoholID, distilleryID, regionID        sql.NullInt64
	inheritedFrom                            sql.NullInt64
	candidateOne                             sql.NullInt64
}

func readSelectionState(t *testing.T, store *Store, rcno string) selectionState {
	t.Helper()
	var state selectionState
	if err := store.db.QueryRow(`
		SELECT alcohol_match_decision, distillery_match_source, region_match_source,
		       selected_alcohol_id, selected_distillery_id, selected_region_id,
		       inherited_from_declaration_id, alcohol_candidate_1_id
		FROM mfds_declarations WHERE rcno = ?
	`, rcno).Scan(
		&state.decision, &state.distillerySource, &state.regionSource,
		&state.alcoholID, &state.distilleryID, &state.regionID, &state.inheritedFrom, &state.candidateOne,
	); err != nil {
		t.Fatal(err)
	}
	return state
}

// cleanupMatchingChildren removes history rows that reference test declarations before the fixture deletes them.
func cleanupMatchingChildren(t *testing.T, store *Store, rcnos *[]string, runIDs *[]int64) {
	t.Helper()
	t.Cleanup(func() {
		for _, rcno := range *rcnos {
			for _, statement := range []string{
				`DELETE e FROM mfds_matching_evidence AS e JOIN mfds_matching_candidates AS c ON c.id = e.candidate_id JOIN mfds_declarations AS d ON d.id = c.declaration_id WHERE d.rcno = ?`,
				`DELETE c FROM mfds_matching_candidates AS c JOIN mfds_declarations AS d ON d.id = c.declaration_id WHERE d.rcno = ?`,
				`DELETE s FROM mfds_matching_selections AS s JOIN mfds_declarations AS d ON d.id = s.declaration_id WHERE d.rcno = ?`,
				`DELETE a FROM mfds_alcohol_match_records AS a JOIN mfds_declarations AS d ON d.id = a.declaration_id WHERE d.rcno = ?`,
				`DELETE r FROM mfds_reference_match_records AS r JOIN mfds_declarations AS d ON d.id = r.declaration_id WHERE d.rcno = ?`,
			} {
				if _, err := store.db.Exec(statement, rcno); err != nil {
					t.Errorf("matching child cleanup failed: %v", err)
				}
			}
		}
		for _, runID := range *runIDs {
			if _, err := store.db.Exec(`DELETE FROM mfds_matching_runs WHERE id = ?`, runID); err != nil {
				t.Errorf("matching run cleanup failed: %v", err)
			}
		}
	})
}

func autoSelectedResult(alcoholID, distilleryID, regionID int64) matchingdomain.MatchResult {
	return matchingdomain.MatchResult{
		Alcohols:           []matchingdomain.Candidate{{ID: alcoholID, Score: 12}},
		AlcoholDecision:    matchingdomain.MatchDecision{Status: matchingdomain.DecisionAutoSelected, SelectedID: alcoholID, StopReason: "A_UNIQUE_STRONG"},
		DistilleryDecision: matchingdomain.MatchDecision{Status: matchingdomain.DecisionAutoSelected, Source: "ALCOHOL_PROPAGATED", SelectedID: distilleryID, StopReason: "B_FROM_ALCOHOL"},
		RegionDecision:     matchingdomain.MatchDecision{Status: matchingdomain.DecisionAutoSelected, Source: "ALCOHOL_PROPAGATED", SelectedID: regionID, StopReason: "B_FROM_ALCOHOL"},
	}
}

func completeWithMatch(t *testing.T, store *Store, rcno, owner string, runID int64, match matchingdomain.MatchResult, now time.Time) {
	t.Helper()
	claimed, err := store.Claim(context.Background(), normalizationRequest(rcno, owner))
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %+v, error = %v", claimed, err)
	}
	var candidates []normalization.ReferenceCandidate
	if match.AlcoholDecision.SelectedID > 0 {
		candidates = []normalization.ReferenceCandidate{{ID: match.AlcoholDecision.SelectedID, Score: 12}}
	}
	if err := store.Complete(context.Background(), normalization.Completion{
		Source: claimed[0],
		Result: normalization.Result{Status: normalization.StatusNormalized, Fields: normalization.Fields{
			AlcoholCandidates: candidates,
			MatchingVersion:   "matching-test", MatchingRunID: runID, MatchingResult: match,
		}},
		NormalizationVersion: "normalization-test", NormalizedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func countAutoSelections(t *testing.T, store *Store, rcno string) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM mfds_matching_selections AS s JOIN mfds_declarations AS d ON d.id = s.declaration_id
		WHERE d.rcno = ? AND s.selection_source = 'AUTO'
	`, rcno).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSelectionPreservation_관리자결정은STALE재정제_강제재정제_match후에도유지된다(t *testing.T) {
	// Given
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	var runIDs []int64
	cleanupMatchingChildren(t, store, &fixture.rcnos, &runIDs)
	ctx := context.Background()
	version := matchingdomain.MatchingVersion{RuleVersion: "matching-test", ReferenceHash: "0000000000000000000000000000000000000000000000000000000000000000"}
	runID, err := store.StartMatchingRun(ctx, version, "normalization-test", "NORMALIZATION")
	if err != nil {
		t.Fatal(err)
	}
	runIDs = append(runIDs, runID)
	base := time.Now().UTC().Truncate(time.Microsecond)
	for _, decision := range []string{"CANDIDATE", "MANUAL", "INHERITED"} {
		t.Run(decision, func(t *testing.T) {
			rcno := fmt.Sprintf("PV-%s-%d", decision[:3], time.Now().UnixNano())
			fixture.item(t, rcno, "preserve-"+rcno, base, base)
			if err := store.SyncDeclarations(ctx); err != nil {
				t.Fatal(err)
			}
			completeWithMatch(t, store, rcno, "preserve-initial", runID, matchingdomain.MatchResult{}, base)
			if _, err := store.db.Exec(`
				UPDATE mfds_declarations
				SET alcohol_match_decision = ?, selected_alcohol_id = 900, selected_distillery_id = NULL,
				    distillery_match_source = NULL, selected_region_id = 902, region_match_source = 'MANUAL',
				    inherited_from_declaration_id = CASE WHEN ? = 'INHERITED' THEN 123 END
				WHERE rcno = ?
			`, decision, decision, rcno); err != nil {
				t.Fatal(err)
			}
			want := readSelectionState(t, store, rcno)

			// When: 원장 의미가 바뀌어 STALE이 된 뒤 RCNO 재정제
			fixture.item(t, rcno, "preserve-changed-"+rcno, base.Add(time.Second), base)
			if err := store.SyncDeclarations(ctx); err != nil {
				t.Fatal(err)
			}
			completeWithMatch(t, store, rcno, "preserve-stale", runID, autoSelectedResult(77, 78, 79), base.Add(time.Second))
			afterStale := readSelectionState(t, store, rcno)
			// When: 강제 재정제
			if err := store.ForceRequeue(ctx); err != nil {
				t.Fatal(err)
			}
			completeWithMatch(t, store, rcno, "preserve-force", runID, autoSelectedResult(77, 78, 79), base.Add(2*time.Second))
			afterForce := readSelectionState(t, store, rcno)
			// When: match 백필
			sources, err := store.ListMatchingSources(ctx, matchingusecase.Query{RCNO: rcno, Force: true})
			if err != nil || len(sources) != 1 {
				t.Fatalf("sources = %+v, error = %v", sources, err)
			}
			if err := store.SaveMatchingResult(ctx, matchingusecase.Completion{
				RunID: runID, Source: sources[0], Result: autoSelectedResult(77, 78, 79), Version: "matching-next", MatchedAt: base.Add(3 * time.Second),
			}); err != nil {
				t.Fatal(err)
			}
			afterMatch := readSelectionState(t, store, rcno)

			// Then
			for name, got := range map[string]selectionState{"stale": afterStale, "force": afterForce, "match": afterMatch} {
				if got.decision != want.decision || got.distillerySource != want.distillerySource || got.regionSource != want.regionSource ||
					got.alcoholID != want.alcoholID || got.distilleryID != want.distilleryID || got.regionID != want.regionID ||
					got.inheritedFrom != want.inheritedFrom {
					t.Fatalf("%s changed preserved selection: want=%+v got=%+v", name, want, got)
				}
			}
			if !afterMatch.candidateOne.Valid || afterMatch.candidateOne.Int64 != 77 {
				t.Fatalf("candidate slot was not refreshed: %+v", afterMatch.candidateOne)
			}
			if count := countAutoSelections(t, store, rcno); count != 0 {
				t.Fatalf("AUTO selection history rows = %d, want 0", count)
			}
		})
	}
}

func TestSelectionPreservation_보존되지않는행은자동선택과이력을기록한다(t *testing.T) {
	// Given
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	var runIDs []int64
	cleanupMatchingChildren(t, store, &fixture.rcnos, &runIDs)
	ctx := context.Background()
	version := matchingdomain.MatchingVersion{RuleVersion: "matching-test", ReferenceHash: "1111111111111111111111111111111111111111111111111111111111111111"}
	runID, err := store.StartMatchingRun(ctx, version, "normalization-test", "NORMALIZATION")
	if err != nil {
		t.Fatal(err)
	}
	runIDs = append(runIDs, runID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	fresh := fmt.Sprintf("PV-AUTO-%d", time.Now().UnixNano())
	kept := fmt.Sprintf("PV-KEPT-%d", time.Now().UnixNano())
	fixture.item(t, fresh, "auto-"+fresh, now, now)
	fixture.item(t, kept, "auto-"+kept, now, now)
	if err := store.SyncDeclarations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE mfds_declarations SET alcohol_match_decision = 'REVIEW', selected_alcohol_id = 555 WHERE rcno = ?`, kept); err != nil {
		t.Fatal(err)
	}

	// When
	completeWithMatch(t, store, fresh, "auto-fresh", runID, autoSelectedResult(77, 78, 79), now)
	completeWithMatch(t, store, kept, "auto-kept", runID, autoSelectedResult(77, 78, 79), now)

	// Then
	freshState := readSelectionState(t, store, fresh)
	if freshState.decision.String != "AUTO_SELECTED" || freshState.alcoholID.Int64 != 77 || countAutoSelections(t, store, fresh) != 3 {
		t.Fatalf("fresh state = %+v, auto rows = %d", freshState, countAutoSelections(t, store, fresh))
	}
	keptState := readSelectionState(t, store, kept)
	if keptState.alcoholID.Int64 != 555 {
		t.Fatalf("COALESCE did not keep existing selection: %+v", keptState)
	}
	var alcoholRows int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM mfds_matching_selections AS s JOIN mfds_declarations AS d ON d.id = s.declaration_id
		WHERE d.rcno = ? AND s.target_type = 'ALCOHOL'
	`, kept).Scan(&alcoholRows); err != nil {
		t.Fatal(err)
	}
	if alcoholRows != 0 {
		t.Fatalf("AUTO ALCOHOL history written for a kept selection: %d", alcoholRows)
	}
}

func TestIdentityFill_저장된정제값으로키를채우고재실행은바꾸지않는다(t *testing.T) {
	// Given
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	rcno := fmt.Sprintf("IDENTITY-%d", time.Now().UnixNano())
	fixture.item(t, rcno, "identity-"+rcno, now, now)
	if err := store.SyncDeclarations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		UPDATE mfds_declarations
		SET normalization_status = 'NORMALIZED', normalized_at = ?, name_search_key_ko = '조니워커 블랙',
		    name_search_key_en = 'johnnie walker black label', age_years = 12, abv_percent = 40.000,
		    product_identity_key_sha256 = NULL
		WHERE rcno = ?
	`, now, rcno); err != nil {
		t.Fatal(err)
	}
	age, abv := 12, 40.0
	want := parser.ProductIdentityKey(parser.ProductIdentity{
		NameSearchKeyKO: "조니워커 블랙", NameSearchKeyEN: "johnnie walker black label", AgeYears: &age, ABVPercent: &abv,
	})
	service, err := identity.NewService(store)
	if err != nil {
		t.Fatal(err)
	}
	readKey := func() string {
		var key sql.NullString
		if err := store.db.QueryRow(`SELECT LOWER(HEX(product_identity_key_sha256)) FROM mfds_declarations WHERE rcno = ?`, rcno).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key.String
	}

	// When
	dryRun, dryErr := service.FillMissing(ctx, true)
	afterDryRun := readKey()
	applied, applyErr := service.FillMissing(ctx, false)
	afterApply := readKey()
	again, againErr := service.FillMissing(ctx, false)

	// Then
	if dryErr != nil || applyErr != nil || againErr != nil {
		t.Fatalf("errors: dry=%v apply=%v again=%v", dryErr, applyErr, againErr)
	}
	if dryRun.Filled < 1 || afterDryRun != "" {
		t.Fatalf("dry run = %+v, key after dry run = %q", dryRun, afterDryRun)
	}
	if applied.Filled < 1 || afterApply != want {
		t.Fatalf("applied = %+v, key = %q, want %q", applied, afterApply, want)
	}
	if again.Filled != 0 {
		t.Fatalf("second run changed rows: %+v", again)
	}
}

func TestIdentityFill_읽은뒤재정제된행은덮어쓰지않는다(t *testing.T) {
	// Given
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	rcno := fmt.Sprintf("ID-FENCE-%d", time.Now().UnixNano())
	fixture.item(t, rcno, "identity-fence-"+rcno, now, now)
	if err := store.SyncDeclarations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE mfds_declarations SET normalization_status = 'NORMALIZED', normalized_at = ? WHERE rcno = ?`, now.Add(time.Second), rcno); err != nil {
		t.Fatal(err)
	}
	var declarationID int64
	if err := store.db.QueryRow(`SELECT id FROM mfds_declarations WHERE rcno = ?`, rcno).Scan(&declarationID); err != nil {
		t.Fatal(err)
	}

	// When: 읽은 시점의 normalized_at이 현재 값과 다르다
	applied, err := store.SaveProductIdentityKeys(ctx, []identity.Update{{
		DeclarationID: declarationID, NormalizedAt: now, Key: "aa" + fmt.Sprintf("%062d", 0),
	}})

	// Then
	if err != nil || applied != 0 {
		t.Fatalf("applied = %d, error = %v", applied, err)
	}
}

func TestComplete_관리자매칭재사용결과는자동선택을덮어쓰고매처기록없이상속이력만남긴다(t *testing.T) {
	// Given: 이전에 다른 알코올로 자동 확정된 행과 관리자 확정 행
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	var runIDs []int64
	cleanupMatchingChildren(t, store, &fixture.rcnos, &runIDs)
	ctx := context.Background()
	version := matchingdomain.MatchingVersion{RuleVersion: "matching-test", ReferenceHash: "0000000000000000000000000000000000000000000000000000000000000000"}
	runID, err := store.StartMatchingRun(ctx, version, "normalization-test", "NORMALIZATION")
	if err != nil {
		t.Fatal(err)
	}
	runIDs = append(runIDs, runID)
	base := time.Now().UTC().Truncate(time.Microsecond)
	auto := fmt.Sprintf("RU-A-%d", time.Now().UnixNano())
	admin := fmt.Sprintf("RU-M-%d", time.Now().UnixNano())
	for _, rcno := range []string{auto, admin} {
		fixture.item(t, rcno, "reuse-"+rcno, base, base)
	}
	if err := store.SyncDeclarations(ctx); err != nil {
		t.Fatal(err)
	}
	completeWithMatch(t, store, auto, "reuse-initial", runID, autoSelectedResult(77, 78, 79), base)
	completeWithMatch(t, store, admin, "reuse-initial", runID, matchingdomain.MatchResult{}, base)
	if _, err := store.db.Exec(`UPDATE mfds_declarations SET alcohol_match_decision = 'MANUAL', selected_alcohol_id = 900 WHERE rcno = ?`, admin); err != nil {
		t.Fatal(err)
	}
	reused := matchingdomain.MatchResult{
		AlcoholDecision:    matchingdomain.MatchDecision{Status: matchingdomain.DecisionInherited, SelectedID: 5582},
		DistilleryDecision: matchingdomain.MatchDecision{SelectedID: 7, Source: "ALCOHOL_PROPAGATED"},
		RegionDecision:     matchingdomain.MatchDecision{SelectedID: 8, Source: "ALCOHOL_PROPAGATED"},
	}
	completeReused := func(rcno string, at time.Time) {
		if _, err := store.db.Exec(`UPDATE mfds_declarations SET normalization_status = 'STALE' WHERE rcno = ?`, rcno); err != nil {
			t.Fatal(err)
		}
		claimed, err := store.Claim(ctx, normalizationRequest(rcno, "reuse-next"))
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim = %+v, error = %v", claimed, err)
		}
		if err := store.Complete(ctx, normalization.Completion{
			Source: claimed[0],
			Result: normalization.Result{Status: normalization.StatusNormalized, Fields: normalization.Fields{
				MatchingVersion: "matching-test", MatchingRunID: runID, MatchingResult: reused, InheritedFromDeclarationID: 4242,
			}},
			NormalizationVersion: "normalization-test", NormalizedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	countRows := func(query, rcno string) int {
		var count int
		if err := store.db.QueryRow(query, rcno).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	recordsBefore := countRows(`SELECT COUNT(*) FROM mfds_alcohol_match_records AS a JOIN mfds_declarations AS d ON d.id = a.declaration_id WHERE d.rcno = ?`, auto)

	// When
	completeReused(auto, base.Add(time.Second))
	completeReused(admin, base.Add(time.Second))

	// Then
	got := readSelectionState(t, store, auto)
	if got.decision.String != "INHERITED" || got.alcoholID.Int64 != 5582 || got.distilleryID.Int64 != 7 || got.regionID.Int64 != 8 ||
		got.inheritedFrom.Int64 != 4242 || got.distillerySource.String != "ALCOHOL_PROPAGATED" {
		t.Fatalf("reused row = %+v", got)
	}
	if after := countRows(`SELECT COUNT(*) FROM mfds_alcohol_match_records AS a JOIN mfds_declarations AS d ON d.id = a.declaration_id WHERE d.rcno = ?`, auto); after != recordsBefore {
		t.Fatalf("matcher records written for a reused match: before=%d after=%d", recordsBefore, after)
	}
	if inherited := countRows(`SELECT COUNT(*) FROM mfds_matching_selections AS s JOIN mfds_declarations AS d ON d.id = s.declaration_id WHERE d.rcno = ? AND s.selection_source = 'INHERITED' AND s.selected_by = 'declaration:4242'`, auto); inherited != 3 {
		t.Fatalf("INHERITED history rows = %d, want 3", inherited)
	}
	if kept := readSelectionState(t, store, admin); kept.decision.String != "MANUAL" || kept.alcoholID.Int64 != 900 {
		t.Fatalf("administrator row changed: %+v", kept)
	}
}
