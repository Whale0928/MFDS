package weblist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumPageSize          = 50
	RunStatusListing         = "RUNNING"
	RunStatusCreated         = "CREATED"
	RunStatusCompleted       = "COMPLETED"
	RunStatusPartialFailed   = "PARTIAL_FAILED"
	RunStatusCancelled       = "CANCELLED"
	PartitionStatusDone      = "DONE"
	PartitionStatusPending   = "PENDING"
	PartitionStatusDirty     = "FAILED"
	PartitionStatusFailed    = "FAILED"
	defaultExecutionLease    = 5 * time.Minute
	leaseRenewInterval       = time.Minute
	workerIdleInterval       = 200 * time.Millisecond
	failureCleanupTimeout    = 5 * time.Second
	maximumStoredErrorLength = 4096
)

var ErrLeaseLost = errors.New("lease lost")

type Target struct {
	Name string
	Code string
}

type Options struct {
	Targets               []Target
	PageSize              int
	QPS                   float64
	MaxAttempts           int
	RetryDelays           []time.Duration
	Location              *time.Location
	WebBaseURL            string
	ProxyMode, ProxyLabel string
	Now                   func() time.Time
	ProcessID             string
}

type Command struct {
	ItemName, ProcessDate string
}

type JobCommand struct {
	// ItemNames는 이전 호출부 호환용이며 고정 4개 품목 실행에서는 사용하지 않습니다.
	ItemNames        []string
	FromDate, ToDate string
	Workers          int
	OnStarted        func(uint64)
}

type ListRequest struct {
	ItemName      string
	ItemCode      string
	ProcessDate   time.Time
	Page          int
	Limit         int
	TotalSnapshot *int
}

type Fetch struct {
	Method, URL                   string
	QueryJSON, RequestHeadersJSON []byte
	RequestKeySHA256              [32]byte
	StartedAt, FinishedAt         time.Time
	Duration                      time.Duration
	HTTPStatus                    int
	ResponseHeadersJSON           []byte
	ContentType                   string
	Body, BodyGZIP                []byte
	BodySize                      int64
	BodySHA256                    [32]byte
	BodyCaptured                  bool
}

type Page struct {
	Total, Page, Limit, TotalPages int
	Rows                           []Row
	Warnings                       []string
}

type Row struct {
	RowNo                                                 int
	RCNO                                                  string
	ProductDivisionName, ImporterName                     string
	ProductNameKO, ProductNameEN, ItemName                string
	OverseasEstablishmentName, ProcessedDateRaw           string
	ProcessedDate                                         time.Time
	ExpiryText, ManufactureCountryName, ExportCountryName string
	DetailHref                                            string
	CanonicalValuesJSON, RawRowHTML                       []byte
	RawRowSHA256, SemanticSHA256                          [32]byte
}

type ListSource interface {
	FetchList(context.Context, ListRequest) (Fetch, error)
	ParseList([]byte, ListRequest) (Page, error)
	ErrorKind(error) string
}

type JobStartParams struct {
	FromDate, ToDate time.Time
	StartedAt        time.Time
	PageSize         uint32
	ConfigJSON       json.RawMessage
}

type JobStartRecord struct {
	RunID uint64
}

type ClaimParams struct {
	RunID           uint64
	Owner           string
	Now, LeaseUntil time.Time
}

type DateTask struct {
	RunID, TaskID uint64
	Attempt       uint32
	Owner         string
	ProcessDate   time.Time
}

type CommitFetchParams struct {
	Task       DateTask
	Request    ListRequest
	Fetch      Fetch
	Page       Page
	ObservedAt time.Time
}

type FailedFetchParams struct {
	Task                    DateTask
	Request                 ListRequest
	Fetch                   Fetch
	ErrorKind, ErrorMessage string
	ParserFailure           bool
}

type AttemptPageEvidence struct {
	PageNo     uint32
	Status     string
	Total      uint64
	ParsedRows uint32
}

type AttemptItemEvidence struct {
	ItemName, ItemCode string
	Pages              []AttemptPageEvidence
	RCNOs              []string
}

type TaskAttemptEvidence struct {
	Attempt uint32
	Items   []AttemptItemEvidence
}

type FailTaskParams struct {
	Task          DateTask
	Retryable     bool
	MaxAttempts   int
	NextAttemptAt time.Time
	ErrorMessage  string
}

type Result struct {
	RunID, PartitionID            uint64
	ItemName, ItemCode            string
	ProcessDate                   time.Time
	RunStatus, PartitionStatus    string
	ExpectedTotal, ParsedRows     uint64
	ExpectedPages, FetchedPages   uint32
	UniqueRCNOCount, NewRCNOCount uint64
}

type JobResult struct {
	RunID                                                  uint64
	Status                                                 string
	TotalPartitions, CompletedPartitions, FailedPartitions uint32
	FetchedPages                                           uint32
	ParsedRows, UniqueRCNOCount, NewRCNOCount              uint64
	Units                                                  []Result
}

type Store interface {
	StartWebListJob(context.Context, JobStartParams) (JobStartRecord, error)
	ClaimTask(context.Context, ClaimParams) (DateTask, bool, error)
	RenewTask(context.Context, DateTask, time.Time) error
	CommitFetch(context.Context, CommitFetchParams) error
	RecordFailedFetch(context.Context, FailedFetchParams) error
	LoadTaskAttemptEvidence(context.Context, DateTask) ([]TaskAttemptEvidence, error)
	CompleteTask(context.Context, DateTask) error
	FailTask(context.Context, FailTaskParams) error
	RequestCancellation(context.Context, uint64) error
	FinalizeJob(context.Context, uint64, time.Time) (string, bool, error)
	LoadJobResult(context.Context, uint64) (JobResult, error)
}

func SanitizeErrorMessage(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= maximumStoredErrorLength {
		return value
	}
	value = value[:maximumStoredErrorLength]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func validateTargets(targets []Target) error {
	if len(targets) != 4 {
		return fmt.Errorf("고정 수집 품목은 정확히 4개여야 합니다: got=%d", len(targets))
	}
	expected := []string{"위스키", "브랜디", "일반증류주", "리큐르"}
	for index, name := range expected {
		if targets[index].Name != name || strings.TrimSpace(targets[index].Code) == "" {
			return fmt.Errorf("고정 수집 품목 순서가 유효하지 않습니다: index=%d name=%q", index, targets[index].Name)
		}
	}
	return nil
}
