package companyregistry

import (
	"context"
	"encoding/json"
	"time"
)

type ServiceID string

const (
	ServiceC001  ServiceID = "C001"
	ServiceI2821 ServiceID = "I2821"
	ServiceI0250 ServiceID = "I0250"
	ServiceI0470 ServiceID = "I0470"
)

var Services = []ServiceID{ServiceC001, ServiceI0250, ServiceI0470, ServiceI2821}

type PageRequest struct {
	Service    ServiceID
	StartIndex int
	EndIndex   int
	Attempt    int
	Filters    map[string]string
}

type Page struct {
	Service             ServiceID
	StartIndex          int
	EndIndex            int
	Attempt             int
	RequestPathRedacted string
	RequestFilterJSON   json.RawMessage
	HTTPStatus          int
	ContentType         string
	ResponseHeaders     map[string][]string
	ResultCode          string
	ResultMessage       string
	TotalCount          uint64
	Rows                []json.RawMessage
	RawBody             []byte
	StartedAt           time.Time
	FinishedAt          time.Time
}

type Client interface {
	FetchPage(context.Context, PageRequest) (Page, error)
}

type Store interface {
	LatestCompletedThrough(context.Context) (*time.Time, error)
	StartCollection(context.Context, time.Time, SyncWindow, json.RawMessage) (uint64, error)
	SavePage(context.Context, uint64, Page, error) error
	CompleteCollection(context.Context, uint64, Summary, time.Time) error
	FailCollection(context.Context, uint64, error, time.Time) error
}

type SyncWindow struct {
	Since   time.Time
	Through time.Time
}

type ServiceSummary struct {
	Fetches       uint64
	Rows          uint64
	ReportedTotal uint64
}

type Summary struct {
	CollectionID uint64
	Since        time.Time
	Through      time.Time
	Services     map[ServiceID]ServiceSummary
}

type Options struct {
	PageSize    int
	MaxPages    int
	MaxRequests int
	QPS         float64
	MaxAttempts int
	RetryDelays []time.Duration
	Since       *time.Time
	Location    *time.Location
	Now         func() time.Time
}

type RetryableError interface {
	error
	Retryable() bool
}
