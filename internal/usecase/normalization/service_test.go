package normalization

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestService_일반실행_동기화후Terminal결과를저장한다(t *testing.T) {
	// Given
	store := &memoryStore{
		claimed:   []Source{{RCNO: "R-1", SourceItemID: 10, ClaimOwner: "worker-1", ClaimAttempt: 1}},
		remaining: Remaining{Pending: 4, Stale: 2},
	}
	parser := parserFunc(func(Source) (Result, error) { return Result{Status: StatusNormalized}, nil })
	service := newTestService(t, store, parser)

	// When
	summary, err := service.Execute(context.Background(), Command{})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !store.synced || len(store.completed) != 1 || store.previewed || store.claimedRequest.Limit != 100 {
		t.Fatalf("store = %+v", store)
	}
	if summary.Processed[StatusNormalized] != 1 || summary.SystemFailures != 0 ||
		summary.RemainingPending != 4 || summary.RemainingStale != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestService_Force_동기화후한번만Terminal을Requeue하고재정제한다(t *testing.T) {
	// Given
	store := &memoryStore{
		claimed:   []Source{{RCNO: "R-1", ClaimOwner: "worker-1", ClaimAttempt: 1}},
		remaining: Remaining{Pending: 0, Stale: 0},
	}
	parser := parserFunc(func(Source) (Result, error) { return Result{Status: StatusNormalized}, nil })
	service := newTestService(t, store, parser)

	// When
	_, err := service.Execute(context.Background(), Command{Force: true, Limit: 12294})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.forceRequeueCalls != 1 || !store.synced || !store.claimedCalled || store.claimedRequest.Limit != 12294 {
		t.Fatalf("store = %+v", store)
	}
}

func TestService_Force와RCNO동시지정_조합을거부한다(t *testing.T) {
	// Given
	store := &memoryStore{}
	service := newTestService(t, store, parserFunc(func(Source) (Result, error) {
		return Result{Status: StatusNormalized}, nil
	}))

	// When
	_, err := service.Execute(context.Background(), Command{Force: true, RCNO: "R-1"})

	// Then
	if err == nil || !strings.Contains(err.Error(), "force와 rcno는 함께 사용할 수 없습니다") {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.forceRequeueCalls != 0 || store.synced || store.claimedCalled {
		t.Fatalf("store = %+v", store)
	}
}

func TestService_ForceDryRun_조합을거부한다(t *testing.T) {
	// Given
	store := &memoryStore{}
	service := newTestService(t, store, parserFunc(func(Source) (Result, error) {
		return Result{Status: StatusNormalized}, nil
	}))

	// When
	_, err := service.Execute(context.Background(), Command{Force: true, DryRun: true})

	// Then
	if err == nil || !strings.Contains(err.Error(), "force와 dry-run은 함께 사용할 수 없습니다") {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.forceRequeueCalls != 0 || store.synced || store.previewed || store.claimedCalled {
		t.Fatalf("store = %+v", store)
	}
}

func TestService_DryRun_모든DB변경없이미리보기결과를반환한다(t *testing.T) {
	// Given
	store := &memoryStore{
		preview:   []Source{{RCNO: "R-1", SourceItemID: 10}},
		remaining: Remaining{Pending: 8, Stale: 3},
	}
	parser := parserFunc(func(Source) (Result, error) { return Result{Status: StatusPartial}, nil })
	service := newTestService(t, store, parser)

	// When
	summary, err := service.Execute(context.Background(), Command{DryRun: true})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.synced || store.claimedCalled || len(store.completed) != 0 || len(store.failed) != 0 || !store.previewed {
		t.Fatalf("dry-run mutated store state: %+v", store)
	}
	if summary.Processed[StatusPartial] != 1 || summary.RemainingPending != 8 || summary.RemainingStale != 3 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestService_데이터상태결과_SystemError없이성공한다(t *testing.T) {
	// Given
	store := &memoryStore{claimed: []Source{{RCNO: "R-1"}, {RCNO: "R-2"}, {RCNO: "R-3"}}}
	statuses := []Status{StatusPartial, StatusReviewRequired, StatusUnparsed}
	index := 0
	parser := parserFunc(func(Source) (Result, error) {
		result := Result{Status: statuses[index]}
		index++
		return result, nil
	})
	service := newTestService(t, store, parser)

	// When
	summary, err := service.Execute(context.Background(), Command{})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, status := range statuses {
		if summary.Processed[status] != 1 {
			t.Fatalf("processed[%s] = %d", status, summary.Processed[status])
		}
	}
	if summary.SystemFailures != 0 || len(store.failed) != 0 {
		t.Fatalf("summary=%+v failed=%+v", summary, store.failed)
	}
}

func TestService_Parser실패_다른RCNO를계속처리하고SystemError를반환한다(t *testing.T) {
	// Given
	store := &memoryStore{claimed: []Source{{RCNO: "R-1"}, {RCNO: "R-2"}}}
	parser := parserFunc(func(source Source) (Result, error) {
		if source.RCNO == "R-1" {
			return Result{}, errors.New("unexpected parser failure")
		}
		return Result{Status: StatusNormalized}, nil
	})
	service := newTestService(t, store, parser)

	// When
	summary, err := service.Execute(context.Background(), Command{})

	// Then
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	if summary.SystemFailures != 1 || summary.Processed[StatusNormalized] != 1 ||
		len(store.failed) != 1 || len(store.completed) != 1 {
		t.Fatalf("summary=%+v store=%+v", summary, store)
	}
}

func TestService_RCNO강제재정제_기본Limit1로Claim한다(t *testing.T) {
	// Given
	store := &memoryStore{claimed: []Source{{RCNO: "R-9"}}}
	parser := parserFunc(func(Source) (Result, error) { return Result{Status: StatusNormalized}, nil })
	service := newTestService(t, store, parser)

	// When
	_, err := service.Execute(context.Background(), Command{RCNO: " R-9 "})

	// Then
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if store.claimedRequest.RCNO != "R-9" || store.claimedRequest.Limit != 1 {
		t.Fatalf("claim request = %+v", store.claimedRequest)
	}
}

func TestService_동기화실패_SystemFailure를집계하고오류를반환한다(t *testing.T) {
	// Given
	store := &memoryStore{syncErr: errors.New("database unavailable")}
	parser := parserFunc(func(Source) (Result, error) { return Result{Status: StatusNormalized}, nil })
	service := newTestService(t, store, parser)

	// When
	summary, err := service.Execute(context.Background(), Command{})

	// Then
	if err == nil || summary.SystemFailures != 1 {
		t.Fatalf("summary=%+v error=%v", summary, err)
	}
}

type parserFunc func(Source) (Result, error)

func (f parserFunc) Normalize(source Source) (Result, error) {
	return f(source)
}

type memoryStore struct {
	synced            bool
	claimedCalled     bool
	previewed         bool
	claimed           []Source
	preview           []Source
	claimedRequest    ClaimRequest
	completed         []Completion
	failed            []Failure
	remaining         Remaining
	syncErr           error
	forceRequeueCalls int
}

func (s *memoryStore) SyncDeclarations(context.Context) error {
	s.synced = true
	return s.syncErr
}

func (s *memoryStore) ForceRequeue(context.Context) error {
	s.forceRequeueCalls++
	return nil
}

func (s *memoryStore) Claim(_ context.Context, request ClaimRequest) ([]Source, error) {
	s.claimedCalled = true
	s.claimedRequest = request
	return s.claimed, nil
}

func (s *memoryStore) Preview(_ context.Context, request ClaimRequest) ([]Source, error) {
	s.previewed = true
	s.claimedRequest = request
	return s.preview, nil
}

func (s *memoryStore) Complete(_ context.Context, completion Completion) error {
	s.completed = append(s.completed, completion)
	return nil
}

func (s *memoryStore) Fail(_ context.Context, failure Failure) error {
	s.failed = append(s.failed, failure)
	return nil
}

func (s *memoryStore) Remaining(context.Context) (Remaining, error) {
	return s.remaining, nil
}

func newTestService(t *testing.T, store Store, parser Parser) *Service {
	t.Helper()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	service, err := NewService(store, parser, Options{
		RunLimit: 100, LeaseDuration: time.Minute, MaxAttempts: 3, RetryDelay: time.Minute,
		NormalizationVersion: "v1", Owner: "worker-1", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
