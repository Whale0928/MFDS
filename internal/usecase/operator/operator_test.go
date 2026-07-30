package operator

import (
	"context"
	"errors"
	"testing"
)

type fakeReader struct {
	events []Event
	items  []Item
	err    error
}

func (f fakeReader) LoadEvents(context.Context, uint64, string, uint64, int32) (EventPage, error) {
	return EventPage{Events: f.events}, f.err
}

func (f fakeReader) LoadPageItems(context.Context, uint64) ([]Item, error) {
	return f.items, f.err
}

func TestService_빈조회결과_nil대신빈목록을반환한다(t *testing.T) {
	service, err := NewService(fakeReader{})
	if err != nil {
		t.Fatal(err)
	}

	events, err := service.Events(context.Background(), 1, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.PageItems(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if events.Events == nil || items == nil {
		t.Fatalf("events=%#v items=%#v", events.Events, items)
	}
}

func TestService_조회오류에운영맥락을추가한다(t *testing.T) {
	service, err := NewService(fakeReader{err: errors.New("db unavailable")})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Events(context.Background(), 7, "worker-1", 10); err == nil {
		t.Fatal("Events() error = nil")
	}
	if _, err := service.PageItems(context.Background(), 9); err == nil {
		t.Fatal("PageItems() error = nil")
	}
}

func TestService_이벤트Cursor를마지막ID로진행한다(t *testing.T) {
	events := make([]Event, EventLimit)
	for index := range events {
		events[index].ID = uint64(101 + index)
	}
	service, err := NewService(fakeReader{events: events})
	if err != nil {
		t.Fatal(err)
	}

	page, err := service.Events(context.Background(), 7, "", 100)

	if err != nil {
		t.Fatal(err)
	}
	if page.NextAfterID != 300 || !page.HasMore {
		t.Fatalf("page=%+v", page)
	}
}
