package importerresolution

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
)

type Service struct {
	store    Store
	source   Source
	pageSize int
	delay    time.Duration
	industry string
	state    string
}

type officialCandidate struct {
	result mfdscompany.SearchResult
	source mfdscompany.SourceMetadata
}

func NewService(store Store, source Source, options Options) (*Service, error) {
	if store == nil || source == nil {
		return nil, errors.New("수입사 해소 store와 source가 필요합니다")
	}
	if options.PageSize < 1 || options.PageSize > mfdscompany.MaximumPageSize {
		return nil, fmt.Errorf("수입사 해소 page size는 1 이상 %d 이하여야 합니다", mfdscompany.MaximumPageSize)
	}
	if options.Delay < 0 {
		return nil, errors.New("수입사 해소 요청 간격은 음수일 수 없습니다")
	}
	industry := strings.TrimSpace(options.Industry)
	state := strings.TrimSpace(options.State)
	if industry == "" {
		return nil, errors.New("수입사 해소 업종이 필요합니다")
	}
	return &Service{store: store, source: source, pageSize: options.PageSize, delay: options.Delay, industry: industry, state: state}, nil
}

func (s *Service) SyncJob(ctx context.Context, jobID uint64) (Summary, error) {
	if jobID == 0 {
		return Summary{}, errors.New("수입사 해소 job ID가 필요합니다")
	}
	groups, err := s.store.ListPendingImporterGroups(ctx, jobID)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Groups: len(groups)}
	for _, group := range groups {
		summary.RCNOs += len(group.Records)
		exactImporter, err := s.store.FindExactImporter(ctx, group.BusinessName)
		if err != nil {
			return summary, fmt.Errorf("수입사 %q DB exact 조회 실패: %w", group.BusinessName, err)
		}
		if exactImporter != nil {
			resolutions := make([]ExactResolution, 0, len(group.Records))
			for _, record := range group.Records {
				resolutions = append(resolutions, ExactResolution{
					RCNO: record.RCNO, BusinessName: group.BusinessName,
					ImporterID: exactImporter.ID, SourceObservedAt: exactImporter.SourceObservedAt,
				})
			}
			if err := s.store.SaveExactImporterResolutions(ctx, resolutions); err != nil {
				return summary, err
			}
			summary.Resolved += len(resolutions)
			continue
		}
		resolutions, err := s.resolveGroup(ctx, group)
		if err != nil {
			return summary, fmt.Errorf("수입사 %q 해소 실패: %w", group.BusinessName, err)
		}
		if err := s.store.SaveImporterResolutions(ctx, resolutions); err != nil {
			return summary, err
		}
		summary.Resolved += len(resolutions)
		summary.Unresolved += len(group.Records) - len(resolutions)
	}
	return summary, nil
}

func (s *Service) resolveGroup(ctx context.Context, group PendingGroup) ([]Resolution, error) {
	candidates, err := s.searchExactCandidates(ctx, group.BusinessName)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	if len(candidates) == 1 {
		detail, err := s.businessDetail(ctx, candidates[0].result)
		if err != nil {
			return nil, err
		}
		result := make([]Resolution, 0, len(group.Records))
		for _, record := range group.Records {
			result = append(result, Resolution{
				RCNO: record.RCNO, BusinessName: group.BusinessName,
				Importer: detail, ListSource: candidates[0].source,
			})
		}
		return result, nil
	}
	return s.resolveFromGallery(ctx, group, candidates)
}

func (s *Service) searchExactCandidates(ctx context.Context, name string) ([]officialCandidate, error) {
	var candidates []officialCandidate
	seenCodes := make(map[string]struct{})
	var totalSnapshot *int
	for pageNo := 1; ; pageNo++ {
		if err := s.wait(ctx); err != nil {
			return nil, err
		}
		page, err := s.source.Search(ctx, mfdscompany.SearchRequest{
			Page: pageNo, Limit: s.pageSize, TotalSnapshot: totalSnapshot, IndustryCode: s.industry,
			BusinessName: name, OperatingState: s.state,
		})
		if err != nil {
			return nil, err
		}
		for _, candidate := range page.Rows {
			code := strings.TrimSpace(candidate.InternalBusinessCode)
			if strings.TrimSpace(candidate.BusinessName) == name && code != "" {
				if _, exists := seenCodes[code]; exists {
					continue
				}
				seenCodes[code] = struct{}{}
				candidates = append(candidates, officialCandidate{result: candidate, source: page.Source})
			}
		}
		if pageNo == 1 {
			snapshot := page.Total
			totalSnapshot = &snapshot
		}
		if page.TotalPages == 0 || pageNo >= page.TotalPages {
			return candidates, nil
		}
	}
}

func (s *Service) resolveFromGallery(ctx context.Context, group PendingGroup, candidates []officialCandidate) ([]Resolution, error) {
	candidateByCode := make(map[string]officialCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByCode[candidate.result.InternalBusinessCode] = candidate
	}
	type recordsByDate struct {
		date    time.Time
		records []PendingRecord
	}
	dated := make(map[string]*recordsByDate)
	for _, record := range group.Records {
		if record.ProcessDate.IsZero() {
			continue
		}
		key := record.ProcessDate.Format(time.DateOnly)
		if dated[key] == nil {
			dated[key] = &recordsByDate{date: record.ProcessDate}
		}
		dated[key].records = append(dated[key].records, record)
	}
	dates := make([]string, 0, len(dated))
	for key := range dated {
		dates = append(dates, key)
	}
	sort.Strings(dates)

	resolved := make(map[string]Resolution, len(group.Records))
	detailByCode := make(map[string]mfdscompany.BusinessDetail)
	for _, key := range dates {
		dateGroup := dated[key]
		dateResolutions, err := s.resolveDateFromGallery(
			ctx, group.BusinessName, dateGroup.date, dateGroup.records, candidateByCode, detailByCode,
		)
		if err != nil {
			return nil, err
		}
		for _, resolution := range dateResolutions {
			if previous, exists := resolved[resolution.RCNO]; exists &&
				previous.Importer.InternalBusinessCode != resolution.Importer.InternalBusinessCode {
				return nil, fmt.Errorf("RCNO %s의 공식 업소코드가 서로 다릅니다", resolution.RCNO)
			}
			resolved[resolution.RCNO] = resolution
		}
	}
	result := make([]Resolution, 0, len(resolved))
	for _, record := range group.Records {
		if resolution, exists := resolved[record.RCNO]; exists {
			result = append(result, resolution)
		}
	}
	return result, nil
}

func (s *Service) resolveDateFromGallery(
	ctx context.Context,
	businessName string,
	processDate time.Time,
	records []PendingRecord,
	candidateByCode map[string]officialCandidate,
	detailByCode map[string]mfdscompany.BusinessDetail,
) ([]Resolution, error) {
	wanted := make(map[string]struct{}, len(records))
	for _, record := range records {
		wanted[record.RCNO] = struct{}{}
	}
	resolved := make(map[string]Resolution, len(records))
	for pageNo := 1; ; pageNo++ {
		if err := s.wait(ctx); err != nil {
			return nil, err
		}
		page, err := s.source.SearchGallery(ctx, mfdscompany.GallerySearchRequest{
			Page: pageNo, Limit: s.pageSize, BusinessName: businessName,
			FromDate: processDate, ToDate: processDate,
		})
		if err != nil {
			return nil, err
		}
		for _, product := range page.Products {
			if err := s.wait(ctx); err != nil {
				return nil, err
			}
			gallery, err := s.source.GalleryDetail(ctx, product.ProductCode)
			if err != nil {
				return nil, err
			}
			if _, exists := wanted[gallery.RCNO]; !exists || strings.TrimSpace(gallery.BusinessName) != businessName {
				continue
			}
			candidate, exists := candidateByCode[gallery.InternalBusinessCode]
			if !exists {
				continue
			}
			detail, exists := detailByCode[candidate.result.InternalBusinessCode]
			if !exists {
				detail, err = s.businessDetail(ctx, candidate.result)
				if err != nil {
					return nil, err
				}
				detailByCode[candidate.result.InternalBusinessCode] = detail
			}
			if previous, exists := resolved[gallery.RCNO]; exists {
				if previous.Importer.InternalBusinessCode != detail.InternalBusinessCode {
					return nil, fmt.Errorf("RCNO %s의 공식 업소코드가 서로 다릅니다", gallery.RCNO)
				}
				continue
			}
			gallerySource := gallery.Source
			resolved[gallery.RCNO] = Resolution{
				RCNO: gallery.RCNO, BusinessName: businessName, Importer: detail,
				ListSource: candidate.source, GallerySource: &gallerySource,
			}
		}
		if len(resolved) == len(wanted) || page.TotalPages == 0 || pageNo >= page.TotalPages {
			break
		}
	}
	result := make([]Resolution, 0, len(resolved))
	for _, record := range records {
		if resolution, exists := resolved[record.RCNO]; exists {
			result = append(result, resolution)
		}
	}
	return result, nil
}

func (s *Service) businessDetail(ctx context.Context, candidate mfdscompany.SearchResult) (mfdscompany.BusinessDetail, error) {
	if err := s.wait(ctx); err != nil {
		return mfdscompany.BusinessDetail{}, err
	}
	detail, err := s.source.Detail(ctx, candidate.InternalBusinessCode)
	if err == nil {
		if strings.TrimSpace(detail.InternalBusinessCode) != strings.TrimSpace(candidate.InternalBusinessCode) ||
			strings.TrimSpace(detail.BusinessName) != strings.TrimSpace(candidate.BusinessName) ||
			(strings.TrimSpace(candidate.LicenseNumber) != "" &&
				strings.TrimSpace(detail.LicenseNumber) != strings.TrimSpace(candidate.LicenseNumber)) {
			return mfdscompany.BusinessDetail{}, errors.New("공식 국내업소 목록과 상세의 식별정보가 일치하지 않습니다")
		}
		return detail, nil
	}
	if !detailUnavailable(err) {
		return mfdscompany.BusinessDetail{}, err
	}
	return mfdscompany.BusinessDetail{
		InternalBusinessCode: candidate.InternalBusinessCode,
		LicenseNumber:        candidate.LicenseNumber, BusinessName: candidate.BusinessName,
		Representative: candidate.Representative, Institution: candidate.Institution,
		Address: candidate.Address, Industry: candidate.Industry, OperatingStatus: "UNKNOWN",
	}, nil
}

func detailUnavailable(err error) bool {
	var statusError *mfdscompany.HTTPStatusError
	if !errors.As(err, &statusError) || statusError.StatusCode != http.StatusFound {
		return false
	}
	location, parseErr := url.Parse(statusError.Location)
	return parseErr == nil && (location.Path == "/error/noAuth" || location.Path == "/error/error")
}

func (s *Service) wait(ctx context.Context) error {
	if s.delay == 0 {
		return nil
	}
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
