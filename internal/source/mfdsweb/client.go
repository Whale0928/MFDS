package mfdsweb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL      url.URL
	httpClient   *http.Client
	userAgent    string
	maxBodyBytes int64
	now          func() time.Time
}

func NewClient(options ClientOptions) (*Client, error) {
	if options.BaseURL == nil {
		return nil, validationError("client 생성", "base URL이 필요합니다")
	}
	baseURL := *options.BaseURL
	if !baseURL.IsAbs() ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.Host == "" ||
		baseURL.Hostname() == "" ||
		baseURL.Opaque != "" ||
		baseURL.User != nil ||
		baseURL.RawQuery != "" ||
		baseURL.ForceQuery ||
		baseURL.Fragment != "" {
		return nil, validationError(
			"client 생성",
			"base URL은 userinfo, query, fragment가 없는 절대 HTTP(S) URL이어야 합니다",
		)
	}

	userAgent := strings.TrimSpace(options.UserAgent)
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	maxBodyBytes := options.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = DefaultMaxBodyBytes
	}
	if maxBodyBytes < 0 {
		return nil, validationError("client 생성", "최대 응답 본문 크기는 0보다 커야 합니다")
	}

	httpClient := &http.Client{Timeout: DefaultRequestTimeout}
	if options.HTTPClient != nil {
		copied := *options.HTTPClient
		httpClient = &copied
		if httpClient.Timeout <= 0 {
			httpClient.Timeout = DefaultRequestTimeout
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

	return &Client{
		baseURL:      baseURL,
		httpClient:   httpClient,
		userAgent:    userAgent,
		maxBodyBytes: maxBodyBytes,
		now:          now,
	}, nil
}

func (c *Client) FetchList(ctx context.Context, listRequest ListRequest) (FetchArtifact, error) {
	if err := validateListRequest(listRequest); err != nil {
		return FetchArtifact{}, err
	}

	values := buildListQuery(listRequest)
	target := c.baseURL.ResolveReference(&url.URL{Path: ListPath})
	target.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return FetchArtifact{}, validationWrap("요청 생성", err)
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", c.userAgent)

	queryJSON, err := marshalStringValues(values)
	if err != nil {
		return FetchArtifact{}, validationWrap("query snapshot 생성", err)
	}
	requestHeadersJSON, err := json.Marshal(map[string]string{
		"Accept":          request.Header.Get("Accept"),
		"Accept-Encoding": request.Header.Get("Accept-Encoding"),
		"User-Agent":      request.Header.Get("User-Agent"),
	})
	if err != nil {
		return FetchArtifact{}, validationWrap("요청 header snapshot 생성", err)
	}

	requestKey := sha256.Sum256([]byte(ListPath + "?" + values.Encode()))
	startedAt := c.now()
	artifact := FetchArtifact{
		Method:             http.MethodGet,
		URL:                target.String(),
		QueryJSON:          queryJSON,
		RequestHeadersJSON: requestHeadersJSON,
		RequestKeySHA256:   requestKey,
		StartedAt:          startedAt,
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		artifact.FinishedAt = c.now()
		artifact.Duration = artifact.FinishedAt.Sub(startedAt)
		return artifact, &AdapterError{Kind: ErrorKindNetwork, Op: "HTTP 요청", Err: err}
	}
	defer response.Body.Close()

	artifact.HTTPStatus = response.StatusCode
	artifact.ContentType = response.Header.Get("Content-Type")
	artifact.ResponseHeadersJSON, err = sanitizeResponseHeaders(response.Header)
	if err != nil {
		return artifact, &AdapterError{Kind: ErrorKindValidation, Op: "응답 header snapshot 생성", Err: err}
	}

	body, compressed, bodyHash, err := captureBody(response.Body, c.maxBodyBytes)
	artifact.FinishedAt = c.now()
	artifact.Duration = artifact.FinishedAt.Sub(startedAt)
	if err != nil {
		kind := ErrorKindNetwork
		if errorsIsBodyLimit(err) {
			kind = ErrorKindBodyLimit
		}
		return artifact, &AdapterError{Kind: kind, Op: "응답 본문 읽기", Err: err}
	}
	artifact.Body = body
	artifact.BodyGZIP = compressed
	artifact.BodySize = int64(len(body))
	artifact.BodySHA256 = bodyHash

	if response.StatusCode != http.StatusOK {
		return artifact, &AdapterError{
			Kind: ErrorKindHTTP,
			Op:   "HTTP 상태 검증",
			Err:  fmt.Errorf("예상 상태 200, 실제 %d", response.StatusCode),
		}
	}
	if err := validateHTMLContentType(artifact.ContentType); err != nil {
		return artifact, &AdapterError{Kind: ErrorKindContentType, Op: "Content-Type 검증", Err: err}
	}
	return artifact, nil
}

func validateListRequest(listRequest ListRequest) error {
	if strings.TrimSpace(listRequest.Item.Name) == "" ||
		strings.TrimSpace(listRequest.Item.Name) != listRequest.Item.Name {
		return validationError("요청 검증", "품목명은 비어 있거나 앞뒤 공백을 포함할 수 없습니다")
	}
	if strings.TrimSpace(listRequest.Item.Code) == "" ||
		strings.TrimSpace(listRequest.Item.Code) != listRequest.Item.Code {
		return validationError("요청 검증", "품목 코드는 비어 있거나 앞뒤 공백을 포함할 수 없습니다")
	}
	for _, character := range listRequest.Item.Code {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return validationError("요청 검증", "품목 코드는 영문 대문자와 숫자만 허용합니다")
		}
	}
	if listRequest.ProcessDate.IsZero() {
		return validationError("요청 검증", "처리일이 필요합니다")
	}
	if listRequest.Page < 1 {
		return validationError("요청 검증", "page는 1 이상이어야 합니다")
	}
	if listRequest.Limit < 1 || listRequest.Limit > MaximumListPageSize {
		return validationError("요청 검증", fmt.Sprintf("limit은 1 이상 %d 이하여야 합니다", MaximumListPageSize))
	}
	if listRequest.Page == 1 && listRequest.TotalSnapshot != nil {
		return validationError("요청 검증", "page 1에는 total snapshot을 전달하지 않습니다")
	}
	if listRequest.Page > 1 {
		if listRequest.TotalSnapshot == nil || *listRequest.TotalSnapshot <= 0 {
			return validationError("요청 검증", "page 2 이상에는 page 1의 양수 total snapshot이 필요합니다")
		}
		totalPages := (*listRequest.TotalSnapshot + listRequest.Limit - 1) / listRequest.Limit
		if listRequest.Page > totalPages {
			return validationError("요청 검증", "page가 total snapshot으로 계산한 마지막 page를 초과합니다")
		}
	}
	return nil
}

func buildListQuery(listRequest ListRequest) url.Values {
	date := listRequest.ProcessDate.Format(time.DateOnly)
	values := url.Values{
		"limit":      {strconv.Itoa(listRequest.Limit)},
		"page":       {strconv.Itoa(listRequest.Page)},
		"rpsntItmCd": {listRequest.Item.Code},
		"rpsntItmNm": {listRequest.Item.Name},
		"srchEndDt":  {date},
		"srchStrtDt": {date},
	}
	if listRequest.Page > 1 {
		values.Set("totalCnt", strconv.Itoa(*listRequest.TotalSnapshot))
	}
	return values
}

func marshalStringValues(values url.Values) ([]byte, error) {
	snapshot := make(map[string]string, len(values))
	for key, entries := range values {
		if len(entries) != 1 {
			return nil, fmt.Errorf("query %q 값은 정확히 하나여야 합니다", key)
		}
		snapshot[key] = entries[0]
	}
	return json.Marshal(snapshot)
}

func validateHTMLContentType(value string) error {
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil {
		return fmt.Errorf("Content-Type을 해석할 수 없습니다: %w", err)
	}
	if !strings.EqualFold(mediaType, "text/html") {
		return fmt.Errorf("Content-Type이 text/html이 아닙니다: %q", mediaType)
	}
	if charset := parameters["charset"]; charset != "" && !strings.EqualFold(charset, "utf-8") {
		return fmt.Errorf("HTML charset이 UTF-8이 아닙니다: %q", charset)
	}
	return nil
}

func validationError(op, message string) error {
	return &AdapterError{Kind: ErrorKindValidation, Op: op, Err: fmt.Errorf("%s", message)}
}

func validationWrap(op string, err error) error {
	return &AdapterError{Kind: ErrorKindValidation, Op: op, Err: err}
}
