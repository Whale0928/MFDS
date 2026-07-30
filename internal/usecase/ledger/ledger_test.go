package ledger

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

type fakeReader struct {
	page     Page
	filters  Filters
	beforeID uint64
	err      error
}

func (f *fakeReader) LoadObservations(
	_ context.Context,
	filters Filters,
	beforeID uint64,
	_ int32,
) (Page, error) {
	f.filters, f.beforeID = filters, beforeID
	return f.page, f.err
}

func TestService_초기Cursor와Filter를전달한다(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	reader := &fakeReader{page: Page{Items: []Observation{{ID: 9}, {ID: 4}}}}
	service, err := NewService(reader)
	if err != nil {
		t.Fatal(err)
	}

	page, err := service.Observations(context.Background(), Filters{
		ItemCode: " item ", FromDate: &from, RCNO: " rcno ",
	}, 0)

	if err != nil {
		t.Fatal(err)
	}
	if reader.beforeID != math.MaxUint64 || reader.filters.ItemCode != "item" ||
		reader.filters.RCNO != "rcno" || page.NextBeforeID != 4 || page.HasMore {
		t.Fatalf("reader=%+v page=%+v", reader, page)
	}
}

func TestService_경계와조회오류를검증한다(t *testing.T) {
	from := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, -1)
	service, err := NewService(&fakeReader{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Observations(context.Background(), Filters{
		FromDate: &from, ToDate: &to,
	}, 10); err == nil {
		t.Fatal("invalid range error = nil")
	}
	service, _ = NewService(&fakeReader{err: errors.New("db unavailable")})
	if _, err := service.Observations(context.Background(), Filters{}, 10); err == nil {
		t.Fatal("reader error = nil")
	}
}
