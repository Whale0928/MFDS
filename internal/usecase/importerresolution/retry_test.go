package importerresolution

import (
	"bytes"
	"context"
	"errors"
	"github.com/bottle-note/mfds-crawler/internal/source/mfdscompany"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSyncJob_통신실패를재시도하고다음수입사를처리한다(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(map[bool]string{false: "exhausted", true: "recovered"}[recover], func(t *testing.T) {
			store := &fakeStore{groups: []PendingGroup{{BusinessName: "first", Records: []PendingRecord{{RCNO: "1"}}}, {BusinessName: "second", Records: []PendingRecord{{RCNO: "2"}}}}}
			calls := map[string]int{}
			source := fakeSource{search: func(r mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
				calls[r.BusinessName]++
				if r.BusinessName == "first" && (!recover || calls[r.BusinessName] < 3) {
					return mfdscompany.SearchPage{}, &mfdscompany.RequestError{Endpoint: mfdscompany.ListPath, Err: context.DeadlineExceeded}
				}
				return mfdscompany.SearchPage{}, nil
			}}
			var logs bytes.Buffer
			s, err := NewService(store, source, Options{PageSize: 50, Industry: "141", RetryDelays: []time.Duration{0}, Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.SyncJob(context.Background(), 42)
			if err != nil {
				t.Fatal(err)
			}
			if calls["first"] != 3 || calls["second"] != 1 {
				t.Fatalf("calls=%v", calls)
			}
			wantFailed := 1
			wantUnresolved := 1
			if recover {
				wantFailed = 0
				wantUnresolved = 2
			}
			if result.FailedGroups != wantFailed || result.FailedRCNOs != wantFailed || result.Unresolved != wantUnresolved {
				t.Fatalf("summary=%+v", result)
			}
			for _, want := range []string{`"code":"HTTP_TIMEOUT"`, `"job_id":42`, `"business_name":"first"`, `"endpoint":"/CFCCC01F02/getList"`, `"attempt":1`} {
				if !strings.Contains(logs.String(), want) {
					t.Fatalf("missing %s: %s", want, logs.String())
				}
			}
			if !recover && !strings.Contains(logs.String(), "IMPORTER_RETRY_EXHAUSTED") {
				t.Fatal("missing exhausted event")
			}
		})
	}
}

func TestSyncJob_CancelDuringRetryStopsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	source := fakeSource{search: func(mfdscompany.SearchRequest) (mfdscompany.SearchPage, error) {
		calls++
		cancel()
		return mfdscompany.SearchPage{}, context.DeadlineExceeded
	}}
	store := &fakeStore{groups: []PendingGroup{{BusinessName: "first"}, {BusinessName: "second"}}}
	_, err := newTestService(t, store, source).SyncJob(ctx, 42)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
