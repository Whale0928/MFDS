package weblist

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Service struct {
	store       Store
	source      ListSource
	targets     []Target
	pageSize    int
	location    *time.Location
	webBaseURL  string
	now         func() time.Time
	rateMu      sync.Mutex
	nextFetch   time.Time
	fetchGap    time.Duration
	processID   string
	maxAttempts int
	retryDelays []time.Duration
}

type jobConfigSnapshot struct {
	SourceKind string   `json:"source_kind"`
	Items      []Target `json:"items"`
	PageSize   int      `json:"page_size"`
	WebBaseURL string   `json:"web_base_url"`
}

func NewService(store Store, source ListSource, options Options) (*Service, error) {
	if store == nil || source == nil {
		return nil, fmt.Errorf("웹 목록 store와 source가 필요합니다")
	}
	if options.Location == nil {
		return nil, fmt.Errorf("웹 목록 timezone이 필요합니다")
	}
	if options.PageSize < 1 || options.PageSize > MaximumPageSize {
		return nil, fmt.Errorf("웹 목록 page size는 1 이상 %d 이하여야 합니다", MaximumPageSize)
	}
	if err := validateTargets(options.Targets); err != nil {
		return nil, err
	}
	baseURL, err := canonicalWebBaseURL(options.WebBaseURL)
	if err != nil {
		return nil, err
	}
	processID := strings.TrimSpace(options.ProcessID)
	if processID == "" {
		random := make([]byte, 8)
		if _, err := rand.Read(random); err != nil {
			return nil, fmt.Errorf("worker process ID 생성 실패: %w", err)
		}
		processID = hex.EncodeToString(random)
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	maxAttempts := options.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	retryDelays := append([]time.Duration(nil), options.RetryDelays...)
	if len(retryDelays) == 0 {
		retryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
	}
	var fetchGap time.Duration
	if options.QPS > 0 {
		fetchGap = time.Duration(float64(time.Second) / options.QPS)
	}
	return &Service{
		store: store, source: source, targets: append([]Target(nil), options.Targets...),
		pageSize: options.PageSize, location: options.Location, webBaseURL: baseURL,
		now: now, fetchGap: fetchGap, processID: processID, maxAttempts: maxAttempts,
		retryDelays: retryDelays,
	}, nil
}

func canonicalWebBaseURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("웹 base URL은 query, fragment, userinfo가 없는 absolute http/https URL이어야 합니다")
	}
	return parsed.String(), nil
}

func (s *Service) ExecuteJob(ctx context.Context, command JobCommand) (JobResult, error) {
	from, to, err := s.parseRange(command.FromDate, command.ToDate)
	if err != nil {
		return JobResult{}, err
	}
	workers := command.Workers
	if workers < 1 {
		workers = 1
	}
	configJSON, err := json.Marshal(jobConfigSnapshot{
		SourceKind: "WEB_LIST", Items: s.targets, PageSize: s.pageSize,
		WebBaseURL: s.webBaseURL,
	})
	if err != nil {
		return JobResult{}, fmt.Errorf("job 설정 snapshot 생성 실패: %w", err)
	}
	record, err := s.store.StartWebListJob(ctx, JobStartParams{
		FromDate: from, ToDate: to, StartedAt: s.now(),
		PageSize: uint32(s.pageSize), ConfigJSON: configJSON,
	})
	if err != nil {
		return JobResult{}, err
	}
	if command.OnStarted != nil {
		command.OnStarted(record.RunID)
	}

	var waitGroup sync.WaitGroup
	errs := make(chan error, workers)
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func(workerIndex int) {
			defer waitGroup.Done()
			if workerErr := s.runWorker(ctx, record.RunID, workerIndex); workerErr != nil {
				errs <- workerErr
			}
		}(index)
	}
	waitGroup.Wait()
	close(errs)
	var executionErr error
	for workerErr := range errs {
		executionErr = errors.Join(executionErr, workerErr)
	}
	result, loadErr := s.store.LoadJobResult(context.WithoutCancel(ctx), record.RunID)
	return result, errors.Join(executionErr, loadErr)
}

func (s *Service) runWorker(ctx context.Context, jobID uint64, workerIndex int) error {
	owner := fmt.Sprintf("web-list/%d/%s/%02d", jobID, s.processID, workerIndex)
	for {
		if err := ctx.Err(); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), failureCleanupTimeout)
			requestErr := s.store.RequestCancellation(cleanupCtx, jobID)
			cancel()
			return errors.Join(err, requestErr)
		}
		task, found, err := s.store.ClaimTask(ctx, ClaimParams{
			RunID: jobID, Owner: owner, Now: s.now(),
			LeaseUntil: s.now().Add(defaultExecutionLease),
		})
		if err != nil {
			return err
		}
		if found {
			if err := s.processDateTask(ctx, task); err != nil {
				return err
			}
			continue
		}
		status, _, err := s.store.FinalizeJob(ctx, jobID, s.now())
		if err != nil {
			return err
		}
		if status == RunStatusCompleted || status == RunStatusPartialFailed || status == RunStatusCancelled {
			return nil
		}
		timer := time.NewTimer(workerIdleInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (s *Service) processDateTask(ctx context.Context, task DateTask) error {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	renewErrors := make(chan error, 1)
	stopRenew := make(chan struct{})
	go s.renewTask(taskCtx, task, cancel, stopRenew, renewErrors)
	defer close(stopRenew)

	for _, target := range s.targets {
		totalSnapshot := (*int)(nil)
		totalPages := 1
		for pageNo := 1; pageNo <= totalPages; pageNo++ {
			request := ListRequest{
				ItemName: target.Name, ItemCode: target.Code, ProcessDate: task.ProcessDate,
				Page: pageNo, Limit: s.pageSize, TotalSnapshot: totalSnapshot,
			}
			if err := s.waitForFetchSlot(taskCtx); err != nil {
				return s.failDateTask(task, request, Fetch{}, "CANCELLED", err, false)
			}
			fetch, fetchErr := s.source.FetchList(taskCtx, request)
			if fetchErr != nil {
				kind := s.source.ErrorKind(fetchErr)
				if kind == "" {
					kind = "NETWORK"
				}
				return s.failDateTask(task, request, fetch, kind, fetchErr, false)
			}
			page, parseErr := s.source.ParseList(fetch.Body, request)
			if parseErr != nil {
				return s.failDateTask(task, request, fetch, "PARSE", parseErr, true)
			}
			if pageNo == 1 {
				totalPages = max(1, page.TotalPages)
				snapshot := page.Total
				totalSnapshot = &snapshot
			}
			if err := s.store.CommitFetch(taskCtx, CommitFetchParams{
				Task: task, Request: request, Fetch: fetch, Page: page,
				ObservedAt: fetch.FinishedAt,
			}); err != nil {
				if errors.Is(err, ErrLeaseLost) {
					return nil
				}
				return err
			}
			select {
			case renewErr := <-renewErrors:
				if renewErr != nil {
					return nil
				}
			default:
			}
		}
	}
	evidence, err := s.store.LoadTaskAttemptEvidence(taskCtx, task)
	if err != nil {
		return err
	}
	reconciliation := ReconcileTaskAttempt(
		evidence,
		task.Attempt,
		s.targets,
		s.pageSize,
	)
	if !reconciliation.Complete {
		return s.retryDateTaskAfterReconciliation(task, reconciliation.Reason)
	}
	if err := s.store.CompleteTask(taskCtx, task); err != nil && !errors.Is(err, ErrLeaseLost) {
		return err
	}
	return nil
}

func (s *Service) retryDateTaskAfterReconciliation(task DateTask, reason string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), failureCleanupTimeout)
	defer cancel()
	err := s.store.FailTask(cleanupCtx, FailTaskParams{
		Task:          task,
		Retryable:     true,
		MaxAttempts:   s.maxAttempts,
		NextAttemptAt: s.now().Add(s.retryDelay(task.Attempt)),
		ErrorMessage:  SanitizeErrorMessage(reason),
	})
	if errors.Is(err, ErrLeaseLost) {
		return nil
	}
	return err
}

func (s *Service) failDateTask(
	task DateTask,
	request ListRequest,
	fetch Fetch,
	kind string,
	cause error,
	parserFailure bool,
) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), failureCleanupTimeout)
	defer cancel()
	if err := s.store.RecordFailedFetch(cleanupCtx, FailedFetchParams{
		Task: task, Request: request, Fetch: fetch, ErrorKind: kind,
		ErrorMessage: SanitizeErrorMessage(cause.Error()), ParserFailure: parserFailure,
	}); err != nil && !errors.Is(err, ErrLeaseLost) {
		return errors.Join(cause, err)
	}
	retryable := kind == "NETWORK" || kind == "HTTP" || kind == "CHALLENGE" || kind == "CANCELLED"
	err := s.store.FailTask(cleanupCtx, FailTaskParams{
		Task: task, Retryable: retryable, MaxAttempts: s.maxAttempts,
		NextAttemptAt: s.now().Add(s.retryDelay(task.Attempt)),
		ErrorMessage:  SanitizeErrorMessage(cause.Error()),
	})
	if errors.Is(err, ErrLeaseLost) {
		return nil
	}
	return err
}

func (s *Service) renewTask(
	ctx context.Context,
	task DateTask,
	cancel context.CancelFunc,
	stop <-chan struct{},
	errs chan<- error,
) {
	ticker := time.NewTicker(leaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.store.RenewTask(ctx, task, s.now().Add(defaultExecutionLease)); err != nil {
				errs <- err
				cancel()
				return
			}
		}
	}
}

func (s *Service) waitForFetchSlot(ctx context.Context) error {
	if s.fetchGap <= 0 {
		return nil
	}
	s.rateMu.Lock()
	wait := time.Until(s.nextFetch)
	if wait < 0 {
		wait = 0
	}
	s.nextFetch = time.Now().Add(wait).Add(s.fetchGap)
	s.rateMu.Unlock()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) retryDelay(attempt uint32) time.Duration {
	index := int(attempt) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(s.retryDelays) {
		index = len(s.retryDelays) - 1
	}
	return s.retryDelays[index]
}

func (s *Service) parseRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(fromRaw), s.location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("수집 시작일은 YYYY-MM-DD 형식이어야 합니다: %w", err)
	}
	to, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(toRaw), s.location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("수집 종료일은 YYYY-MM-DD 형식이어야 합니다: %w", err)
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("수집 시작일은 종료일보다 늦을 수 없습니다")
	}
	return from, to, nil
}
