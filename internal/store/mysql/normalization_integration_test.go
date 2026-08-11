package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

func normalizationStore(t *testing.T) *Store {
	t.Helper()
	store := integrationStore(t)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

type normalizationFixture struct {
	store  *Store
	rcnos  []string
	jobIDs []uint64
}

func newNormalizationFixture(t *testing.T, store *Store) *normalizationFixture {
	t.Helper()
	fixture := &normalizationFixture{store: store}
	t.Cleanup(func() {
		for _, rcno := range fixture.rcnos {
			if _, err := store.db.Exec("DELETE FROM declarations WHERE rcno = ?", rcno); err != nil {
				t.Errorf("declaration cleanup failed: %v", err)
			}
		}
		for _, jobID := range fixture.jobIDs {
			if _, err := store.db.Exec("DELETE FROM jobs WHERE id = ?", jobID); err != nil {
				t.Errorf("job cleanup failed: %v", err)
			}
		}
	})
	return fixture
}

func (fixture *normalizationFixture) item(
	t *testing.T,
	rcno string,
	semanticSeed string,
	observedAt time.Time,
	processedDate time.Time,
) int64 {
	t.Helper()
	ctx := context.Background()
	result, err := fixture.store.db.ExecContext(ctx, `
		INSERT INTO jobs (job_type, requested_from_date, requested_to_date, status, config_json)
		VALUES ('TEST', ?, ?, 'DONE', JSON_OBJECT())
	`, processedDate, processedDate)
	if err != nil {
		t.Fatal(err)
	}
	jobID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	fixture.jobIDs = append(fixture.jobIDs, uint64(jobID))
	result, err = fixture.store.db.ExecContext(ctx, `
		INSERT INTO tasks (job_id, process_date, status) VALUES (?, ?, 'DONE')
	`, jobID, processedDate)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	rawHash := sha256.Sum256([]byte("raw-" + semanticSeed))
	semanticHash := sha256.Sum256([]byte(semanticSeed))
	result, err = fixture.store.db.ExecContext(ctx, `
		INSERT INTO fetches (
			job_id, task_id, item_code, item_name, page_no, request_key_sha256,
			request_method, request_url, request_query_json, attempt_no, started_at, status
		) VALUES (?, ?, 'TEST', '테스트 품목', 1, ?, 'GET', 'https://example.test', JSON_OBJECT(), 1, ?, 'PARSED')
	`, jobID, taskID, rawHash[:], observedAt)
	if err != nil {
		t.Fatal(err)
	}
	fetchID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	result, err = fixture.store.db.ExecContext(ctx, `
		INSERT INTO items (
			job_id, task_id, fetch_id, row_no, rcno, queried_item_code, queried_item_name,
			product_name_ko, product_name_en, item_name, importer_name, processed_date,
			canonical_values_json, raw_row_html, raw_row_sha256, semantic_sha256, parser_version, observed_at
		) VALUES (?, ?, ?, 1, ?, 'TEST', '테스트 품목', ?, ?, '위스키', '테스트 수입사', ?, JSON_OBJECT(), ?, ?, ?, 'test', ?)
	`, jobID, taskID, fetchID, rcno, "한글 "+semanticSeed, "English "+semanticSeed,
		processedDate, "synthetic-row-"+semanticSeed, rawHash[:], semanticHash[:], observedAt)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	fixture.rcnos = append(fixture.rcnos, rcno)
	return itemID
}

func normalizationRequest(rcno, owner string) normalization.ClaimRequest {
	return normalization.ClaimRequest{
		Limit: 1, RCNO: rcno, Owner: owner, LeaseDuration: time.Minute,
		MaxAttempts: 3, RetryDelay: time.Second,
	}
}

func TestNormalizationStore_최신관찰동기화와의미드리프트를반영한다(t *testing.T) {
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	rcno := fmt.Sprintf("NORM-SYNC-%d", time.Now().UnixNano())
	base := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	firstID := fixture.item(t, rcno, "first", base, base)
	latestSameID := fixture.item(t, rcno, "first", base, base.AddDate(0, 0, 1))

	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	var sourceID int64
	var status string
	if err := store.db.QueryRow(`SELECT source_item_id, normalization_status FROM declarations WHERE rcno = ?`, rcno).Scan(&sourceID, &status); err != nil {
		t.Fatal(err)
	}
	if firstID == latestSameID || sourceID != latestSameID || status != string(normalization.StatusPending) {
		t.Fatalf("source=%d status=%s, want latest=%d and PENDING", sourceID, status, latestSameID)
	}
	fixedUpdatedAt := time.Date(2026, 8, 4, 10, 0, 0, 123000, time.UTC)
	if _, err := store.db.Exec(`UPDATE declarations SET updated_at = ? WHERE rcno = ?`, fixedUpdatedAt, rcno); err != nil {
		t.Fatal(err)
	}
	var updatedAtBefore string
	if err := store.db.QueryRow(`SELECT DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f') FROM declarations WHERE rcno = ?`, rcno).Scan(&updatedAtBefore); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	var updatedAtAfter string
	if err := store.db.QueryRow(`SELECT DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f') FROM declarations WHERE rcno = ?`, rcno).Scan(&updatedAtAfter); err != nil {
		t.Fatal(err)
	}
	if updatedAtAfter != updatedAtBefore {
		t.Fatalf("idempotent sync changed updated_at: got %s want %s", updatedAtAfter, updatedAtBefore)
	}
	if _, err := store.db.Exec(`
		UPDATE declarations
		SET normalization_status = 'NORMALIZED', claim_owner = 'old-worker',
		    claim_lease_until = DATE_ADD(NOW(6), INTERVAL 1 HOUR), claim_attempts = 3,
		    claim_next_attempt_at = DATE_ADD(NOW(6), INTERVAL 1 HOUR), claim_last_error = 'old failure',
		    review_status = 'APPROVED', reviewed_by = 'reviewer', reviewed_at = NOW(6), review_note = 'old review'
		WHERE rcno = ?
	`, rcno); err != nil {
		t.Fatal(err)
	}
	latestDriftID := fixture.item(t, rcno, "changed", base.Add(2*time.Second), base.AddDate(0, 0, 2))
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT source_item_id, normalization_status FROM declarations WHERE rcno = ?`, rcno).Scan(&sourceID, &status); err != nil {
		t.Fatal(err)
	}
	if sourceID != latestDriftID || status != string(normalization.StatusStale) {
		t.Fatalf("source=%d status=%s, want latest=%d and STALE", sourceID, status, latestDriftID)
	}
	var attempts int
	var owner, lastError, reviewedBy, reviewNote sql.NullString
	var leaseUntil, nextAttemptAt, reviewedAt sql.NullTime
	var reviewStatus string
	if err := store.db.QueryRow(`
		SELECT claim_attempts, claim_owner, claim_lease_until, claim_next_attempt_at, claim_last_error,
		       review_status, reviewed_by, reviewed_at, review_note
		FROM declarations WHERE rcno = ?
	`, rcno).Scan(&attempts, &owner, &leaseUntil, &nextAttemptAt, &lastError, &reviewStatus, &reviewedBy, &reviewedAt, &reviewNote); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || owner.Valid || leaseUntil.Valid || nextAttemptAt.Valid || lastError.Valid ||
		reviewStatus != "NOT_REQUIRED" || reviewedBy.Valid || reviewedAt.Valid || reviewNote.Valid {
		t.Fatalf("semantic drift did not reset revision state: attempts=%d owner=%v lease=%v next=%v error=%v review=%s",
			attempts, owner, leaseUntil, nextAttemptAt, lastError, reviewStatus)
	}
}

func TestNormalizationStore_ClaimFencing실패재시도와강제재정제를보장한다(t *testing.T) {
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	rcno := fmt.Sprintf("NORM-CLAIM-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture.item(t, rcno, "claim", now, now)
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(context.Background(), normalizationRequest(rcno, "worker-a"))
	if err != nil || len(first) != 1 || first[0].ClaimAttempt != 1 {
		t.Fatalf("first claim=%+v error=%v", first, err)
	}
	second, err := store.Claim(context.Background(), normalizationRequest(rcno, "worker-b"))
	if err != nil || len(second) != 0 {
		t.Fatalf("concurrent claim=%+v error=%v, want no row", second, err)
	}
	if _, err := store.db.Exec(`UPDATE declarations SET claim_lease_until = DATE_SUB(NOW(6), INTERVAL 1 SECOND) WHERE rcno = ?`, rcno); err != nil {
		t.Fatal(err)
	}
	retried, err := store.Claim(context.Background(), normalizationRequest(rcno, "worker-b"))
	if err != nil || len(retried) != 1 || retried[0].ClaimAttempt != 2 {
		t.Fatalf("retry claim=%+v error=%v", retried, err)
	}
	if err := store.Complete(context.Background(), normalization.Completion{
		Source: first[0], Result: normalization.Result{Status: normalization.StatusNormalized},
		NormalizationVersion: "test", NormalizedAt: time.Now(),
	}); !errors.Is(err, errNormalizationLeaseLost) {
		t.Fatalf("stale completion error=%v, want lease lost", err)
	}
	if err := store.Fail(context.Background(), normalization.Failure{
		Source: retried[0], RetryDelay: -time.Second, LastError: "parser failed",
	}); err != nil {
		t.Fatal(err)
	}
	forced, err := store.Claim(context.Background(), normalizationRequest(rcno, "worker-c"))
	if err != nil || len(forced) != 1 || forced[0].ClaimAttempt != 3 {
		t.Fatalf("forced claim=%+v error=%v", forced, err)
	}
	if err := store.Complete(context.Background(), normalization.Completion{
		Source: forced[0], Result: normalization.Result{Status: normalization.StatusPartial},
		NormalizationVersion: "test", NormalizedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	forcedAgain, err := store.Claim(context.Background(), normalizationRequest(rcno, "worker-d"))
	if err != nil || len(forcedAgain) != 1 || forcedAgain[0].ClaimAttempt != 4 {
		t.Fatalf("force beyond max attempts=%+v error=%v", forcedAgain, err)
	}
}

func TestNormalizationStore_Complete_후보를저장하고관리자선택을보존한다(t *testing.T) {
	// Given
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	rcno := fmt.Sprintf("NORM-MATCH-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture.item(t, rcno, "matching", now, now)
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	sources, err := store.Claim(context.Background(), normalizationRequest(rcno, "matching-worker"))
	if err != nil || len(sources) != 1 {
		t.Fatalf("claim = %+v, error = %v", sources, err)
	}
	if _, err := store.db.Exec(`
		UPDATE declarations SET selected_alcohol_id = 900, selected_distillery_id = 901, selected_region_id = 902 WHERE rcno = ?
	`, rcno); err != nil {
		t.Fatal(err)
	}

	// When
	err = store.Complete(context.Background(), normalization.Completion{
		Source: sources[0],
		Result: normalization.Result{
			Status: normalization.StatusNormalized,
			Fields: normalization.Fields{
				AlcoholCandidates:    []normalization.ReferenceCandidate{{ID: 1, Score: 100}, {ID: 2, Score: 60}},
				DistilleryCandidates: []normalization.ReferenceCandidate{{ID: 11, Score: 100}, {ID: 12, Score: 60}},
				RegionCandidates:     []normalization.ReferenceCandidate{{ID: 21, Score: 80}},
				MatchingVersion:      "matching-test",
			},
		},
		NormalizationVersion: "normalization-test",
		NormalizedAt:         now,
	})

	// Then
	if err != nil {
		t.Fatal(err)
	}
	var selectedAlcohol, selectedDistillery, selectedRegion, firstAlcohol, secondAlcohol, firstDistillery, secondDistillery, firstRegion int64
	var firstAlcoholScore, secondAlcoholScore, firstDistilleryScore, secondDistilleryScore, firstRegionScore float64
	if err := store.db.QueryRow(`
		SELECT selected_alcohol_id, selected_distillery_id, selected_region_id,
		       alcohol_candidate_1_id, alcohol_candidate_1_score,
		       alcohol_candidate_2_id, alcohol_candidate_2_score,
		       distillery_candidate_1_id, distillery_candidate_1_score,
		       distillery_candidate_2_id, distillery_candidate_2_score,
		       region_candidate_1_id, region_candidate_1_score
		FROM declarations WHERE rcno = ?
	`, rcno).Scan(
		&selectedAlcohol, &selectedDistillery, &selectedRegion,
		&firstAlcohol, &firstAlcoholScore, &secondAlcohol, &secondAlcoholScore,
		&firstDistillery, &firstDistilleryScore, &secondDistillery, &secondDistilleryScore,
		&firstRegion, &firstRegionScore,
	); err != nil {
		t.Fatal(err)
	}
	if selectedAlcohol != 900 || selectedDistillery != 901 || selectedRegion != 902 {
		t.Fatalf("selected IDs changed: alcohol=%d distillery=%d region=%d", selectedAlcohol, selectedDistillery, selectedRegion)
	}
	if firstAlcohol != 1 || firstAlcoholScore != 100 || secondAlcohol != 2 || secondAlcoholScore != 60 ||
		firstDistillery != 11 || firstDistilleryScore != 100 || secondDistillery != 12 || secondDistilleryScore != 60 ||
		firstRegion != 21 || firstRegionScore != 80 {
		t.Fatalf("candidate slots mismatch: alcohol=(%d,%.2f),(%d,%.2f) distillery=(%d,%.2f),(%d,%.2f) region=(%d,%.2f)",
			firstAlcohol, firstAlcoholScore, secondAlcohol, secondAlcoholScore,
			firstDistillery, firstDistilleryScore, secondDistillery, secondDistilleryScore, firstRegion, firstRegionScore)
	}
}

func TestNormalizationStore_동시Claim_하나의Worker만획득한다(t *testing.T) {
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	rcno := fmt.Sprintf("NC-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture.item(t, rcno, "concurrent", now, now)
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		sources []normalization.Source
		err     error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for _, owner := range []string{"worker-a", "worker-b"} {
		go func(owner string) {
			<-start
			sources, err := store.Claim(context.Background(), normalizationRequest(rcno, owner))
			results <- claimResult{sources: sources, err: err}
		}(owner)
	}
	close(start)
	claimed := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		claimed += len(result.sources)
	}
	if claimed != 1 {
		t.Fatalf("claimed rows = %d, want 1", claimed)
	}
}

func TestNormalizationStore_재시도소진행_뒤의Pending을굶기지않는다(t *testing.T) {
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	now := time.Now().UTC().Truncate(time.Microsecond)
	exhaustedRCNO := fmt.Sprintf("NE-%d", time.Now().UnixNano())
	nextRCNO := fmt.Sprintf("NN-%d", time.Now().UnixNano())
	fixture.item(t, exhaustedRCNO, "exhausted", now, now.AddDate(0, 0, 1))
	fixture.item(t, nextRCNO, "next", now, now)
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE declarations SET claim_attempts = 3 WHERE rcno = ?`, exhaustedRCNO); err != nil {
		t.Fatal(err)
	}
	request := normalizationRequest("", "worker-next")
	claimed, err := store.Claim(context.Background(), request)
	if err != nil || len(claimed) != 1 || claimed[0].RCNO != nextRCNO {
		t.Fatalf("claim=%+v error=%v, want rcno=%s", claimed, err, nextRCNO)
	}
}

func TestNormalizationStore_Preview와Remaining은미동기화원장을읽기전용으로반영한다(t *testing.T) {
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	rcno := fmt.Sprintf("NORM-PREVIEW-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	itemID := fixture.item(t, rcno, "preview", now, now)
	var rawHTML string
	var semanticHash []byte
	if err := store.db.QueryRow(`SELECT raw_row_html, semantic_sha256 FROM items WHERE id = ?`, itemID).Scan(&rawHTML, &semanticHash); err != nil {
		t.Fatal(err)
	}
	preview, err := store.Preview(context.Background(), normalizationRequest(rcno, "dry-run"))
	if err != nil || len(preview) != 1 || preview[0].DeclarationID != 0 || preview[0].SourceItemID != itemID {
		t.Fatalf("preview=%+v error=%v", preview, err)
	}
	remaining, err := store.Remaining(context.Background())
	if err != nil || remaining.Pending < 1 {
		t.Fatalf("remaining=%+v error=%v, want pending including unmaterialized RCNO", remaining, err)
	}
	var declarationCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM declarations WHERE rcno = ?`, rcno).Scan(&declarationCount); err != nil {
		t.Fatal(err)
	}
	if declarationCount != 0 {
		t.Fatalf("preview created %d declarations", declarationCount)
	}
	var rawHTMLAfter string
	var semanticHashAfter []byte
	if err := store.db.QueryRow(`SELECT raw_row_html, semantic_sha256 FROM items WHERE id = ?`, itemID).Scan(&rawHTMLAfter, &semanticHashAfter); err != nil {
		t.Fatal(err)
	}
	if rawHTMLAfter != rawHTML || string(semanticHashAfter) != string(semanticHash) {
		t.Fatal("preview changed immutable ledger source")
	}
}
