package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func TestCollect_필수Flag누락_오류를반환한다(t *testing.T) {
	for _, args := range [][]string{
		{"collect", "--to", "2026-07-28"},
		{"collect", "--from", "2026-07-28"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root, _ := newTestRoot(t)
			root.SetArgs(args)

			err := root.Execute()

			if err == nil {
				t.Fatal("Execute() error = nil")
			}
			if !strings.Contains(err.Error(), "required flag") {
				t.Fatalf("Execute() error = %q", err)
			}
		})
	}
}

func TestCollect_여러품목과기간을전달하고집계결과를출력한다(t *testing.T) {
	var captured weblist.JobCommand
	run := func(_ context.Context, _ config.Config, command weblist.JobCommand) (weblist.JobResult, error) {
		captured = command
		unit := weblist.Result{
			RunStatus: weblist.RunStatusCompleted, PartitionStatus: weblist.PartitionStatusDone,
			FetchedPages: 1, ParsedRows: 2, UniqueRCNOCount: 2, NewRCNOCount: 1,
		}
		return weblist.JobResult{
			RunID: 31, Status: weblist.RunStatusCompleted,
			TotalPartitions: 2, CompletedPartitions: 2, FetchedPages: 2,
			ParsedRows: 4, UniqueRCNOCount: 4, NewRCNOCount: 2,
			Units: []weblist.Result{unit, unit},
		}, nil
	}
	output := &strings.Builder{}
	command, err := newCollectCommand(func() config.Config {
		return config.Config{Targets: []config.TargetItem{
			{Name: "위스키"}, {Name: "브랜디"}, {Name: "일반증류주"}, {Name: "리큐르"},
		}}
	}, run, output)
	if err != nil {
		t.Fatal(err)
	}
	command.SetArgs([]string{
		"--from", "2023-07-03", "--to", "2023-08-02", "--workers", "12",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if captured.Workers != 12 || len(captured.ItemNames) != 0 ||
		captured.FromDate != "2023-07-03" || captured.ToDate != "2023-08-02" {
		t.Fatalf("captured = %+v", captured)
	}
	want := "collect 완료: job_id=31 items=4 tasks=2 fetches=2 parsed_rows=4 unique_rcno=4 new_rcno=2 job_status=COMPLETED\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestCollect_Workers미지정시Config기본값을사용한다(t *testing.T) {
	var captured weblist.JobCommand
	run := func(_ context.Context, _ config.Config, command weblist.JobCommand) (weblist.JobResult, error) {
		captured = command
		return weblist.JobResult{
			RunID: 32, Status: weblist.RunStatusCompleted,
			TotalPartitions: 1, CompletedPartitions: 1,
		}, nil
	}
	command, err := newCollectCommand(func() config.Config {
		return config.Config{Web: config.WebConfig{ListWorkers: 7}}
	}, run, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	command.SetArgs([]string{
		"--from", "2025-01-01", "--to", "2025-01-01",
	})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if captured.Workers != 7 {
		t.Fatalf("workers=%d", captured.Workers)
	}
}
