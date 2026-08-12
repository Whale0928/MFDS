package mysql

import (
	"context"
	"fmt"
	"testing"
	"time"

	matchingdomain "github.com/bottle-note/mfds-crawler/internal/matching"
	matchingusecase "github.com/bottle-note/mfds-crawler/internal/usecase/matching"
	"github.com/bottle-note/mfds-crawler/internal/usecase/normalization"
)

func TestMatchingStore_SaveMatchingResult_동시변경된버전을덮어쓰지않는다(t *testing.T) {
	// Given
	store := normalizationStore(t)
	fixture := newNormalizationFixture(t, store)
	rcno := fmt.Sprintf("MATCH-FENCE-%d", time.Now().UnixNano())
	now := time.Now().UTC().Truncate(time.Microsecond)
	fixture.item(t, rcno, "matching-fence", now, now)
	if err := store.SyncDeclarations(context.Background()); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim(context.Background(), normalizationRequest(rcno, "matching-fence-worker"))
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %+v, error = %v", claimed, err)
	}
	if err := store.Complete(context.Background(), normalization.Completion{
		Source: claimed[0], Result: normalization.Result{Status: normalization.StatusNormalized},
		NormalizationVersion: "normalization-test", NormalizedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	sources, err := store.ListMatchingSources(context.Background(), matchingusecase.Query{RCNO: rcno, Force: true})
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources = %+v, error = %v", sources, err)
	}
	if _, err := store.db.Exec(`UPDATE mfds_declarations SET matching_version = 'newer-version' WHERE rcno = ?`, rcno); err != nil {
		t.Fatal(err)
	}

	// When
	err = store.SaveMatchingResult(context.Background(), matchingusecase.Completion{
		Source:  sources[0],
		Result:  matchingdomain.MatchResult{},
		Version: "stale-version", MatchedAt: now.Add(time.Second),
	})

	// Then
	if err == nil {
		t.Fatal("SaveMatchingResult() error = nil")
	}
	var version string
	if err := store.db.QueryRow(`SELECT matching_version FROM mfds_declarations WHERE rcno = ?`, rcno).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "newer-version" {
		t.Fatalf("matching_version = %q", version)
	}
}
