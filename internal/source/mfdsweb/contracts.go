package mfdsweb

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	ListPath              = "/CFCCC01F01/getList"
	ParserVersion         = "mfds-web-list/v1"
	DefaultUserAgent      = "bottle-note-mfds-crawler/1.0"
	DefaultMaxBodyBytes   = int64(2 << 20)
	DefaultRequestTimeout = 20 * time.Second
	MaximumListPageSize   = 50
)

var ErrBodyTooLarge = errors.New("MFDS 응답 본문이 허용 크기를 초과했습니다")

type ErrorKind string

const (
	ErrorKindValidation  ErrorKind = "VALIDATION"
	ErrorKindNetwork     ErrorKind = "NETWORK"
	ErrorKindHTTP        ErrorKind = "HTTP"
	ErrorKindBodyLimit   ErrorKind = "BODY_LIMIT"
	ErrorKindContentType ErrorKind = "CONTENT_TYPE"
	ErrorKindParse       ErrorKind = "PARSE"
)

type AdapterError struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *AdapterError) Error() string {
	return fmt.Sprintf("MFDS 웹 목록 %s 실패 [%s]: %v", e.Op, e.Kind, e.Err)
}

func (e *AdapterError) Unwrap() error {
	return e.Err
}

type Item struct {
	Name string
	Code string
}

type ListRequest struct {
	Item          Item
	ProcessDate   time.Time
	Page          int
	Limit         int
	TotalSnapshot *int
}

type ClientOptions struct {
	BaseURL      *url.URL
	HTTPClient   *http.Client
	UserAgent    string
	MaxBodyBytes int64
	Now          func() time.Time
}

type FetchArtifact struct {
	Method              string
	URL                 string
	QueryJSON           []byte
	RequestHeadersJSON  []byte
	RequestKeySHA256    [32]byte
	StartedAt           time.Time
	FinishedAt          time.Time
	Duration            time.Duration
	HTTPStatus          int
	ResponseHeadersJSON []byte
	ContentType         string
	Body                []byte
	BodyGZIP            []byte
	BodySize            int64
	BodySHA256          [32]byte
}

type ListPage struct {
	Total      int
	Page       int
	Limit      int
	TotalPages int
	Rows       []ListRow
	Warnings   []string
}

type ListRow struct {
	RowNo                     int
	RCNO                      string
	ProductDivisionName       string
	ImporterName              string
	ProductNameKO             string
	ProductNameEN             string
	ItemName                  string
	OverseasEstablishmentName string
	ProcessedDateRaw          string
	ProcessedDate             time.Time
	ExpiryText                string
	ManufactureCountryName    string
	ExportCountryName         string
	DetailHref                string
	CanonicalValuesJSON       []byte
	RawRowHTML                []byte
	RawRowSHA256              [32]byte
	SemanticSHA256            [32]byte
}
