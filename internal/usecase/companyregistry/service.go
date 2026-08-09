package companyregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Service struct {
	store         Store
	client        Client
	opts          Options
	nextRequestAt time.Time
	requestCount  int
}

func NewService(store Store, client Client, opts Options) (*Service, error) {
	if store == nil || client == nil {
		return nil, errors.New("업체 원장 store와 client가 필요합니다")
	}
	if opts.PageSize <= 0 || opts.PageSize > 1000 || opts.MaxPages <= 0 || opts.MaxRequests <= 0 ||
		opts.QPS <= 0 || opts.MaxAttempts <= 0 {
		return nil, errors.New("업체 원장 page size, max pages, max requests, QPS, max attempts 설정이 유효하지 않습니다")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Location == nil {
		opts.Location = time.UTC
	}
	return &Service{store: store, client: client, opts: opts}, nil
}

func (s *Service) Collect(ctx context.Context) (Summary, error) {
	startedAt := s.opts.Now()
	window, err := s.syncWindow(ctx, startedAt)
	if err != nil {
		return Summary{}, err
	}
	configJSON, err := json.Marshal(map[string]any{
		"services": Services, "page_size": s.opts.PageSize, "max_pages": s.opts.MaxPages,
		"max_requests_per_run": s.opts.MaxRequests,
		"qps":                  s.opts.QPS, "max_attempts": s.opts.MaxAttempts,
		"change_since_date": window.Since.Format("2006-01-02"),
		"sync_through_date": window.Through.Format("2006-01-02"),
	})
	if err != nil {
		return Summary{}, fmt.Errorf("업체 원장 설정 snapshot 생성 실패: %w", err)
	}
	runID, err := s.store.StartCollection(ctx, startedAt, window, configJSON)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{CollectionID: runID, Since: window.Since, Through: window.Through, Services: make(map[ServiceID]ServiceSummary)}
	for _, serviceID := range Services {
		serviceSummary, collectErr := s.collectService(ctx, runID, serviceID, window.Since)
		if collectErr != nil {
			return Summary{}, s.failCollection(ctx, runID, collectErr)
		}
		summary.Services[serviceID] = serviceSummary
	}
	if err := s.store.CompleteCollection(ctx, runID, summary, s.opts.Now()); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func (s *Service) syncWindow(ctx context.Context, now time.Time) (SyncWindow, error) {
	through := dateInLocation(now, s.opts.Location)
	since := s.opts.Since
	if since == nil {
		latest, err := s.store.LatestCompletedThrough(ctx)
		if err != nil {
			return SyncWindow{}, err
		}
		if latest == nil {
			return SyncWindow{}, errors.New("첫 업체 공식정보 동기화에는 --since YYYY-MM-DD가 필요합니다")
		}
		since = latest
	}
	from := dateInLocation(*since, s.opts.Location)
	if from.After(through) {
		return SyncWindow{}, fmt.Errorf("변경일자 %s는 동기화 기준일 %s 이후일 수 없습니다", from.Format("2006-01-02"), through.Format("2006-01-02"))
	}
	return SyncWindow{Since: from, Through: through}, nil
}

func dateInLocation(value time.Time, location *time.Location) time.Time {
	localized := value.In(location)
	return time.Date(localized.Year(), localized.Month(), localized.Day(), 0, 0, 0, 0, location)
}

func (s *Service) collectService(ctx context.Context, runID uint64, serviceID ServiceID, since time.Time) (ServiceSummary, error) {
	summary := ServiceSummary{}
	for pageNo := 1; pageNo <= s.opts.MaxPages; pageNo++ {
		request := PageRequest{Service: serviceID, StartIndex: (pageNo-1)*s.opts.PageSize + 1, EndIndex: pageNo * s.opts.PageSize}
		if serviceID != ServiceI0250 {
			request.Filters = map[string]string{"CHNG_DT": since.Format("20060102")}
		}
		page, err := s.fetchWithRetry(ctx, runID, request)
		if err != nil {
			return ServiceSummary{}, fmt.Errorf("%s %d페이지 수집 실패: %w", serviceID, pageNo, err)
		}
		summary.Fetches++
		summary.Rows += uint64(len(page.Rows))
		summary.ReportedTotal = page.TotalCount
		if page.ResultCode == "INFO-200" || len(page.Rows) < s.opts.PageSize {
			return summary, nil
		}
		if serviceID != ServiceI2821 && uint64(page.EndIndex) >= page.TotalCount {
			return summary, nil
		}
	}
	return ServiceSummary{}, fmt.Errorf("%s가 최대 페이지 %d에 도달했습니다", serviceID, s.opts.MaxPages)
}

func (s *Service) fetchWithRetry(ctx context.Context, runID uint64, request PageRequest) (Page, error) {
	var lastErr error
	for attempt := 1; attempt <= s.opts.MaxAttempts; attempt++ {
		if s.requestCount >= s.opts.MaxRequests {
			return Page{}, fmt.Errorf("식품안전나라 run당 요청 제한 %d회에 도달했습니다", s.opts.MaxRequests)
		}
		if err := s.waitForRateLimit(ctx); err != nil {
			return Page{}, err
		}
		s.requestCount++
		request.Attempt = attempt
		page, err := s.client.FetchPage(ctx, request)
		page.Service, page.StartIndex, page.EndIndex, page.Attempt = request.Service, request.StartIndex, request.EndIndex, attempt
		if saveErr := s.store.SavePage(ctx, runID, page, err); saveErr != nil {
			return Page{}, saveErr
		}
		if err == nil {
			return page, nil
		}
		lastErr = err
		var retryable RetryableError
		if !errors.As(err, &retryable) || !retryable.Retryable() || attempt == s.opts.MaxAttempts {
			break
		}
		delay := retryDelay(s.opts.RetryDelays, attempt)
		if err := waitContext(ctx, delay); err != nil {
			return Page{}, err
		}
	}
	return Page{}, lastErr
}

func (s *Service) waitForRateLimit(ctx context.Context) error {
	now := s.opts.Now()
	if now.Before(s.nextRequestAt) {
		if err := waitContext(ctx, s.nextRequestAt.Sub(now)); err != nil {
			return err
		}
		now = s.opts.Now()
	}
	s.nextRequestAt = now.Add(time.Duration(float64(time.Second) / s.opts.QPS))
	return nil
}

func retryDelay(delays []time.Duration, attempt int) time.Duration {
	if len(delays) == 0 {
		return 0
	}
	index := attempt - 1
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return delays[index]
}

func waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Service) failCollection(ctx context.Context, runID uint64, cause error) error {
	return errors.Join(cause, s.store.FailCollection(ctx, runID, cause, s.opts.Now()))
}
