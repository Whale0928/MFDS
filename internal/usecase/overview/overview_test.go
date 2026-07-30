package overview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingReader struct {
	limit    int32
	snapshot Snapshot
	detail   RunDetail
	err      error
}

func (r *recordingReader) LoadOverview(_ context.Context, limit int32) (Snapshot, error) {
	r.limit = limit
	return r.snapshot, r.err
}

func (r *recordingReader) LoadRunDetail(_ context.Context, _ uint64) (RunDetail, error) {
	return r.detail, r.err
}

func TestService_Load_최근실행제한과조회시각을적용한다(t *testing.T) {
	reader := &recordingReader{snapshot: Snapshot{}}
	service, err := NewService(reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	snapshot, err := service.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reader.limit != RecentRunLimit {
		t.Fatalf("limit = %d", reader.limit)
	}
	if !snapshot.LoadedAt.Equal(now) {
		t.Fatalf("LoadedAt = %s", snapshot.LoadedAt)
	}
	if snapshot.RecentRuns == nil {
		t.Fatal("RecentRuns = nil")
	}
}

func TestService_Load_조회오류에문맥을추가한다(t *testing.T) {
	reader := &recordingReader{err: errors.New("database unavailable")}
	service, err := NewService(reader)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "운영 현황 조회 실패") {
		t.Fatalf("error = %v", err)
	}
}

func TestService_LoadRunDetail_빈목록을초기화한다(t *testing.T) {
	reader := &recordingReader{detail: RunDetail{RunID: 41}}
	service, err := NewService(reader)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := service.LoadRunDetail(context.Background(), 41)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Partitions == nil || detail.Pages == nil {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestService_LoadRunDetail_잘못된RunID를거부한다(t *testing.T) {
	reader := &recordingReader{}
	service, err := NewService(reader)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.LoadRunDetail(context.Background(), 0)
	if err == nil || !strings.Contains(err.Error(), "양수") {
		t.Fatalf("error = %v", err)
	}
}
