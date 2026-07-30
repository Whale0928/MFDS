//go:build legacy

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

const (
	RunStatusCreated       = "CREATED"
	PartitionStatusPending = "PENDING"
)

type Command struct {
	ItemName    string
	ProcessDate string
}

type JobCommand struct {
	ItemNames []string
	FromDate  string
	ToDate    string
	Workers   int
	OnStarted func(uint64)
}

type Target struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type Options struct {
	Targets     []Target
	PageSize    int
	QPS         float64
	Location    *time.Location
	WebBaseURL  string
	ProxyMode   string
	ProxyLabel  string
	ProcessID   string
	MaxAttempts int
	RetryDelays []time.Duration
	Now         func() time.Time
}

type StartParams struct {
	ItemName    string
	ItemCode    string
	ProcessDate time.Time
	PageSize    uint32
	ConfigJSON  json.RawMessage
}

type StartRecord struct {
	RunID       uint64
	PartitionID uint64
}

type JobPartition struct {
	ItemName    string
	ItemCode    string
	ProcessDate time.Time
}

type JobStartParams struct {
	FromDate, ToDate time.Time
	StartedAt        time.Time
	PageSize         uint32
	ConfigJSON       json.RawMessage
	Partitions       []JobPartition
}

type JobUnit struct {
	PartitionID uint64
	Target      Target
	ProcessDate time.Time
}

type JobStartRecord struct {
	RunID uint64
	Units []JobUnit
}

type JobResult struct {
	RunID                                                  uint64
	Status                                                 string
	TotalPartitions, CompletedPartitions, FailedPartitions uint32
	FetchedPages                                           uint32
	ParsedRows, UniqueRCNOCount, NewRCNOCount              uint64
	Units                                                  []Result
}

type Starter interface {
	StartWebListBackfill(context.Context, StartParams) (StartRecord, error)
}

type JobStarter interface {
	StartWebListJob(context.Context, JobStartParams) (JobStartRecord, error)
}

type Result struct {
	RunID, PartitionID            uint64
	ItemName, ItemCode            string
	ProcessDate                   time.Time
	RunStatus, PartitionStatus    string
	ExpectedTotal, ParsedRows     uint64
	ExpectedPages, FetchedPages   uint32
	UniqueRCNOCount, NewRCNOCount uint64
}

type Service struct {
	store       Store
	source      ListSource
	targets     map[string]Target
	pageSize    int
	location    *time.Location
	webBaseURL  string
	proxyMode   string
	proxyLabel  string
	now         func() time.Time
	rateMu      sync.Mutex
	nextFetch   time.Time
	fetchGap    time.Duration
	qps         float64
	processID   string
	maxAttempts int
	retryDelays []time.Duration
}

type configSnapshot struct {
	SourceKind  string         `json:"source_kind"`
	Item        snapshotTarget `json:"item"`
	ProcessDate string         `json:"process_date"`
	PageSize    uint32         `json:"page_size"`
	WebBaseURL  string         `json:"web_base_url"`
	ProxyMode   string         `json:"proxy_mode"`
	ProxyLabel  string         `json:"proxy_label"`
}

type snapshotTarget struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

func NewService(store Store, source ListSource, options Options) (*Service, error) {
	if store == nil || source == nil {
		return nil, fmt.Errorf("웹 목록 backfill store와 source가 필요합니다")
	}
	if options.Location == nil {
		return nil, fmt.Errorf("웹 목록 backfill timezone이 필요합니다")
	}
	if options.PageSize < 1 || options.PageSize > MaximumPageSize {
		return nil, fmt.Errorf("웹 목록 page size는 1 이상 %d 이하여야 합니다", MaximumPageSize)
	}
	if len(options.Targets) == 0 {
		return nil, fmt.Errorf("웹 목록 backfill 대상 품목이 필요합니다")
	}
	webBaseURL, err := canonicalWebBaseURL(options.WebBaseURL)
	if err != nil {
		return nil, err
	}
	targets := make(map[string]Target, len(options.Targets))
	for _, target := range options.Targets {
		if strings.TrimSpace(target.Name) == "" || strings.TrimSpace(target.Code) == "" {
			return nil, fmt.Errorf("웹 목록 backfill 대상 품목의 이름과 코드가 필요합니다")
		}
		if _, duplicated := targets[target.Name]; duplicated {
			return nil, fmt.Errorf("웹 목록 backfill 대상 품목 %q이 중복되었습니다", target.Name)
		}
		targets[target.Name] = target
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	var fetchGap time.Duration
	if options.QPS > 0 {
		fetchGap = time.Duration(float64(time.Second) / options.QPS)
		if fetchGap < time.Nanosecond {
			fetchGap = time.Nanosecond
		}
	}
	processID := strings.TrimSpace(options.ProcessID)
	if processID == "" {
		rawID := make([]byte, 8)
		if _, err := rand.Read(rawID); err != nil {
			return nil, fmt.Errorf("worker process ID 생성 실패: %w", err)
		}
		processID = hex.EncodeToString(rawID)
	} else if decoded, err := hex.DecodeString(processID); err != nil || len(decoded) != 8 {
		return nil, fmt.Errorf("worker process ID는 16자리 hexadecimal이어야 합니다")
	}
	maxAttempts := options.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 3
	}
	retryDelays := append([]time.Duration(nil), options.RetryDelays...)
	if len(retryDelays) == 0 {
		retryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
	}
	return &Service{
		store: store, source: source, targets: targets, pageSize: options.PageSize,
		location: options.Location, webBaseURL: webBaseURL, proxyMode: options.ProxyMode,
		proxyLabel: options.ProxyLabel, now: now, fetchGap: fetchGap, qps: options.QPS,
		processID: processID, maxAttempts: maxAttempts, retryDelays: retryDelays,
	}, nil
}

func canonicalWebBaseURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if strings.ContainsAny(value, "?#") {
		return "", fmt.Errorf("웹 base URL에는 query 또는 fragment를 사용할 수 없습니다")
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("웹 base URL은 absolute http/https URL이어야 합니다")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("웹 base URL은 http 또는 https scheme이어야 합니다")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("웹 base URL에는 userinfo를 사용할 수 없습니다")
	}
	parsed.Scheme, parsed.Host = scheme, strings.ToLower(parsed.Host)
	return parsed.String(), nil
}

func (s *Service) Execute(ctx context.Context, command Command) (Result, error) {
	target, processDate, _, err := s.prepare(command)
	if err != nil {
		return Result{}, err
	}
	job, err := s.ExecuteJob(ctx, JobCommand{
		ItemNames: []string{target.Name},
		FromDate:  processDate.Format(time.DateOnly), ToDate: processDate.Format(time.DateOnly),
		Workers: 1,
	})
	if len(job.Units) == 0 {
		return Result{RunID: job.RunID, ItemName: target.Name, ItemCode: target.Code, ProcessDate: processDate, RunStatus: job.Status}, err
	}
	result := job.Units[0]
	result.ItemName, result.ItemCode, result.ProcessDate = target.Name, target.Code, processDate
	return result, err
}

func (s *Service) ExecuteJob(ctx context.Context, command JobCommand) (JobResult, error) {
	if err := ctx.Err(); err != nil {
		return JobResult{}, err
	}
	command.Workers = max(1, command.Workers)
	targets, from, to, snapshot, err := s.prepareJob(command)
	if err != nil {
		return JobResult{}, err
	}
	partitions := make([]JobPartition, 0, len(targets)*inclusiveDays(from, to))
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		for _, target := range targets {
			partitions = append(partitions, JobPartition{
				ItemName: target.Name, ItemCode: target.Code, ProcessDate: date,
			})
		}
	}
	record, err := s.store.StartWebListJob(ctx, JobStartParams{
		FromDate: from, ToDate: to, StartedAt: s.now(), PageSize: uint32(s.pageSize),
		ConfigJSON: snapshot, Partitions: partitions,
	})
	if err != nil {
		return JobResult{}, err
	}
	if command.OnStarted != nil {
		command.OnStarted(record.RunID)
	}

	workers := max(1, command.Workers)
	resultCh := make(chan Result, len(record.Units))
	errCh := make(chan error, workers)
	results := make([]Result, 0, len(record.Units))
	var executionErr error
	resultsDone := make(chan struct{})
	errorsDone := make(chan struct{})
	go func() {
		defer close(resultsDone)
		for result := range resultCh {
			results = append(results, result)
		}
	}()
	go func() {
		defer close(errorsDone)
		for workerErr := range errCh {
			executionErr = errors.Join(executionErr, workerErr)
		}
	}()
	var workerGroup sync.WaitGroup
	for workerIndex := 1; workerIndex <= workers; workerIndex++ {
		workerGroup.Add(1)
		go func(index int) {
			defer workerGroup.Done()
			if workerErr := s.runWorker(ctx, record.RunID, index, resultCh); workerErr != nil {
				errCh <- workerErr
			}
		}(workerIndex)
	}
	workerGroup.Wait()
	close(resultCh)
	close(errCh)
	<-resultsDone
	<-errorsDone
	finalizeCtx, finalizeCancel := detachedFailureContext(ctx)
	status, _, finalizeErr := s.store.FinalizeJob(finalizeCtx, FinalizeJobParams{
		RunID: record.RunID, Cancelled: ctx.Err() != nil, FinishedAt: s.now(),
	})
	finalizeCancel()
	for index := range results {
		results[index].RunStatus = status
	}
	aggregateCtx, aggregateCancel := detachedFailureContext(ctx)
	jobResult, aggregateErr := s.store.LoadJobResult(aggregateCtx, record.RunID)
	aggregateCancel()
	if aggregateErr != nil {
		jobResult = aggregateLocalJobResult(record.RunID, status, record.Units, results)
	}
	jobResult.Units = results
	return jobResult,
		errors.Join(executionErr, finalizeErr, aggregateErr)
}

func aggregateLocalJobResult(
	runID uint64,
	status string,
	units []JobUnit,
	results []Result,
) JobResult {
	jobResult := JobResult{
		RunID: runID, Status: status, TotalPartitions: uint32(len(units)), Units: results,
	}
	for _, result := range results {
		if result.PartitionStatus == PartitionStatusDone {
			jobResult.CompletedPartitions++
		} else if result.PartitionStatus == PartitionStatusDirty ||
			result.PartitionStatus == PartitionStatusFailed {
			jobResult.FailedPartitions++
		}
		jobResult.FetchedPages += result.FetchedPages
		jobResult.ParsedRows += result.ParsedRows
		jobResult.UniqueRCNOCount += result.UniqueRCNOCount
		jobResult.NewRCNOCount += result.NewRCNOCount
	}
	if terminalMissing := jobResult.TotalPartitions -
		jobResult.CompletedPartitions - jobResult.FailedPartitions; isTerminalRunStatus(status) {
		jobResult.FailedPartitions += terminalMissing
	}
	return jobResult
}

func (s *Service) waitForFetchSlot(ctx context.Context) error {
	if s.fetchGap == 0 {
		return nil
	}
	s.rateMu.Lock()
	now := time.Now()
	slot := now
	if s.nextFetch.After(slot) {
		slot = s.nextFetch
	}
	s.nextFetch = slot.Add(s.fetchGap)
	s.rateMu.Unlock()

	timer := time.NewTimer(time.Until(slot))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func detachedFailureContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), failureCleanupTimeout)
}

func (s *Service) prepare(command Command) (Target, time.Time, []byte, error) {
	itemName := strings.TrimSpace(command.ItemName)
	target, ok := s.targets[itemName]
	if !ok {
		return Target{}, time.Time{}, nil, fmt.Errorf("설정에 등록되지 않은 대상 품목입니다: %q", itemName)
	}
	processDate, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(command.ProcessDate), s.location)
	if err != nil {
		return Target{}, time.Time{}, nil, fmt.Errorf("처리일은 YYYY-MM-DD 형식의 유효한 날짜여야 합니다: %w", err)
	}
	snapshot, err := json.Marshal(configSnapshot{
		SourceKind: "WEB_LIST", Item: snapshotTarget{Name: target.Name, Code: target.Code},
		ProcessDate: processDate.Format(time.DateOnly), PageSize: uint32(s.pageSize),
		WebBaseURL: s.webBaseURL, ProxyMode: s.proxyMode, ProxyLabel: s.proxyLabel,
	})
	if err != nil {
		return Target{}, time.Time{}, nil, fmt.Errorf("웹 목록 backfill 설정 snapshot 생성 실패: %w", err)
	}
	return target, processDate, snapshot, nil
}

func (s *Service) prepareJob(command JobCommand) ([]Target, time.Time, time.Time, []byte, error) {
	if len(command.ItemNames) == 0 {
		return nil, time.Time{}, time.Time{}, nil, fmt.Errorf("수집할 품목을 하나 이상 선택해야 합니다")
	}
	selected := make([]Target, 0, len(command.ItemNames))
	seen := make(map[string]struct{}, len(command.ItemNames))
	for _, rawName := range command.ItemNames {
		name := strings.TrimSpace(rawName)
		target, ok := s.targets[name]
		if !ok {
			return nil, time.Time{}, time.Time{}, nil, fmt.Errorf("설정에 등록되지 않은 대상 품목입니다: %q", name)
		}
		if _, duplicated := seen[name]; duplicated {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, target)
	}
	from, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(command.FromDate), s.location)
	if err != nil {
		return nil, time.Time{}, time.Time{}, nil, fmt.Errorf("시작일은 YYYY-MM-DD 형식의 유효한 날짜여야 합니다: %w", err)
	}
	to, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(command.ToDate), s.location)
	if err != nil {
		return nil, time.Time{}, time.Time{}, nil, fmt.Errorf("종료일은 YYYY-MM-DD 형식의 유효한 날짜여야 합니다: %w", err)
	}
	if from.After(to) {
		return nil, time.Time{}, time.Time{}, nil, fmt.Errorf("시작일은 종료일보다 늦을 수 없습니다")
	}
	snapshot, err := json.Marshal(map[string]any{
		"source_kind": "WEB_LIST",
		"items":       selected,
		"from_date":   from.Format(time.DateOnly),
		"to_date":     to.Format(time.DateOnly),
		"page_size":   s.pageSize,
		"workers":     command.Workers,
		"qps":         s.qps,
		"retry": map[string]any{
			"max_attempts": s.maxAttempts,
			"delays":       durationStrings(s.retryDelays),
		},
		"web_base_url": s.webBaseURL,
		"proxy_mode":   s.proxyMode,
		"proxy_label":  s.proxyLabel,
	})
	if err != nil {
		return nil, time.Time{}, time.Time{}, nil, fmt.Errorf("웹 목록 Job 설정 snapshot 생성 실패: %w", err)
	}
	return selected, from, to, snapshot, nil
}

func durationStrings(values []time.Duration) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.String()
	}
	return result
}

func inclusiveDays(from, to time.Time) int {
	return int(to.Sub(from).Hours()/24) + 1
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
