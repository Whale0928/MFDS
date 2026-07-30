package operator

import (
	"context"
	"fmt"
	"time"
)

const (
	EventLimit = 200
)

type Event struct {
	ID                              uint64
	RunID, PartitionID, PageID      uint64
	WorkerID, Level, Phase, Message string
	MetadataJSON                    string
	CreatedAt                       time.Time
}

type Item struct {
	ID, RunID, PartitionID, PageID, FetchID uint64
	RowNo                                   uint32
	RCNO, ItemCode, ItemName                string
	ProductNameKO, ProductNameEN            string
	ImporterName, CountryName               string
	ProcessedDate                           *time.Time
	ObservedAt                              time.Time
}

type EventPage struct {
	Events      []Event
	NextAfterID uint64
	HasMore     bool
}

type Reader interface {
	LoadEvents(context.Context, uint64, string, uint64, int32) (EventPage, error)
	LoadPageItems(context.Context, uint64) ([]Item, error)
}

type Service struct {
	reader Reader
}

func NewService(reader Reader) (*Service, error) {
	if reader == nil {
		return nil, fmt.Errorf("operator reader가 필요합니다")
	}
	return &Service{reader: reader}, nil
}

func (s *Service) Events(
	ctx context.Context,
	runID uint64,
	workerID string,
	afterID uint64,
) (EventPage, error) {
	if runID == 0 {
		return EventPage{Events: []Event{}, NextAfterID: afterID}, nil
	}
	page, err := s.reader.LoadEvents(ctx, runID, workerID, afterID, EventLimit)
	if err != nil {
		return EventPage{}, fmt.Errorf("Job %d 이벤트 조회 실패: %w", runID, err)
	}
	if page.Events == nil {
		page.Events = []Event{}
	}
	if len(page.Events) > 0 {
		page.NextAfterID = page.Events[len(page.Events)-1].ID
	}
	page.HasMore = len(page.Events) == EventLimit
	return page, nil
}

func (s *Service) PageItems(ctx context.Context, pageID uint64) ([]Item, error) {
	if pageID == 0 {
		return nil, fmt.Errorf("Fetch ID는 양수여야 합니다")
	}
	items, err := s.reader.LoadPageItems(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("Fetch %d Item 조회 실패: %w", pageID, err)
	}
	if items == nil {
		items = []Item{}
	}
	return items, nil
}
