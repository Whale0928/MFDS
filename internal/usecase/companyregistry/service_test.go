package companyregistry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	latest     *time.Time
	window     SyncWindow
	started    bool
	completed  bool
	failed     bool
}

func (s *memoryStore) LatestCompletedThrough(context.Context) (*time.Time, error) {
	return s.latest, nil
}

func (s *memoryStore) StartCollection(_ context.Context, _ time.Time, window SyncWindow, _ json.RawMessage) (uint64, error) {
	s.started = true
	s.window = window
	return 41, nil
}

func (s *memoryStore) SavePage(_ context.Context, _ uint64, page Page, err error) error {
	s.pages = append(s.pages, page)
	s.pageErrors = append(s.pageErrors, err)
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

func TestCollect_네서비스정상응답_변경일자필터와원문페이지를저장한다(t *testing.T) {
	store := &memoryStore{}
	filters := map[ServiceID]map[string]string{}
	client := fakeClient{fetch: func(request PageRequest) (Page, error) {
		filters[request.Service] = request.Filters
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
	if filters[ServiceC001]["CHNG_DT"] != "20260801" || filters[ServiceI2821]["CHNG_DT"] != "20260801" || filters[ServiceI0470]["CHNG_DT"] != "20260801" {
		t.Fatalf("filters = %+v", filters)
	}
	if len(filters[ServiceI0250]) != 0 {
		t.Fatalf("I0250 filters = %+v", filters[ServiceI0250])
	}
	if summary.Since.Format("2006-01-02") != "2026-08-01" || summary.Through.Format("2006-01-02") != "2026-08-09" {
		t.Fatalf("summary window = %s~%s", summary.Since, summary.Through)
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

func TestCollect_Since생략하고완료이력없음_실행을시작하지않는다(t *testing.T) {
	store := &memoryStore{}
	service := newTestService(t, store, fakeClient{fetch: func(PageRequest) (Page, error) { return Page{}, nil }}, 10)
	service.opts.Since = nil

	_, err := service.Collect(context.Background())

	if err == nil || store.started || !strings.Contains(err.Error(), "--since") {
		t.Fatalf("error=%v started=%t", err, store.started)
	}
}

func TestCollect_Since생략하고완료이력있음_마지막성공일을다시포함한다(t *testing.T) {
	latest := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	store := &memoryStore{latest: &latest}
	client := fakeClient{fetch: func(PageRequest) (Page, error) { return Page{ResultCode: "INFO-200"}, nil }}
	service := newTestService(t, store, client, 10)
	service.opts.Since = nil

	_, err := service.Collect(context.Background())

	if err != nil || store.window.Since.Format("2006-01-02") != "2026-08-07" {
		t.Fatalf("error=%v window=%+v", err, store.window)
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
		RetryDelays: []time.Duration{0},
		Since: func() *time.Time {
			value := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
			return &value
		}(),
		Location: time.UTC,
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
