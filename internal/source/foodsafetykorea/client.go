package foodsafetykorea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ClientOptions struct {
	APIKey     string
	HTTPClient *http.Client
	Now        func() time.Time
}

type Client struct {
	apiKey     string
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(options ClientOptions) (*Client, error) {
	if strings.TrimSpace(options.APIKey) == "" {
		return nil, adapterError(ErrorKindValidation, "client 생성", "API key가 필요합니다")
	}
	if strings.TrimSpace(options.APIKey) != options.APIKey {
		return nil, adapterError(ErrorKindValidation, "client 생성", "API key는 앞뒤 공백을 포함할 수 없습니다")
	}
	for _, character := range options.APIKey {
		if character < 0x21 || character == 0x7f {
			return nil, adapterError(ErrorKindValidation, "client 생성", "API key에 제어 문자나 공백을 사용할 수 없습니다")
		}
	}

	httpClient := &http.Client{Timeout: DefaultTimeout}
	if options.HTTPClient != nil {
		copied := *options.HTTPClient
		httpClient = &copied
		if httpClient.Timeout <= 0 {
			httpClient.Timeout = DefaultTimeout
		}
	}
	httpClient.Jar = nil
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Client{apiKey: options.APIKey, httpClient: httpClient, now: now}, nil
}

func (c *Client) String() string {
	return fmt.Sprintf("foodsafetykorea.Client{baseURL:%s, apiKey:%s}", APIBaseURL, MaskedAPIKey)
}

func (c *Client) GoString() string {
	return c.String()
}

func (c *Client) FetchC001(ctx context.Context, request PageRequest) (Page[C001Row], error) {
	return fetchPage[C001Row](ctx, c, ServiceC001, request)
}

func (c *Client) FetchI2821(ctx context.Context, request PageRequest) (Page[I2821Row], error) {
	return fetchPage[I2821Row](ctx, c, ServiceI2821, request)
}

func (c *Client) FetchI0250(ctx context.Context, request PageRequest) (Page[I0250Row], error) {
	return fetchPage[I0250Row](ctx, c, ServiceI0250, request)
}

func (c *Client) FetchI0470(ctx context.Context, request PageRequest) (Page[I0470Row], error) {
	return fetchPage[I0470Row](ctx, c, ServiceI0470, request)
}

func fetchPage[T any](
	ctx context.Context,
	client *Client,
	serviceID ServiceID,
	pageRequest PageRequest,
) (page Page[T], err error) {
	if err := validatePageRequest(pageRequest); err != nil {
		return page, err
	}
	if err := validateServiceFilters(serviceID, pageRequest.Filters); err != nil {
		return page, err
	}

	target, maskedTarget := buildTargets(client.apiKey, serviceID, pageRequest)
	maskedURL := redact(client.apiKey, maskedTarget.String())
	page.ServiceID = serviceID
	page.Request = RequestSnapshot{
		Method:     http.MethodGet,
		URL:        maskedURL,
		ServiceID:  serviceID,
		StartIndex: pageRequest.StartIndex,
		EndIndex:   pageRequest.EndIndex,
		Filters:    snapshotFilters(pageRequest.Filters, client.apiKey),
	}
	page.SnapshotJSON, err = json.Marshal(page.Request)
	if err != nil {
		return page, adapterError(ErrorKindValidation, "요청 snapshot 생성", err.Error())
	}
	page.HTTP = HTTPMetadata{
		Method:    http.MethodGet,
		URL:       maskedURL,
		StartedAt: client.now(),
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		finishMetadata(&page.HTTP, client.now())
		return page, safeError(client.apiKey, ErrorKindValidation, "요청 생성", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")

	response, err := client.httpClient.Do(request)
	if err != nil {
		finishMetadata(&page.HTTP, client.now())
		return page, safeError(client.apiKey, ErrorKindNetwork, "HTTP 요청", err)
	}

	page.HTTP.StatusCode = response.StatusCode
	page.HTTP.ContentType = redact(client.apiKey, response.Header.Get("Content-Type"))
	page.HTTP.ResponseHeaders = sanitizedHeaders(response.Header, client.apiKey)
	rawBody, truncated, readErr := readBounded(response.Body)
	closeErr := response.Body.Close()
	page.RawBody = rawBody
	page.HTTP.BodySize = int64(len(rawBody))
	page.HTTP.BodyTruncated = truncated
	finishMetadata(&page.HTTP, client.now())

	if readErr != nil {
		return page, safeError(client.apiKey, ErrorKindNetwork, "응답 본문 읽기", readErr)
	}
	if closeErr != nil {
		return page, safeError(client.apiKey, ErrorKindNetwork, "응답 본문 닫기", closeErr)
	}
	if truncated {
		return page, adapterError(
			ErrorKindBodyLimit,
			"응답 본문 검증",
			fmt.Sprintf("응답 본문이 최대 %d bytes를 초과했습니다", MaxResponseBodySize),
		)
	}
	if response.StatusCode != http.StatusOK {
		return page, adapterError(
			ErrorKindHTTP,
			"HTTP 상태 검증",
			fmt.Sprintf("예상 상태 200, 실제 %d", response.StatusCode),
		)
	}
	if len(rawBody) == 0 {
		return page, adapterError(ErrorKindEmptyBody, "응답 본문 검증", "HTTP 200 응답 본문이 비어 있습니다")
	}

	mediaType, err := responseMediaType(page.HTTP.ContentType)
	if err != nil {
		return page, adapterError(ErrorKindNonJSON, "Content-Type 검증", err.Error())
	}
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
		return page, adapterError(
			ErrorKindHTML,
			"Content-Type 검증",
			fmt.Sprintf("HTTP 200에서 HTML 응답을 받았습니다: %q", mediaType),
		)
	}
	if mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return page, adapterError(
			ErrorKindNonJSON,
			"Content-Type 검증",
			fmt.Sprintf("HTTP 200 응답이 JSON Content-Type이 아닙니다: %q", mediaType),
		)
	}

	wrapper, topLevelResult, err := decodeWrapper(rawBody, serviceID)
	if err != nil {
		kind := ErrorKindContract
		if errors.Is(err, errInvalidJSON) {
			kind = ErrorKindNonJSON
		}
		return page, adapterError(kind, "JSON 계약 검증", err.Error())
	}
	if topLevelResult != nil {
		page.Result = *topLevelResult
		if topLevelResult.Code == "INFO-200" {
			return page, nil
		}
		return page, apiResultError(client.apiKey, *topLevelResult)
	}

	page.TotalCount = wrapper.TotalCount
	page.Result = wrapper.Result
	switch wrapper.Result.Code {
	case "INFO-200":
		return page, nil
	case "INFO-000":
	default:
		return page, apiResultError(client.apiKey, wrapper.Result)
	}
	if wrapper.TotalCount == "" {
		return page, adapterError(ErrorKindContract, "JSON 계약 검증", "total_count 문자열이 비어 있습니다")
	}
	if len(wrapper.Row) == 0 || string(wrapper.Row) == "null" {
		return page, adapterError(ErrorKindContract, "JSON 계약 검증", "INFO-000 응답에 row 배열이 없습니다")
	}

	var rawRows []json.RawMessage
	if err := json.Unmarshal(wrapper.Row, &rawRows); err != nil {
		return page, adapterError(ErrorKindContract, "row 계약 검증", fmt.Sprintf("row가 JSON 배열이 아닙니다: %v", err))
	}
	page.Rows = make([]T, len(rawRows))
	for index := range rawRows {
		if err := json.Unmarshal(rawRows[index], &page.Rows[index]); err != nil {
			return page, adapterError(
				ErrorKindContract,
				"row 계약 검증",
				fmt.Sprintf("row %d decode 실패: %v", index+1, err),
			)
		}
	}
	return page, nil
}

func validatePageRequest(request PageRequest) error {
	if request.StartIndex < 1 {
		return adapterError(ErrorKindValidation, "요청 검증", "start index는 1 이상이어야 합니다")
	}
	if request.EndIndex < request.StartIndex {
		return adapterError(ErrorKindValidation, "요청 검증", "end index는 start index 이상이어야 합니다")
	}
	if request.EndIndex-request.StartIndex+1 > MaxPageSize {
		return adapterError(
			ErrorKindValidation,
			"요청 검증",
			fmt.Sprintf("한 요청의 page 범위는 1 이상 %d 이하여야 합니다", MaxPageSize),
		)
	}
	for key := range request.Filters {
		if !validFilterKey(key) {
			return adapterError(
				ErrorKindValidation,
				"요청 검증",
				fmt.Sprintf("filter key %q는 영문 대문자, 숫자, underscore만 사용할 수 있습니다", key),
			)
		}
	}
	return nil
}

func validFilterKey(key string) bool {
	if key == "" {
		return false
	}
	for _, character := range key {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func buildTargets(apiKey string, serviceID ServiceID, request PageRequest) (*url.URL, *url.URL) {
	actualPath, actualRawPath := buildPath(apiKey, serviceID, request)
	maskedPath, maskedRawPath := buildPath(MaskedAPIKey, serviceID, request)
	return &url.URL{
			Scheme:  "https",
			Host:    "openapi.foodsafetykorea.go.kr",
			Path:    actualPath,
			RawPath: actualRawPath,
		}, &url.URL{
			Scheme:  "https",
			Host:    "openapi.foodsafetykorea.go.kr",
			Path:    maskedPath,
			RawPath: maskedRawPath,
		}
}

func buildPath(apiKey string, serviceID ServiceID, request PageRequest) (string, string) {
	decodedSegments := []string{
		"api",
		apiKey,
		string(serviceID),
		"json",
		strconv.Itoa(request.StartIndex),
		strconv.Itoa(request.EndIndex),
	}
	encodedSegments := []string{
		"api",
		url.PathEscape(apiKey),
		url.PathEscape(string(serviceID)),
		"json",
		strconv.Itoa(request.StartIndex),
		strconv.Itoa(request.EndIndex),
	}

	if len(request.Filters) > 0 {
		keys := make([]string, 0, len(request.Filters))
		for key := range request.Filters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		decodedFilters := make([]string, 0, len(keys))
		encodedFilters := make([]string, 0, len(keys))
		for _, key := range keys {
			value := request.Filters[key]
			decodedFilters = append(decodedFilters, key+"="+value)
			encodedFilters = append(encodedFilters, url.PathEscape(key)+"="+url.PathEscape(value))
		}
		decodedSegments = append(decodedSegments, strings.Join(decodedFilters, "&"))
		encodedSegments = append(encodedSegments, strings.Join(encodedFilters, "&"))
	}

	return "/" + strings.Join(decodedSegments, "/"), "/" + strings.Join(encodedSegments, "/")
}

func decodeWrapper(body []byte, serviceID ServiceID) (Wrapper, *Result, error) {
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(body, &topLevel); err != nil {
		return Wrapper{}, nil, fmt.Errorf("%w: %v", errInvalidJSON, err)
	}
	rawWrapper, ok := topLevel[string(serviceID)]
	if !ok {
		if rawResult, resultOK := topLevel["RESULT"]; resultOK {
			var result Result
			if err := json.Unmarshal(rawResult, &result); err != nil {
				return Wrapper{}, nil, fmt.Errorf("top-level RESULT decode 실패: %v", err)
			}
			if result.Code == "" {
				return Wrapper{}, nil, errors.New("top-level RESULT.CODE가 비어 있습니다")
			}
			return Wrapper{}, &result, nil
		}
		return Wrapper{}, nil, fmt.Errorf("service wrapper %q가 없습니다", serviceID)
	}

	var wrapper Wrapper
	if err := json.Unmarshal(rawWrapper, &wrapper); err != nil {
		return Wrapper{}, nil, fmt.Errorf("service wrapper %q decode 실패: %v", serviceID, err)
	}
	if wrapper.Result.Code == "" {
		return Wrapper{}, nil, errors.New("RESULT.CODE가 비어 있습니다")
	}
	return wrapper, nil, nil
}

func responseMediaType(contentType string) (string, error) {
	if contentType == "" {
		return "", errors.New("HTTP 200 응답에 Content-Type이 없습니다")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("Content-Type을 해석할 수 없습니다: %v", err)
	}
	return strings.ToLower(mediaType), nil
}

func readBounded(reader io.Reader) ([]byte, bool, error) {
	limited := io.LimitReader(reader, MaxResponseBodySize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return body, false, err
	}
	if int64(len(body)) > MaxResponseBodySize {
		return body[:MaxResponseBodySize], true, nil
	}
	return body, false, nil
}

func finishMetadata(metadata *HTTPMetadata, finishedAt time.Time) {
	metadata.FinishedAt = finishedAt
	metadata.Duration = finishedAt.Sub(metadata.StartedAt)
}

func snapshotFilters(filters map[string]string, apiKey string) map[string]string {
	if len(filters) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(filters))
	for key, value := range filters {
		cloned[redact(apiKey, key)] = redact(apiKey, value)
	}
	return cloned
}

func sanitizedHeaders(header http.Header, apiKey string) http.Header {
	sanitized := make(http.Header, len(header))
	for key, values := range header {
		copied := make([]string, len(values))
		for index, value := range values {
			copied[index] = redact(apiKey, value)
		}
		sanitized[redact(apiKey, key)] = copied
	}
	return sanitized
}

func safeError(apiKey string, kind ErrorKind, op string, err error) error {
	return adapterError(kind, op, redact(apiKey, err.Error()))
}

func redact(apiKey, value string) string {
	redacted := strings.ReplaceAll(value, apiKey, MaskedAPIKey)
	redacted = strings.ReplaceAll(redacted, url.PathEscape(apiKey), MaskedAPIKey)
	return redacted
}

func adapterError(kind ErrorKind, op, message string) error {
	return &AdapterError{Kind: kind, Op: op, Message: message}
}

func apiResultError(apiKey string, result Result) error {
	return adapterError(
		ErrorKindAPI,
		"RESULT 검증",
		redact(apiKey, fmt.Sprintf("API 결과 코드 %q: %s", result.Code, result.Message)),
	)
}

var errInvalidJSON = errors.New("invalid JSON")
