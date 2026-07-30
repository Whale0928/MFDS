package mysql

type CrawlRunType string

const (
	CrawlRunTypeDaily    CrawlRunType = "DAILY"
	CrawlRunTypeBackfill CrawlRunType = "BACKFILL"
	CrawlRunTypeResume   CrawlRunType = "RESUME"
)

type CrawlRunStatus string

const (
	CrawlRunStatusCreated       CrawlRunStatus = "CREATED"
	CrawlRunStatusListing       CrawlRunStatus = "LISTING"
	CrawlRunStatusDetailing     CrawlRunStatus = "DETAILING"
	CrawlRunStatusVerifying     CrawlRunStatus = "VERIFYING"
	CrawlRunStatusCompleted     CrawlRunStatus = "COMPLETED"
	CrawlRunStatusRetryWait     CrawlRunStatus = "RETRY_WAIT"
	CrawlRunStatusPartialFailed CrawlRunStatus = "PARTIAL_FAILED"
	CrawlRunStatusCancelled     CrawlRunStatus = "CANCELLED"
)

type CrawlPartitionStatus string

const (
	CrawlPartitionStatusPending     CrawlPartitionStatus = "PENDING"
	CrawlPartitionStatusLeased      CrawlPartitionStatus = "LEASED"
	CrawlPartitionStatusPaging      CrawlPartitionStatus = "PAGING"
	CrawlPartitionStatusReconciling CrawlPartitionStatus = "RECONCILING"
	CrawlPartitionStatusDone        CrawlPartitionStatus = "DONE"
	CrawlPartitionStatusDirty       CrawlPartitionStatus = "DIRTY"
	CrawlPartitionStatusRetryWait   CrawlPartitionStatus = "RETRY_WAIT"
	CrawlPartitionStatusFailed      CrawlPartitionStatus = "FAILED"
)

type CrawlPageStatus string

const (
	CrawlPageStatusPending         CrawlPageStatus = "PENDING"
	CrawlPageStatusLeased          CrawlPageStatus = "LEASED"
	CrawlPageStatusRawStored       CrawlPageStatus = "RAW_STORED"
	CrawlPageStatusParsedCommitted CrawlPageStatus = "PARSED_COMMITTED"
	CrawlPageStatusDone            CrawlPageStatus = "DONE"
	CrawlPageStatusRetryWait       CrawlPageStatus = "RETRY_WAIT"
	CrawlPageStatusParseFailed     CrawlPageStatus = "PARSE_FAILED"
	CrawlPageStatusFailed          CrawlPageStatus = "FAILED"
)

type CrawlFetchSourceKind string

const (
	CrawlFetchSourceKindWebList   CrawlFetchSourceKind = "WEB_LIST"
	CrawlFetchSourceKindWebDetail CrawlFetchSourceKind = "WEB_DETAIL"
)

type CrawlFetchStatus string

const (
	CrawlFetchStatusStarted   CrawlFetchStatus = "STARTED"
	CrawlFetchStatusRawStored CrawlFetchStatus = "RAW_STORED"
	CrawlFetchStatusParsed    CrawlFetchStatus = "PARSED"
	CrawlFetchStatusFailed    CrawlFetchStatus = "FAILED"
)

type RcnoDetailStatus string

const (
	RcnoDetailStatusUnseen    RcnoDetailStatus = "UNSEEN"
	RcnoDetailStatusQueued    RcnoDetailStatus = "QUEUED"
	RcnoDetailStatusLeased    RcnoDetailStatus = "LEASED"
	RcnoDetailStatusRetryWait RcnoDetailStatus = "RETRY_WAIT"
	RcnoDetailStatusStored    RcnoDetailStatus = "STORED"
	RcnoDetailStatusDead      RcnoDetailStatus = "DEAD"
)
