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
	StartCollection(context.Context, time.Time, json.RawMessage) (uint64, error)
	SavePage(context.Context, uint64, Page, error) error
	ListLatestImporters(context.Context) ([]Importer, error)
	ListC001Candidates(context.Context, uint64) ([]LicenseCandidate, error)
	SaveMatchEvidence(context.Context, uint64, []MatchEvidence) error
	CompleteCollection(context.Context, uint64, Summary, time.Time) error
	FailCollection(context.Context, uint64, error, time.Time) error
}

type Importer struct {
	SourceItemID uint64
	RCNO         string
	Name         string
}

type LicenseCandidate struct {
	RawID        uint64
	LicenseNo    string
	BusinessName string
	Address      string
}

type MatchStatus string

const (
	MatchExactName      MatchStatus = "EXACT_NAME"
	MatchNormalizedName MatchStatus = "NORMALIZED_NAME"
	MatchNameAndAddress MatchStatus = "NAME_AND_ADDRESS"
	MatchConfirmedAlias MatchStatus = "CONFIRMED_ALIAS"
	MatchAmbiguous      MatchStatus = "AMBIGUOUS"
	MatchUnresolved     MatchStatus = "UNRESOLVED"
	MatchManual         MatchStatus = "MANUAL"
)

type MatchEvidence struct {
	SourceItemID     uint64
	RCNO             string
	ImporterName     string
	ImporterMatchKey string
	C001RawID        *uint64
	LicenseNo        string
	BusinessName     string
	Address          string
	Status           MatchStatus
	CandidateCount   int
	MatcherVersion   string
	EvidenceJSON     json.RawMessage
	MatchedAt        time.Time
}

type ServiceSummary struct {
	Fetches       uint64
	Rows          uint64
	ReportedTotal uint64
}

type Summary struct {
	CollectionID uint64
	Services     map[ServiceID]ServiceSummary
	Matches      map[MatchStatus]uint64
}

type Options struct {
	PageSize       int
	MaxPages       int
	MaxRequests    int
	QPS            float64
	MaxAttempts    int
	RetryDelays    []time.Duration
	MatcherVersion string
	Now            func() time.Time
}

type RetryableError interface {
	error
	Retryable() bool
}
