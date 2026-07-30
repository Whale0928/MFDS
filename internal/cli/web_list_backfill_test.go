package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

func TestWebListBackfill_필수Flag누락_오류를반환한다(t *testing.T) {
	for _, args := range [][]string{
		{"web", "list-backfill", "--date", "2026-07-28"},
		{"web", "list-backfill", "--item", "위스키"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root, _ := newTestRoot(t, noOpTUI)
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

func TestWebListBackfill_수집성공_결과를한줄출력한다(t *testing.T) {
	processDate := time.Date(2026, time.July, 28, 0, 0, 0, 0, time.FixedZone("Asia/Seoul", 9*60*60))
	run := func(context.Context, config.Config, weblist.Command) (weblist.Result, error) {
		return weblist.Result{
			RunID:           11,
			PartitionID:     21,
			ItemName:        "위스키",
			ItemCode:        "C0314210000000000000",
			ProcessDate:     processDate,
			RunStatus:       weblist.RunStatusCompleted,
			PartitionStatus: weblist.PartitionStatusDone,
			ExpectedTotal:   20, ExpectedPages: 2, FetchedPages: 2,
			ParsedRows: 20, UniqueRCNOCount: 20, NewRCNOCount: 7,
		}, nil
	}
	root, output := newTestRootWithRunner(t, noOpTUI, &fakeDatabase{}, run)
	root.SetArgs([]string{"web", "list-backfill", "--item", "위스키", "--date", "2026-07-28"})

	err := root.Execute()

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "web list-backfill 완료: run_id=11 partition_id=21 item=위스키 item_code=C0314210000000000000 process_date=2026-07-28 total=20 expected_pages=2 fetched_pages=2 parsed_rows=20 unique_rcno=20 new_rcno=7 run_status=COMPLETED partition_status=DONE\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestWebListBackfill_준비실패_성공출력을남기지않는다(t *testing.T) {
	run := func(context.Context, config.Config, weblist.Command) (weblist.Result, error) {
		return weblist.Result{}, errors.New("transaction 실패")
	}
	root, output := newTestRootWithRunner(t, noOpTUI, &fakeDatabase{}, run)
	root.SetArgs([]string{"web", "list-backfill", "--item", "위스키", "--date", "2026-07-28"})

	err := root.Execute()

	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWebListBackfill_Nonterminal결과_완료출력을남기지않는다(t *testing.T) {
	run := func(context.Context, config.Config, weblist.Command) (weblist.Result, error) {
		return weblist.Result{
			RunStatus: weblist.RunStatusCreated, PartitionStatus: weblist.PartitionStatusPending,
		}, nil
	}
	root, output := newTestRootWithRunner(t, noOpTUI, &fakeDatabase{}, run)
	root.SetArgs([]string{"web", "list-backfill", "--item", "위스키", "--date", "2026-07-28"})

	err := root.Execute()

	if err == nil || output.Len() != 0 {
		t.Fatalf("error=%v output=%q", err, output.String())
	}
}

func TestWebListJob_여러품목과기간을전달하고집계결과를출력한다(t *testing.T) {
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
	command, err := newWebListJobCommand(func() config.Config {
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
	want := "web list-job 완료: job_id=31 items=4 tasks=2 fetches=2 parsed_rows=4 unique_rcno=4 new_rcno=2 job_status=COMPLETED\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestWebListJob_Workers미지정시Config기본값을사용한다(t *testing.T) {
	var captured weblist.JobCommand
	run := func(_ context.Context, _ config.Config, command weblist.JobCommand) (weblist.JobResult, error) {
		captured = command
		return weblist.JobResult{
			RunID: 32, Status: weblist.RunStatusCompleted,
			TotalPartitions: 1, CompletedPartitions: 1,
		}, nil
	}
	command, err := newWebListJobCommand(func() config.Config {
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
