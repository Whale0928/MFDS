package weblist

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

var fixedTargets = []Target{
	{Name: "위스키", Code: "C0314210000000000000"},
	{Name: "브랜디", Code: "C0314220000000000000"},
	{Name: "일반증류주", Code: "C0314230000000000000"},
	{Name: "리큐르", Code: "C0314240000000000000"},
}

type fakeJobStore struct {
	mu       sync.Mutex
	task     DateTask
	status   string
	claimed  bool
	commits  []CommitFetchParams
	failures int
}

func (s *fakeJobStore) StartWebListJob(_ context.Context, params JobStartParams) (JobStartRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.task = DateTask{RunID: 1, TaskID: 1, ProcessDate: params.FromDate}
	s.status = "CREATED"
	return JobStartRecord{RunID: 1}, nil
}

func (s *fakeJobStore) ClaimTask(_ context.Context, params ClaimParams) (DateTask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed || s.status == PartitionStatusDone || s.status == PartitionStatusFailed {
		return DateTask{}, false, nil
	}
	s.claimed = true
	s.task.Attempt++
	s.task.Owner = params.Owner
	s.status = "LEASED"
	return s.task, true, nil
}

func (s *fakeJobStore) RenewTask(context.Context, DateTask, time.Time) error { return nil }

func (s *fakeJobStore) CommitFetch(_ context.Context, params CommitFetchParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commits = append(s.commits, params)
	return nil
}

func (s *fakeJobStore) RecordFailedFetch(context.Context, FailedFetchParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
	return nil
}

func (s *fakeJobStore) LoadTaskAttemptEvidence(
	_ context.Context,
	task DateTask,
) ([]TaskAttemptEvidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempts := make(map[uint32]*TaskAttemptEvidence)
	itemIndexes := make(map[uint32]map[string]int)
	for _, commit := range s.commits {
		attempt := commit.Task.Attempt
		evidence, exists := attempts[attempt]
		if !exists {
			evidence = &TaskAttemptEvidence{Attempt: attempt}
			attempts[attempt] = evidence
			itemIndexes[attempt] = make(map[string]int)
		}
		index, exists := itemIndexes[attempt][commit.Request.ItemCode]
		if !exists {
			index = len(evidence.Items)
			evidence.Items = append(evidence.Items, AttemptItemEvidence{
				ItemName: commit.Request.ItemName,
				ItemCode: commit.Request.ItemCode,
			})
			itemIndexes[attempt][commit.Request.ItemCode] = index
		}
		item := &evidence.Items[index]
		item.Pages = append(item.Pages, AttemptPageEvidence{
			PageNo:     uint32(commit.Request.Page),
			Status:     "PARSED",
			Total:      uint64(commit.Page.Total),
			ParsedRows: uint32(len(commit.Page.Rows)),
		})
		for _, row := range commit.Page.Rows {
			item.RCNOs = append(item.RCNOs, row.RCNO)
		}
	}
	result := make([]TaskAttemptEvidence, 0, len(attempts))
	for attempt := uint32(1); attempt <= task.Attempt; attempt++ {
		if evidence, exists := attempts[attempt]; exists {
			result = append(result, *evidence)
		}
	}
	return result, nil
}

func (s *fakeJobStore) CompleteTask(_ context.Context, _ DateTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = PartitionStatusDone
	return nil
}

func (s *fakeJobStore) FailTask(_ context.Context, params FailTaskParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimed = false
	if params.Retryable && int(params.Task.Attempt) < params.MaxAttempts {
		s.status = "RETRY_WAIT"
	} else {
		s.status = PartitionStatusFailed
	}
	return nil
}

func (s *fakeJobStore) RequestCancellation(context.Context, uint64) error { return nil }

func (s *fakeJobStore) FinalizeJob(_ context.Context, _ uint64, _ time.Time) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.status {
	case PartitionStatusDone:
		s.status = RunStatusCompleted
	case PartitionStatusFailed:
		s.status = RunStatusPartialFailed
	}
	return s.status, true, nil
}

func (s *fakeJobStore) LoadJobResult(context.Context, uint64) (JobResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := JobResult{RunID: 1, Status: s.status, TotalPartitions: 1}
	if s.status == RunStatusCompleted {
		result.CompletedPartitions = 1
	}
	if s.status == RunStatusPartialFailed {
		result.FailedPartitions = 1
	}
	result.FetchedPages = uint32(len(s.commits))
	return result, nil
}

type fakeListSource struct {
	mu        sync.Mutex
	requests  []ListRequest
	failFirst bool
}

func (s *fakeListSource) FetchList(_ context.Context, request ListRequest) (Fetch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if s.failFirst {
		s.failFirst = false
		return Fetch{}, errors.New("temporary network error")
	}
	var key [32]byte
	key[0] = byte(len(s.requests))
	return Fetch{
		Method: "GET", URL: "http://example.test/list", QueryJSON: []byte(`{}`),
		RequestKeySHA256: key, StartedAt: time.Now(), FinishedAt: time.Now(),
		Body: []byte("fixture"), BodyGZIP: []byte("gzip"), BodyCaptured: true,
	}, nil
}

func (s *fakeListSource) ParseList(_ []byte, request ListRequest) (Page, error) {
	total := 1
	if request.ItemName == "위스키" {
		total = 2
	}
	totalPages := (total + request.Limit - 1) / request.Limit
	rcno := fmt.Sprintf("%09d%03d", request.Page, targetIndex(request.ItemCode)+1)
	return Page{
		Total: total, Page: request.Page, Limit: request.Limit, TotalPages: totalPages,
		Rows: []Row{{RowNo: 1, RCNO: rcno}},
	}, nil
}

func (s *fakeListSource) ErrorKind(error) string { return "NETWORK" }

func newTestService(t *testing.T, store Store, source ListSource) *Service {
	t.Helper()
	service, err := NewService(store, source, Options{
		Targets: fixedTargets, PageSize: 1, QPS: 0, MaxAttempts: 3,
		RetryDelays: []time.Duration{time.Nanosecond}, Location: time.UTC,
		WebBaseURL: "http://example.test", ProcessID: "0011223344556677",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestExecuteJob_날짜Task하나_고정4개품목과추가페이지를모두조회한다(t *testing.T) {
	store := &fakeJobStore{}
	source := &fakeListSource{}
	service := newTestService(t, store, source)

	result, err := service.ExecuteJob(context.Background(), JobCommand{
		FromDate: "2025-03-28", ToDate: "2025-03-28", Workers: 2,
	})
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("status = %s, want %s", result.Status, RunStatusCompleted)
	}
	if len(source.requests) != 10 {
		t.Fatalf("request count = %d, want 10", len(source.requests))
	}
	gotItems := map[string]int{}
	for _, request := range source.requests {
		gotItems[request.ItemName]++
	}
	if gotItems["위스키"] != 4 || gotItems["브랜디"] != 2 ||
		gotItems["일반증류주"] != 2 || gotItems["리큐르"] != 2 {
		t.Fatalf("item requests = %#v", gotItems)
	}
}

func TestExecuteJob_Task첫요청일시실패_날짜전체를재시도한다(t *testing.T) {
	store := &fakeJobStore{}
	source := &fakeListSource{failFirst: true}
	service := newTestService(t, store, source)

	result, err := service.ExecuteJob(context.Background(), JobCommand{
		FromDate: "2025-03-28", ToDate: "2025-03-28", Workers: 1,
	})
	if err != nil {
		t.Fatalf("ExecuteJob() error = %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if store.failures != 1 {
		t.Fatalf("failure count = %d, want 1", store.failures)
	}
	if len(source.requests) != 11 {
		t.Fatalf("request count = %d, want 11", len(source.requests))
	}
}

func TestNewService_고정품목이4개가아님_오류를반환한다(t *testing.T) {
	_, err := NewService(&fakeJobStore{}, &fakeListSource{}, Options{
		Targets: fixedTargets[:3], PageSize: 10, Location: time.UTC,
		WebBaseURL: "http://example.test",
	})
	if err == nil {
		t.Fatal("NewService() error = nil, want error")
	}
}

func targetIndex(code string) int {
	for index, target := range fixedTargets {
		if target.Code == code {
			return index
		}
	}
	return 0
}
