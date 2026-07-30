package overview

import (
	"context"
	"fmt"
	"time"
)

const RecentRunLimit = 12

type Snapshot struct {
	TotalRuns            int64
	ActiveRuns           int64
	CompletedRuns        int64
	FailedRuns           int64
	DirtyPartitions      int64
	ListRawRows          int64
	UniqueRCNO           int64
	DetailQueued         int64
	DetailWorking        int64
	GlobalPendingDetails int64
	DetailStored         int64
	DetailDead           int64
	DetailRawRows        int64
	RecentRuns           []Run
	LoadedAt             time.Time
}

type Run struct {
	ID                  uint64
	RunType             string
	RequestedFromDate   time.Time
	RequestedToDate     time.Time
	Status              string
	TotalPartitions     uint32
	CompletedPartitions uint32
	ParsedRows          uint64
	NewRCNOCount        uint64
	LastError           string
	CreatedAt           time.Time
	FinishedAt          *time.Time
}

type RunDetail struct {
	RunID               uint64
	Status              string
	TotalPartitions     uint32
	CompletedPartitions uint32
	FetchedRequests     uint64
	ParsedRows          uint64
	DirtyPartitions     int64
	PendingPages        int64
	Partitions          []Partition
	Pages               []Page
}

type Partition struct {
	ID              uint64
	ItemName        string
	ProcessDate     time.Time
	Status          string
	ExpectedTotal   *int64
	ExpectedPages   *int32
	ParsedRows      uint64
	UniqueRCNOCount uint64
	Attempts        uint32
	LastError       string
}

type Page struct {
	ID              uint64
	PartitionID     uint64
	ItemName        string
	ItemCode        string
	ProcessDate     time.Time
	PageNo          uint32
	Status          string
	WorkerID        string
	TotalSnapshot   *int64
	RowCount        *int32
	UniqueRCNOCount *int32
	Attempts        uint32
	LastError       string
}

type Reader interface {
	LoadOverview(context.Context, int32) (Snapshot, error)
	LoadRunDetail(context.Context, uint64) (RunDetail, error)
}

type Service struct {
	reader Reader
	now    func() time.Time
}

func NewService(reader Reader) (*Service, error) {
	if reader == nil {
		return nil, fmt.Errorf("overview reader가 필요합니다")
	}
	return &Service{reader: reader, now: time.Now}, nil
}

func (s *Service) Load(ctx context.Context) (Snapshot, error) {
	snapshot, err := s.reader.LoadOverview(ctx, RecentRunLimit)
	if err != nil {
		return Snapshot{}, fmt.Errorf("운영 현황 조회 실패: %w", err)
	}
	snapshot.LoadedAt = s.now()
	if snapshot.RecentRuns == nil {
		snapshot.RecentRuns = []Run{}
	}
	return snapshot, nil
}

func (s *Service) LoadRunDetail(ctx context.Context, runID uint64) (RunDetail, error) {
	if runID == 0 {
		return RunDetail{}, fmt.Errorf("run ID는 양수여야 합니다")
	}
	detail, err := s.reader.LoadRunDetail(ctx, runID)
	if err != nil {
		return RunDetail{}, fmt.Errorf("run %d 상세 조회 실패: %w", runID, err)
	}
	if detail.Partitions == nil {
		detail.Partitions = []Partition{}
	}
	if detail.Pages == nil {
		detail.Pages = []Page{}
	}
	return detail, nil
}
