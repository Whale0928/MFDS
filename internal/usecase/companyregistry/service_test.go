package companyregistry

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeClient struct {
	fetch func(PageRequest) (Page, error)
}

func (f fakeClient) FetchPage(_ context.Context, request PageRequest) (Page, error) {
	return f.fetch(request)
}

type memoryStore struct {
	pages      []Page
	pageErrors []error
	importers  []Importer
	candidates []LicenseCandidate
	evidence   []MatchEvidence
	completed  bool
	failed     bool
}

func (s *memoryStore) StartCollection(context.Context, time.Time, json.RawMessage) (uint64, error) {
	return 41, nil
}

func (s *memoryStore) SavePage(_ context.Context, _ uint64, page Page, err error) error {
	s.pages = append(s.pages, page)
	s.pageErrors = append(s.pageErrors, err)
	return nil
}

func (s *memoryStore) ListLatestImporters(context.Context) ([]Importer, error) {
	return s.importers, nil
}

func (s *memoryStore) ListC001Candidates(context.Context, uint64) ([]LicenseCandidate, error) {
	return s.candidates, nil
}

func (s *memoryStore) SaveMatchEvidence(_ context.Context, _ uint64, evidence []MatchEvidence) error {
	s.evidence = append(s.evidence, evidence...)
	return nil
}

func (s *memoryStore) CompleteCollection(context.Context, uint64, Summary, time.Time) error {
	s.completed = true
	return nil
}

func (s *memoryStore) FailCollection(context.Context, uint64, error, time.Time) error {
	s.failed = true
	return nil
}

type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary" }
func (temporaryError) Retryable() bool { return true }

func TestCollect_네서비스정상응답_원문페이지와매칭근거를저장한다(t *testing.T) {
	store := &memoryStore{
		importers:  []Importer{{SourceItemID: 7, RCNO: "R1", Name: "주식회사 회사A"}},
		candidates: []LicenseCandidate{{RawID: 9, LicenseNo: "L1", BusinessName: "회사A"}},
	}
	client := fakeClient{fetch: func(request PageRequest) (Page, error) {
		return Page{
			Service: request.Service, StartIndex: request.StartIndex, EndIndex: request.EndIndex,
			ResultCode: "INFO-000", TotalCount: 1, Rows: []json.RawMessage{json.RawMessage(`{"LCNS_NO":"L1"}`)},
		}, nil
	}}
	service := newTestService(t, store, client, 2)

	summary, err := service.Collect(context.Background())

	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !store.completed || store.failed || len(store.pages) != 4 {
		t.Fatalf("completed=%t failed=%t pages=%d", store.completed, store.failed, len(store.pages))
	}
	if len(store.evidence) != 1 || store.evidence[0].Status != MatchNormalizedName {
		t.Fatalf("evidence = %+v", store.evidence)
	}
	if summary.Matches[MatchNormalizedName] != 1 {
		t.Fatalf("matches = %+v", summary.Matches)
	}
}

func TestCollect_I2821TotalCount가범위값_짧은페이지에서종료한다(t *testing.T) {
	store := &memoryStore{}
	client := fakeClient{fetch: func(request PageRequest) (Page, error) {
		rows := []json.RawMessage{json.RawMessage(`{}`)}
		return Page{Service: request.Service, ResultCode: "INFO-000", TotalCount: uint64(request.EndIndex), Rows: rows}, nil
	}}
	service := newTestService(t, store, client, 1000)

	_, err := service.Collect(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	counts := map[ServiceID]int{}
	for _, page := range store.pages {
		counts[page.Service]++
	}
	if counts[ServiceI2821] != 1 {
		t.Fatalf("I2821 fetches = %d", counts[ServiceI2821])
	}
}

func TestCollect_Retryable오류후성공_실패응답과재시도를모두기록한다(t *testing.T) {
	store := &memoryStore{}
	attempts := map[ServiceID]int{}
	client := fakeClient{fetch: func(request PageRequest) (Page, error) {
		attempts[request.Service]++
		if request.Service == ServiceC001 && attempts[request.Service] == 1 {
			return Page{RawBody: []byte("<html>")}, temporaryError{}
		}
		return Page{ResultCode: "INFO-200", TotalCount: 0}, nil
	}}
	service := newTestService(t, store, client, 10)

	_, err := service.Collect(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if len(store.pages) != 5 || store.pageErrors[0] == nil || attempts[ServiceC001] != 2 {
		t.Fatalf("pages=%d first_error=%v attempts=%d", len(store.pages), store.pageErrors[0], attempts[ServiceC001])
	}
}

func TestCollect_동일정규화명에복수인허가_자동확정하지않는다(t *testing.T) {
	store := &memoryStore{
		importers: []Importer{{SourceItemID: 1, RCNO: "R1", Name: "(주)회사A"}},
		candidates: []LicenseCandidate{
			{RawID: 1, LicenseNo: "L1", BusinessName: "회사A"},
			{RawID: 2, LicenseNo: "L2", BusinessName: "주식회사 회사A"},
		},
	}
	client := fakeClient{fetch: func(PageRequest) (Page, error) {
		return Page{ResultCode: "INFO-200"}, nil
	}}
	service := newTestService(t, store, client, 10)

	_, err := service.Collect(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if len(store.evidence) != 1 || store.evidence[0].Status != MatchAmbiguous || store.evidence[0].C001RawID != nil {
		t.Fatalf("evidence = %+v", store.evidence)
	}
}

func TestCollect_재시도불가오류_실행을실패로종료한다(t *testing.T) {
	store := &memoryStore{}
	client := fakeClient{fetch: func(PageRequest) (Page, error) {
		return Page{}, errors.New("invalid key")
	}}
	service := newTestService(t, store, client, 10)

	_, err := service.Collect(context.Background())

	if err == nil || !store.failed || store.completed {
		t.Fatalf("error=%v failed=%t completed=%t", err, store.failed, store.completed)
	}
}

func TestCollect_Run요청제한도달_추가호출없이실패로종료한다(t *testing.T) {
	store := &memoryStore{}
	requests := 0
	client := fakeClient{fetch: func(PageRequest) (Page, error) {
		requests++
		return Page{ResultCode: "INFO-000", TotalCount: 2, Rows: []json.RawMessage{json.RawMessage(`{}`)}}, nil
	}}
	service := newTestService(t, store, client, 1)
	service.opts.MaxRequests = 1

	_, err := service.Collect(context.Background())

	if err == nil || !store.failed || requests != 1 {
		t.Fatalf("error=%v failed=%t requests=%d", err, store.failed, requests)
	}
}

func newTestService(t *testing.T, store Store, client Client, pageSize int) *Service {
	t.Helper()
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	service, err := NewService(store, client, Options{
		PageSize: pageSize, MaxPages: 5, MaxRequests: 100, QPS: 1, MaxAttempts: 3,
		RetryDelays: []time.Duration{0}, MatcherVersion: "test-v1",
		Now: func() time.Time {
			now = now.Add(2 * time.Second)
			return now
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}
