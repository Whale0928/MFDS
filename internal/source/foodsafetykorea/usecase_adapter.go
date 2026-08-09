package foodsafetykorea

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/bottle-note/mfds-crawler/internal/usecase/companyregistry"
)

type UsecaseAdapter struct {
	client *Client
}

func NewUsecaseAdapter(client *Client) *UsecaseAdapter {
	return &UsecaseAdapter{client: client}
}

func (a *UsecaseAdapter) FetchPage(
	ctx context.Context,
	request companyregistry.PageRequest,
) (companyregistry.Page, error) {
	if a == nil || a.client == nil {
		return companyregistry.Page{}, &usecaseError{kind: string(ErrorKindValidation), cause: errors.New("식품안전나라 client가 필요합니다")}
	}

	sourceRequest := PageRequest{StartIndex: request.StartIndex, EndIndex: request.EndIndex, Filters: request.Filters}
	switch request.Service {
	case companyregistry.ServiceC001:
		page, err := a.client.FetchC001(ctx, sourceRequest)
		return adaptPage(page, request, err, func(row C001Row) json.RawMessage { return row.RawJSON })
	case companyregistry.ServiceI2821:
		page, err := a.client.FetchI2821(ctx, sourceRequest)
		return adaptPage(page, request, err, func(row I2821Row) json.RawMessage { return row.RawJSON })
	case companyregistry.ServiceI0250:
		page, err := a.client.FetchI0250(ctx, sourceRequest)
		return adaptPage(page, request, err, func(row I0250Row) json.RawMessage { return row.RawJSON })
	case companyregistry.ServiceI0470:
		page, err := a.client.FetchI0470(ctx, sourceRequest)
		return adaptPage(page, request, err, func(row I0470Row) json.RawMessage { return row.RawJSON })
	default:
		return companyregistry.Page{}, &usecaseError{
			kind:  string(ErrorKindValidation),
			cause: fmt.Errorf("지원하지 않는 식품안전나라 서비스: %s", request.Service),
		}
	}
}

func adaptPage[T any](
	source Page[T],
	request companyregistry.PageRequest,
	fetchErr error,
	raw func(T) json.RawMessage,
) (companyregistry.Page, error) {
	filters, marshalErr := json.Marshal(source.Request.Filters)
	if marshalErr != nil {
		return companyregistry.Page{}, &usecaseError{kind: string(ErrorKindContract), cause: marshalErr}
	}
	if string(filters) == "null" {
		filters = []byte(`{}`)
	}
	path := source.Request.URL
	if parsed, err := url.Parse(source.Request.URL); err == nil {
		path = parsed.EscapedPath()
	}
	page := companyregistry.Page{
		Service:             request.Service,
		StartIndex:          request.StartIndex,
		EndIndex:            request.EndIndex,
		Attempt:             request.Attempt,
		RequestPathRedacted: path,
		RequestFilterJSON:   filters,
		HTTPStatus:          source.HTTP.StatusCode,
		ContentType:         source.HTTP.ContentType,
		ResponseHeaders:     source.HTTP.ResponseHeaders,
		ResultCode:          source.Result.Code,
		ResultMessage:       source.Result.Message,
		RawBody:             source.RawBody,
		StartedAt:           source.HTTP.StartedAt,
		FinishedAt:          source.HTTP.FinishedAt,
	}
	if source.TotalCount != "" {
		total, err := strconv.ParseUint(source.TotalCount, 10, 64)
		if err != nil {
			return page, &usecaseError{
				kind:  string(ErrorKindContract),
				cause: fmt.Errorf("%s total_count 해석 실패: %w", request.Service, err),
			}
		}
		page.TotalCount = total
	}
	page.Rows = make([]json.RawMessage, 0, len(source.Rows))
	for _, row := range source.Rows {
		page.Rows = append(page.Rows, append(json.RawMessage(nil), raw(row)...))
	}
	if fetchErr != nil {
		return page, classifyError(fetchErr, page)
	}
	return page, nil
}

func classifyError(cause error, page companyregistry.Page) error {
	kind := "UNKNOWN"
	var adapterErr *AdapterError
	if errors.As(cause, &adapterErr) {
		kind = string(adapterErr.Kind)
	}
	retryable := false
	switch ErrorKind(kind) {
	case ErrorKindNetwork, ErrorKindEmptyBody, ErrorKindHTML, ErrorKindNonJSON:
		retryable = true
	case ErrorKindHTTP:
		retryable = page.HTTPStatus == 429 || page.HTTPStatus >= 500
	case ErrorKindAPI:
		retryable = page.ResultCode == "INFO-300" || page.ResultCode == "ERROR-500" || page.ResultCode == "ERROR-601"
	}
	return &usecaseError{kind: kind, retryable: retryable, cause: cause}
}

type usecaseError struct {
	kind      string
	retryable bool
	cause     error
}

func (e *usecaseError) Error() string { return e.cause.Error() }
func (e *usecaseError) Unwrap() error { return e.cause }
func (e *usecaseError) Kind() string  { return e.kind }
func (e *usecaseError) Retryable() bool {
	return e.retryable
}

func supportedFilters(service ServiceID) map[string]struct{} {
	switch service {
	case ServiceC001, ServiceI2821:
		return map[string]struct{}{"CHNG_DT": {}, "LCNS_NO": {}}
	case ServiceI0470:
		return map[string]struct{}{"CHNG_DT": {}, "DSPS_DCSNDT": {}, "LCNS_NO": {}}
	default:
		return map[string]struct{}{}
	}
}

func validateServiceFilters(service ServiceID, filters map[string]string) error {
	allowed := supportedFilters(service)
	for key, value := range filters {
		if _, ok := allowed[key]; !ok {
			return adapterError(ErrorKindValidation, "요청 검증", fmt.Sprintf("%s은 %s filter를 지원하지 않습니다", service, key))
		}
		if strings.TrimSpace(value) == "" {
			return adapterError(ErrorKindValidation, "요청 검증", fmt.Sprintf("%s filter 값이 비어 있습니다", key))
		}
	}
	return nil
}
