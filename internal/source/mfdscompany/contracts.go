package mfdscompany

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// HTTPStatusError preserves a non-200 response without following an
// unverified redirect away from the requested official page.
type HTTPStatusError struct {
	StatusCode int
	Location   string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("예상 상태 200, 실제 %d", e.StatusCode)
}

const (
	ListPath          = "/CFCCC01F02/getList"
	DetailPath        = "/CFCCC01P01"
	GalleryListPath   = "/CFCCC01F12/getList"
	GalleryDetailPath = "/CFCCC01F13/getGalleryDetail"

	DefaultUserAgent   = "bottle-note-mfds-company-scraper/1.0"
	DefaultMaxBodySize = 2 << 20
	MaximumPageSize    = 50
)

type Options struct {
	BaseURL      *url.URL
	HTTPClient   *http.Client
	UserAgent    string
	MaxBodyBytes int64
	Now          func() time.Time
}

type SearchRequest struct {
	Page           int
	Limit          int
	TotalSnapshot  *int
	IndustryCode   string
	BusinessName   string
	OperatingState string
}

type SearchPage struct {
	Total      int
	Page       int
	Limit      int
	TotalPages int
	Rows       []SearchResult
	Source     SourceMetadata
}

type GallerySearchRequest struct {
	Page         int
	Limit        int
	BusinessName string
	FromDate     time.Time
	ToDate       time.Time
}

type GalleryPage struct {
	Total      int
	Page       int
	Limit      int
	TotalPages int
	Products   []GalleryProduct
	Source     SourceMetadata
}

type GalleryProduct struct {
	ProductCode string
	ProductName string
}

type GalleryDetail struct {
	ProductCode          string
	RCNO                 string
	InternalBusinessCode string
	BusinessName         string
	Source               SourceMetadata
}

type SearchResult struct {
	RowNumber            int
	InternalBusinessCode string
	LicenseNumber        string
	BusinessName         string
	Representative       string
	Address              string
	Industry             string
	Institution          string
	DetailURL            string
}

type BusinessDetail struct {
	InternalBusinessCode string
	LicenseNumber        string
	BusinessName         string
	Representative       string
	PermitDate           string
	Institution          string
	Address              string
	Phone                string
	Industry             string
	OperatingStatus      string
	Source               SourceMetadata
}

type SourceMetadata struct {
	Endpoint    string
	RequestURL  string
	RetrievedAt time.Time
	HTTPStatus  int
	ContentType string
	BodyBytes   int64
	BodySHA256  string
}

type Scraper struct {
	baseURL      url.URL
	httpClient   *http.Client
	userAgent    string
	maxBodyBytes int64
	now          func() time.Time
}
