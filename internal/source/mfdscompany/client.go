package mfdscompany

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func NewScraper(options Options) (*Scraper, error) {
	if options.BaseURL == nil || !options.BaseURL.IsAbs() || options.BaseURL.Host == "" ||
		(options.BaseURL.Scheme != "http" && options.BaseURL.Scheme != "https") ||
		options.BaseURL.User != nil || options.BaseURL.RawQuery != "" || options.BaseURL.Fragment != "" {
		return nil, errors.New("base URL은 userinfo, query, fragment가 없는 절대 HTTP(S) URL이어야 합니다")
	}

	client := &http.Client{Timeout: 20 * time.Second}
	if options.HTTPClient != nil {
		copied := *options.HTTPClient
		client = &copied
		if client.Timeout <= 0 {
			client.Timeout = 20 * time.Second
		}
	}
	client.Jar = nil
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	userAgent := strings.TrimSpace(options.UserAgent)
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	maxBodyBytes := options.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = DefaultMaxBodySize
	}
	if maxBodyBytes < 1 {
		return nil, errors.New("최대 응답 본문 크기는 0보다 커야 합니다")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Scraper{
		baseURL:      *options.BaseURL,
		httpClient:   client,
		userAgent:    userAgent,
		maxBodyBytes: maxBodyBytes,
		now:          now,
	}, nil
}

func (s *Scraper) Search(ctx context.Context, request SearchRequest) (SearchPage, error) {
	if err := validateSearchRequest(request); err != nil {
		return SearchPage{}, err
	}
	query := url.Values{
		"page":         {strconv.Itoa(request.Page)},
		"limit":        {strconv.Itoa(request.Limit)},
		"totalCnt":     {"0"},
		"indutyCd":     {request.IndustryCode},
		"bsnOfcName":   {request.BusinessName},
		"selBsshSttus": {request.OperatingState},
	}
	body, source, err := s.fetchHTML(ctx, ListPath, query)
	if err != nil {
		return SearchPage{}, err
	}
	page, err := parseSearchPage(body, request)
	if err != nil {
		return SearchPage{}, err
	}
	page.Source = source
	return page, nil
}

func (s *Scraper) Detail(ctx context.Context, internalBusinessCode string) (BusinessDetail, error) {
	internalBusinessCode = strings.TrimSpace(internalBusinessCode)
	if internalBusinessCode == "" {
		return BusinessDetail{}, errors.New("내부업소코드가 필요합니다")
	}
	query := url.Values{"srchBsshCode": {internalBusinessCode}}
	body, source, err := s.fetchHTML(ctx, DetailPath, query)
	if err != nil {
		return BusinessDetail{}, err
	}
	detail, err := parseBusinessDetail(body, internalBusinessCode)
	if err != nil {
		return BusinessDetail{}, err
	}
	detail.Source = source
	return detail, nil
}

func (s *Scraper) SearchGallery(ctx context.Context, request GallerySearchRequest) (GalleryPage, error) {
	if err := validateGallerySearchRequest(request); err != nil {
		return GalleryPage{}, err
	}
	query := url.Values{
		"page":          {strconv.Itoa(request.Page)},
		"limit":         {strconv.Itoa(request.Limit)},
		"totalCnt":      {"0"},
		"bsshNm":        {request.BusinessName},
		"rceptBeginDtm": {request.FromDate.Format(time.DateOnly)},
		"rceptEndDtm":   {request.ToDate.Format(time.DateOnly)},
	}
	body, source, err := s.fetchHTML(ctx, GalleryListPath, query)
	if err != nil {
		return GalleryPage{}, err
	}
	page, err := parseGalleryPage(body, request)
	if err != nil {
		return GalleryPage{}, err
	}
	page.Source = source
	return page, nil
}

func (s *Scraper) GalleryDetail(ctx context.Context, productCode string) (GalleryDetail, error) {
	productCode = strings.TrimSpace(productCode)
	if productCode == "" {
		return GalleryDetail{}, errors.New("갤러리 제품코드가 필요합니다")
	}
	body, source, err := s.fetchHTML(ctx, GalleryDetailPath, url.Values{
		"prductCd": {productCode},
		"appPopup": {"Y"},
	})
	if err != nil {
		return GalleryDetail{}, err
	}
	detail, err := parseGalleryDetail(body, productCode)
	if err != nil {
		return GalleryDetail{}, err
	}
	detail.Source = source
	return detail, nil
}

func (s *Scraper) fetchHTML(ctx context.Context, path string, query url.Values) ([]byte, SourceMetadata, error) {
	target := s.baseURL.ResolveReference(&url.URL{Path: path})
	target.RawQuery = query.Encode()
	startedAt := s.now()
	source := SourceMetadata{
		Endpoint:    path,
		RequestURL:  target.String(),
		RetrievedAt: startedAt,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, source, fmt.Errorf("요청 생성: %w", err)
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", s.userAgent)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, source, fmt.Errorf("HTTP 요청: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, s.maxBodyBytes+1))
	closeErr := response.Body.Close()
	source.HTTPStatus = response.StatusCode
	source.ContentType = response.Header.Get("Content-Type")
	source.BodyBytes = int64(len(body))
	digest := sha256.Sum256(body)
	source.BodySHA256 = hex.EncodeToString(digest[:])
	if readErr != nil {
		return body, source, fmt.Errorf("응답 본문 읽기: %w", readErr)
	}
	if closeErr != nil {
		return body, source, fmt.Errorf("응답 본문 닫기: %w", closeErr)
	}
	if int64(len(body)) > s.maxBodyBytes {
		return body, source, fmt.Errorf("응답 본문이 최대 %d bytes를 초과했습니다", s.maxBodyBytes)
	}
	if response.StatusCode != http.StatusOK {
		return body, source, &HTTPStatusError{
			StatusCode: response.StatusCode,
			Location:   response.Header.Get("Location"),
		}
	}
	mediaType, _, err := mime.ParseMediaType(source.ContentType)
	if err != nil || !strings.EqualFold(mediaType, "text/html") {
		return body, source, fmt.Errorf("Content-Type이 text/html이 아닙니다: %q", source.ContentType)
	}
	if len(body) == 0 {
		return body, source, errors.New("HTML 응답 본문이 비어 있습니다")
	}
	return body, source, nil
}

func validateSearchRequest(request SearchRequest) error {
	if request.Page < 1 {
		return errors.New("page는 1 이상이어야 합니다")
	}
	if request.Limit < 1 || request.Limit > MaximumPageSize {
		return fmt.Errorf("limit은 1 이상 %d 이하여야 합니다", MaximumPageSize)
	}
	if strings.TrimSpace(request.IndustryCode) == "" {
		return errors.New("영업종류 코드가 필요합니다")
	}
	if strings.TrimSpace(request.IndustryCode) != request.IndustryCode {
		return errors.New("영업종류 코드는 앞뒤 공백을 포함할 수 없습니다")
	}
	if strings.TrimSpace(request.BusinessName) != request.BusinessName ||
		strings.TrimSpace(request.OperatingState) != request.OperatingState {
		return errors.New("검색 조건은 앞뒤 공백을 포함할 수 없습니다")
	}
	return nil
}

func validateGallerySearchRequest(request GallerySearchRequest) error {
	if request.Page < 1 {
		return errors.New("page는 1 이상이어야 합니다")
	}
	if request.Limit < 1 || request.Limit > MaximumPageSize {
		return fmt.Errorf("limit은 1 이상 %d 이하여야 합니다", MaximumPageSize)
	}
	if strings.TrimSpace(request.BusinessName) == "" || strings.TrimSpace(request.BusinessName) != request.BusinessName {
		return errors.New("갤러리 수입업체명이 필요하며 앞뒤 공백을 포함할 수 없습니다")
	}
	if request.FromDate.IsZero() || request.ToDate.IsZero() || request.FromDate.After(request.ToDate) {
		return errors.New("갤러리 조회 기간이 유효하지 않습니다")
	}
	return nil
}
