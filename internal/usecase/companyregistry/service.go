package companyregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const defaultMatcherVersion = "importer-license-match-v1"

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
	if opts.MatcherVersion == "" {
		opts.MatcherVersion = defaultMatcherVersion
	}
	return &Service{store: store, client: client, opts: opts}, nil
}

func (s *Service) Collect(ctx context.Context) (Summary, error) {
	startedAt := s.opts.Now()
	configJSON, err := json.Marshal(map[string]any{
		"services": Services, "page_size": s.opts.PageSize, "max_pages": s.opts.MaxPages,
		"max_requests_per_run": s.opts.MaxRequests,
		"qps":                  s.opts.QPS, "max_attempts": s.opts.MaxAttempts,
		"matcher_version": s.opts.MatcherVersion,
	})
	if err != nil {
		return Summary{}, fmt.Errorf("업체 원장 설정 snapshot 생성 실패: %w", err)
	}
	runID, err := s.store.StartCollection(ctx, startedAt, configJSON)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{CollectionID: runID, Services: make(map[ServiceID]ServiceSummary), Matches: make(map[MatchStatus]uint64)}
	for _, serviceID := range Services {
		serviceSummary, collectErr := s.collectService(ctx, runID, serviceID)
		if collectErr != nil {
			return Summary{}, s.failCollection(ctx, runID, collectErr)
		}
		summary.Services[serviceID] = serviceSummary
	}
	if err := s.matchImporters(ctx, runID, &summary); err != nil {
		return Summary{}, s.failCollection(ctx, runID, err)
	}
	if err := s.store.CompleteCollection(ctx, runID, summary, s.opts.Now()); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func (s *Service) collectService(ctx context.Context, runID uint64, serviceID ServiceID) (ServiceSummary, error) {
	summary := ServiceSummary{}
	for pageNo := 1; pageNo <= s.opts.MaxPages; pageNo++ {
		request := PageRequest{Service: serviceID, StartIndex: (pageNo-1)*s.opts.PageSize + 1, EndIndex: pageNo * s.opts.PageSize}
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

func (s *Service) matchImporters(ctx context.Context, runID uint64, summary *Summary) error {
	importers, err := s.store.ListLatestImporters(ctx)
	if err != nil {
		return err
	}
	candidates, err := s.store.ListC001Candidates(ctx, runID)
	if err != nil {
		return err
	}
	exact, normalized := candidateIndexes(candidates)
	evidence := make([]MatchEvidence, 0, len(importers))
	for _, importer := range importers {
		match := evaluateImporter(importer, exact, normalized, s.opts.MatcherVersion, s.opts.Now())
		evidence = append(evidence, match)
		summary.Matches[match.Status]++
	}
	return s.store.SaveMatchEvidence(ctx, runID, evidence)
}

func candidateIndexes(candidates []LicenseCandidate) (map[string][]LicenseCandidate, map[string][]LicenseCandidate) {
	exact := make(map[string][]LicenseCandidate)
	normalized := make(map[string][]LicenseCandidate)
	for _, candidate := range candidates {
		exact[rawNameKey(candidate.BusinessName)] = append(exact[rawNameKey(candidate.BusinessName)], candidate)
		normalized[companyNameKey(candidate.BusinessName)] = append(normalized[companyNameKey(candidate.BusinessName)], candidate)
	}
	return exact, normalized
}

func evaluateImporter(importer Importer, exact, normalized map[string][]LicenseCandidate, version string, now time.Time) MatchEvidence {
	candidates := uniqueCandidates(exact[rawNameKey(importer.Name)])
	status := MatchExactName
	if len(candidates) == 0 {
		candidates = uniqueCandidates(normalized[companyNameKey(importer.Name)])
		status = MatchNormalizedName
	}
	if len(candidates) == 0 {
		status = MatchUnresolved
	} else if len(candidates) > 1 {
		status = MatchAmbiguous
	}
	payload := map[string]any{"candidate_raw_ids": candidateIDs(candidates), "source_name": importer.Name}
	evidenceJSON, _ := json.Marshal(payload)
	result := MatchEvidence{
		SourceItemID: importer.SourceItemID, RCNO: importer.RCNO, ImporterName: importer.Name,
		ImporterMatchKey: companyNameKey(importer.Name), Status: status, CandidateCount: len(candidates),
		MatcherVersion: version, EvidenceJSON: evidenceJSON, MatchedAt: now,
	}
	if len(candidates) == 1 {
		candidate := candidates[0]
		result.C001RawID = &candidate.RawID
		result.LicenseNo = candidate.LicenseNo
		result.BusinessName = candidate.BusinessName
		result.Address = candidate.Address
	}
	return result
}

func uniqueCandidates(values []LicenseCandidate) []LicenseCandidate {
	seen := make(map[uint64]LicenseCandidate, len(values))
	for _, value := range values {
		seen[value.RawID] = value
	}
	result := make([]LicenseCandidate, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RawID < result[j].RawID })
	return result
}

func candidateIDs(values []LicenseCandidate) []uint64 {
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		ids = append(ids, value.RawID)
	}
	return ids
}

func rawNameKey(value string) string { return strings.Join(strings.Fields(value), " ") }

func companyNameKey(value string) string {
	value = strings.ToLower(rawNameKey(value))
	replacer := strings.NewReplacer("㈜", "", "(주)", "", "주식회사", "", "유한회사", "", " ", "", "(", "", ")", "")
	return replacer.Replace(value)
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
