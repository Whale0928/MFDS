package ledger

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

const PageLimit = 100

type Filters struct {
	ItemCode string
	FromDate *time.Time
	ToDate   *time.Time
	RCNO     string
}

type Observation struct {
	ID, RunID, PartitionID, PageID, FetchID uint64
	RCNO, ItemCode, ItemName                string
	ProductNameKO, ProductNameEN            string
	ImporterName, CountryName               string
	ProcessedDate                           *time.Time
	SemanticSHA256                          []byte
	ParserVersion, ParserWarning            string
	ObservedAt                              time.Time
}

type Page struct {
	Items        []Observation
	NextBeforeID uint64
	HasMore      bool
}

type Reader interface {
	LoadObservations(context.Context, Filters, uint64, int32) (Page, error)
}

type Service struct {
	reader Reader
}

func NewService(reader Reader) (*Service, error) {
	if reader == nil {
		return nil, fmt.Errorf("ledger reader가 필요합니다")
	}
	return &Service{reader: reader}, nil
}

func (s *Service) Observations(
	ctx context.Context,
	filters Filters,
	beforeID uint64,
) (Page, error) {
	if beforeID == 0 {
		beforeID = math.MaxUint64
	}
	filters.ItemCode = strings.TrimSpace(filters.ItemCode)
	filters.RCNO = strings.TrimSpace(filters.RCNO)
	if filters.FromDate != nil && filters.ToDate != nil && filters.FromDate.After(*filters.ToDate) {
		return Page{}, fmt.Errorf("원장 시작일은 종료일보다 늦을 수 없습니다")
	}
	page, err := s.reader.LoadObservations(ctx, filters, beforeID, PageLimit)
	if err != nil {
		return Page{}, fmt.Errorf("전체 원장 조회 실패: %w", err)
	}
	if page.Items == nil {
		page.Items = []Observation{}
	}
	page.NextBeforeID = beforeID
	if len(page.Items) > 0 {
		page.NextBeforeID = page.Items[len(page.Items)-1].ID
	}
	page.HasMore = len(page.Items) == PageLimit
	return page, nil
}
