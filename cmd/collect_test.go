package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func TestCollectRecent_KST오늘을포함한최근7일을수집한다(t *testing.T) {
	var captured weblist.JobCommand
	run := func(_ context.Context, _ config.Config, command weblist.JobCommand) (weblist.JobResult, error) {
		captured = command
		return weblist.JobResult{
			RunID: 41, Status: weblist.RunStatusCompleted,
			TotalPartitions: 7, CompletedPartitions: 7,
		}, nil
	}
	output := &strings.Builder{}
	command := newCollectRecentCommand(func() config.Config {
		return config.Config{
			Web:     config.WebConfig{ListWorkers: 2},
			Targets: make([]config.TargetItem, 4),
		}
	}, run, output, func() time.Time {
		return time.Date(2026, 8, 11, 15, 30, 0, 0, time.UTC)
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.FromDate != "2026-08-06" || captured.ToDate != "2026-08-12" || captured.Workers != 2 {
		t.Fatalf("captured = %+v", captured)
	}
	want := "collect-recent 완료: from=2026-08-06 to=2026-08-12 job_id=41 items=4 tasks=7 fetches=0 parsed_rows=0 unique_rcno=0 new_rcno=0 job_status=COMPLETED\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestCollectRecent_KST자정직전에는이전날을오늘로사용한다(t *testing.T) {
	fromDate, toDate, err := recentCollectionRange(time.Date(2026, 8, 11, 14, 59, 0, 0, time.UTC))

	if err != nil {
		t.Fatal(err)
	}
	if fromDate != "2026-08-05" || toDate != "2026-08-11" {
		t.Fatalf("range = %s..%s", fromDate, toDate)
	}
}

func TestCollectRecent_인자를받지않는다(t *testing.T) {
	command := newCollectRecentCommand(
		func() config.Config { return config.Config{} },
		successfulWebListJob,
		&strings.Builder{},
		time.Now,
	)
	command.SetArgs([]string{"2026-08-12"})

	err := command.Execute()

	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Execute() error = %v", err)
	}
}
