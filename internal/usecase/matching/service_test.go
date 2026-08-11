package matching

import (
	"context"
	"testing"
	"time"

	domain "github.com/bottle-note/mfds-crawler/internal/matching"
)

func TestService_일반실행_후보를저장하고선택값을다루지않는다(t *testing.T) {
	// Given
	distilleryID, regionID := int64(10), int64(20)
	snapshot, err := domain.NewReferenceSnapshot(
		[]domain.AlcoholReference{{ID: 1, KorName: "테스트 위스키", EngName: "Test Whisky", DistilleryID: distilleryID, RegionID: regionID}},
		[]domain.DistilleryReference{{ID: distilleryID, KorName: "테스트 증류소"}},
		[]domain.RegionReference{{ID: regionID, KorName: "테스트 리전"}},
		domain.DefaultMatchingVersion("test-hash"),
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{sources: []Source{{DeclarationID: 1, RCNO: "R-1", BaseProductNameKO: "테스트 위스키"}}}
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	service, err := NewService(store, snapshot, Options{DefaultLimit: 100, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}

	// When
	summary, err := service.Execute(context.Background(), Command{})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(store.saved) != 1 || len(store.saved[0].Result.Distilleries) != 1 || len(store.saved[0].Result.Regions) != 1 {
		t.Fatalf("saved = %#v", store.saved)
	}
	if store.saved[0].MatchedAt != now || summary.Processed != 1 || summary.DistilleryMatched != 1 || summary.RegionMatched != 1 {
		t.Fatalf("saved=%#v summary=%+v", store.saved[0], summary)
	}
}

func TestService_DryRun_결과를저장하지않고전체를강제평가한다(t *testing.T) {
	// Given
	snapshot, err := domain.NewReferenceSnapshot(nil, []domain.DistilleryReference{{ID: 1, EngName: "Alpha"}}, nil, domain.DefaultMatchingVersion("test-hash"))
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{sources: []Source{{RCNO: "R-1", BaseProductNameEN: "Alpha"}}, remaining: 7}
	service, err := NewService(store, snapshot, Options{DefaultLimit: 100})
	if err != nil {
		t.Fatal(err)
	}

	// When
	summary, err := service.Execute(context.Background(), Command{All: true, DryRun: true})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(store.saved) != 0 || !store.query.Force || store.query.Limit != 0 {
		t.Fatalf("store = %+v", store)
	}
	if summary.Processed != 1 || summary.DistilleryMatched != 1 || summary.Remaining != 7 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestService_RCNO와범위옵션충돌_오류를반환한다(t *testing.T) {
	// Given
	snapshot, err := domain.NewReferenceSnapshot(nil, []domain.DistilleryReference{{ID: 1, EngName: "Alpha"}}, nil, domain.DefaultMatchingVersion("test-hash"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(&memoryStore{}, snapshot, Options{DefaultLimit: 100})
	if err != nil {
		t.Fatal(err)
	}

	// When
	_, runErr := service.Execute(context.Background(), Command{RCNO: "R-1", Limit: 10})

	// Then
	if runErr == nil {
		t.Fatal("Execute() error = nil")
	}
}

func TestService_RCNO_현재버전도강제재매칭한다(t *testing.T) {
	// Given
	snapshot, err := domain.NewReferenceSnapshot(nil, []domain.DistilleryReference{{ID: 1, EngName: "Alpha"}}, nil, domain.DefaultMatchingVersion("test-hash"))
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{sources: []Source{{RCNO: "R-1", BaseProductNameEN: "Alpha"}}}
	service, err := NewService(store, snapshot, Options{DefaultLimit: 100})
	if err != nil {
		t.Fatal(err)
	}

	// When
	_, err = service.Execute(context.Background(), Command{RCNO: "R-1"})

	// Then
	if err != nil {
		t.Fatal(err)
	}
	if !store.query.Force || store.query.Limit != 1 || store.query.RCNO != "R-1" {
		t.Fatalf("query = %+v", store.query)
	}
}

type memoryStore struct {
	sources   []Source
	saved     []Completion
	query     Query
	remaining int
}

func (s *memoryStore) ListMatchingSources(_ context.Context, query Query) ([]Source, error) {
	s.query = query
	return append([]Source(nil), s.sources...), nil
}

func (s *memoryStore) SaveMatchingResult(_ context.Context, completion Completion) error {
	s.saved = append(s.saved, completion)
	return nil
}

func (s *memoryStore) MatchingRemaining(context.Context, string) (int, error) {
	return s.remaining, nil
}
