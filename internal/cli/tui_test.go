package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bottle-note/mfds-crawler/internal/config"
	storemysql "github.com/bottle-note/mfds-crawler/internal/store/mysql"
	"github.com/bottle-note/mfds-crawler/internal/usecase/ledger"
	"github.com/bottle-note/mfds-crawler/internal/usecase/operator"
	"github.com/bottle-note/mfds-crawler/internal/usecase/overview"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type tuiDatabase struct {
	snapshot      overview.Snapshot
	detail        overview.RunDetail
	eventPage     operator.EventPage
	eventAfterIDs []uint64
	loadErr       error
}

var testTUITargets = []config.TargetItem{
	{Name: "위스키"},
	{Name: "브랜디"},
	{Name: "일반증류주"},
	{Name: "리큐르"},
}

func (d *tuiDatabase) Ping(context.Context) error {
	return nil
}

func (d *tuiDatabase) Migrate(context.Context) error {
	return nil
}

func (d *tuiDatabase) MigrationStatus(context.Context) ([]storemysql.MigrationStatus, error) {
	return []storemysql.MigrationStatus{{Version: 1, Applied: true}}, nil
}

func (d *tuiDatabase) LoadOverview(_ context.Context, limit int32) (overview.Snapshot, error) {
	if limit != overview.RecentRunLimit {
		return overview.Snapshot{}, errors.New("unexpected limit")
	}
	return d.snapshot, d.loadErr
}

func (d *tuiDatabase) LoadRunDetail(_ context.Context, _ uint64) (overview.RunDetail, error) {
	return d.detail, d.loadErr
}

func (d *tuiDatabase) LoadEvents(
	_ context.Context,
	_ uint64,
	_ string,
	afterID uint64,
	_ int32,
) (operator.EventPage, error) {
	d.eventAfterIDs = append(d.eventAfterIDs, afterID)
	return d.eventPage, d.loadErr
}

func (d *tuiDatabase) LoadPageItems(context.Context, uint64) ([]operator.Item, error) {
	return []operator.Item{}, d.loadErr
}

func (d *tuiDatabase) Close() error {
	return nil
}

func TestTUIModel_Database결과_단계별현황과최근실행을표시한다(t *testing.T) {
	processDate := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	model := tuiModel{
		cfg:     config.Config{Targets: []config.TargetItem{{Name: "위스키"}}},
		dbState: "확인 중",
		busy:    true,
		width:   100,
	}
	result, _ := model.Update(databaseResultMsg{
		migrationVersion: 1,
		snapshot: overview.Snapshot{
			TotalRuns:            4,
			CompletedRuns:        3,
			ListRawRows:          27,
			UniqueRCNO:           21,
			DetailQueued:         21,
			GlobalPendingDetails: 21,
			RecentRuns: []overview.Run{{
				ID:                41,
				RequestedFromDate: processDate,
				RequestedToDate:   processDate,
				Status:            "COMPLETED",
				ParsedRows:        4,
				NewRCNOCount:      2,
			}},
		},
	})
	view := result.(tuiModel).View()

	for _, expected := range []string{
		"DB 정상 · schema v1",
		"01  수집",
		"R27 C21",
		"02  상세",
		"S0  P21",
		"03  정제",
		"LOCKED",
		"41   2026-07-28 COMPLETED",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("View()에 %q 없음:\n%s", expected, view)
		}
	}
}

func TestTUIModel_아래키_선택실행의오류를표시한다(t *testing.T) {
	model := tuiModel{
		dashboard: overview.Snapshot{RecentRuns: []overview.Run{
			{ID: 2, Status: "COMPLETED"},
			{ID: 1, Status: "PARTIAL_FAILED", LastError: "parse failed\ninvalid row"},
		}},
		width: 80,
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	selected := updated.(tuiModel)
	if selected.cursor != 1 {
		t.Fatalf("cursor = %d", selected.cursor)
	}
	if !strings.Contains(selected.View(), "선택 오류  parse failed invalid row") {
		t.Fatalf("View() = %q", selected.View())
	}
}

func TestTUIModel_CheckDatabase_조회오류를연결실패로표시한다(t *testing.T) {
	database := &tuiDatabase{loadErr: errors.New("query failed")}
	model := tuiModel{
		ctx: context.Background(),
		openDatabase: func(config.DatabaseConfig) (Database, error) {
			return database, nil
		},
		busy: true,
	}

	message := model.checkDatabase()()
	updated, _ := model.Update(message)
	failed := updated.(tuiModel)
	if failed.dbState != "연결 실패" {
		t.Fatalf("dbState = %q", failed.dbState)
	}
	if !strings.Contains(failed.message, "운영 현황 조회 실패") {
		t.Fatalf("message = %q", failed.message)
	}
}

func TestTUIModel_View_화면높이에맞춰최근실행을제한한다(t *testing.T) {
	runs := make([]overview.Run, 12)
	for index := range runs {
		runs[index] = overview.Run{ID: uint64(12 - index)}
	}
	model := tuiModel{
		dashboard: overview.Snapshot{RecentRuns: runs},
		cursor:    0,
		height:    24,
	}

	view := model.View()
	if lines := strings.Count(view, "\n"); lines > 24 {
		t.Fatalf("line count = %d\n%s", lines, view)
	}
	if strings.Contains(view, "   6      ") {
		t.Fatalf("화면 밖 run이 렌더링됨:\n%s", view)
	}
}

func TestTUIModel_수집폼_선택한품목과처리일로수집하고현황을갱신한다(t *testing.T) {
	var command weblist.JobCommand
	model := tuiModel{
		ctx: context.Background(),
		cfg: config.Config{
			Timezone: "Asia/Seoul",
			Targets:  testTUITargets,
		},
		runJob: func(_ context.Context, _ config.Config, received weblist.JobCommand) (weblist.JobResult, error) {
			command = received
			received.OnStarted(42)
			return weblist.JobResult{RunID: 42, Status: weblist.RunStatusCompleted}, nil
		},
		jobUpdates: make(chan tea.Msg, 4),
	}

	collecting, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	form := collecting.(tuiModel)
	form.collectFrom = "2026-07-28"
	form.collectTo = "2026-07-28"
	form.collectFocus = 2
	form.collectWorkers = 2
	running, commandFn := form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if commandFn == nil {
		t.Fatal("collection command = nil")
	}
	resultMessage := commandFn()
	published := resultMessage.(jobPublishedMsg)
	if published.runID != 42 {
		t.Fatalf("published run = %d", published.runID)
	}
	_ = running
	if len(command.ItemNames) != 0 ||
		command.FromDate != "2026-07-28" || command.ToDate != "2026-07-28" ||
		command.Workers != 2 {
		t.Fatalf("command = %#v", command)
	}
}

func TestTUIModel_수집범위_양끝날짜를포함해일자별로수집한다(t *testing.T) {
	var commands []weblist.JobCommand
	model := tuiModel{
		ctx: context.Background(),
		cfg: config.Config{Targets: testTUITargets},
		runJob: func(_ context.Context, _ config.Config, command weblist.JobCommand) (weblist.JobResult, error) {
			commands = append(commands, command)
			command.OnStarted(7)
			return weblist.JobResult{RunID: 7, Status: weblist.RunStatusCompleted}, nil
		},
		mode:           tuiModeCollect,
		collectFocus:   2,
		collectWorkers: 1,
		collectFrom:    "2026-07-27",
		collectTo:      "2026-07-29",
		jobUpdates:     make(chan tea.Msg, 4),
	}

	running, commandFn := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if commandFn == nil {
		t.Fatal("collection command = nil")
	}
	_ = commandFn()
	_ = running
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	if commands[0].FromDate != "2026-07-27" || commands[0].ToDate != "2026-07-29" {
		t.Fatalf("command = %#v", commands[0])
	}
}

func TestTUIModel_최근실행Enter_Partition과Page상세를표시한다(t *testing.T) {
	total := int64(4)
	rows := int32(2)
	database := &tuiDatabase{detail: overview.RunDetail{
		RunID:               41,
		Status:              "COMPLETED",
		TotalPartitions:     1,
		CompletedPartitions: 1,
		FetchedRequests:     2,
		ParsedRows:          4,
		Partitions: []overview.Partition{{
			ID: 9, ItemName: "위스키", Status: "FAILED", ParsedRows: 4, UniqueRCNOCount: 4,
			LastError: "reconciliation failed",
		}},
		Pages: []overview.Page{{
			ID: 20, PartitionID: 9, PageNo: 1, Status: "DONE",
			TotalSnapshot: &total, RowCount: &rows, UniqueRCNOCount: &rows,
		}},
	}}
	model := tuiModel{
		ctx:       context.Background(),
		dashboard: overview.Snapshot{RecentRuns: []overview.Run{{ID: 41}}},
		openDatabase: func(config.DatabaseConfig) (Database, error) {
			return database, nil
		},
		height: 24,
	}

	loading, commandFn := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if commandFn == nil {
		t.Fatal("detail command = nil")
	}
	loaded, _ := loading.(tuiModel).Update(commandFn())
	detailModel := loaded.(tuiModel)
	if detailModel.mode != tuiModeRunDetail {
		t.Fatalf("mode = %d", detailModel.mode)
	}
	view := detailModel.View()
	for _, expected := range []string{
		"RUN 41",
		"COMPLETED",
		"위스키",
		"FAILED",
		"Task 9 오류  reconciliation failed",
		"FETCHES",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("View()에 %q 없음:\n%s", expected, view)
		}
	}
}

func TestTUIModel_View_고정앱셸과세화면탭을표시한다(t *testing.T) {
	model := tuiModel{
		dbState: "정상 · schema v1",
		width:   100,
		height:  24,
	}

	view := model.View()
	for _, expected := range []string{
		"MFDS  CONTROL",
		"1 ▌ Jobs",
		"2 Work",
		"3 New",
		"4 Logs",
		"5 Raw",
		"WORKSPACE",
		"PIPELINE",
		"DATABASE",
		"JOBS",
		"RAW 0  RCNO 0",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("View()에 %q 없음:\n%s", expected, view)
		}
	}
}

func TestTUIModel_숫자탭_Runs와Collect화면을이동한다(t *testing.T) {
	model := tuiModel{
		cfg: config.Config{Targets: testTUITargets},
		dashboard: overview.Snapshot{RecentRuns: []overview.Run{{
			ID: 41, Status: "COMPLETED",
		}}},
	}

	runsResult, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	runs := runsResult.(tuiModel)
	if runs.mode != tuiModeRuns || !strings.Contains(runs.View(), "DISPATCH") {
		t.Fatalf("runs mode = %d\n%s", runs.mode, runs.View())
	}

	collectResult, _ := runs.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	collect := collectResult.(tuiModel)
	if collect.mode != tuiModeCollect || !strings.Contains(collect.View(), "NEW JOB") {
		t.Fatalf("collect mode = %d\n%s", collect.mode, collect.View())
	}
}

func TestTUIModel_Run상세Esc_Runs화면으로돌아간다(t *testing.T) {
	model := tuiModel{mode: tuiModeRunDetail}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(tuiModel)
	if got.mode != tuiModeOverview {
		t.Fatalf("mode = %d", got.mode)
	}
}

func TestTUIModel_수집폼_시작일기본값은종료일의7일전이다(t *testing.T) {
	model := tuiModel{
		cfg: config.Config{
			Timezone: "Asia/Seoul",
			Targets:  testTUITargets,
		},
	}

	opened, _ := model.openCollect()
	form := opened.(tuiModel)
	from, to, err := validateCollectionRange(form.collectFrom, form.collectTo)
	if err != nil {
		t.Fatal(err)
	}
	if days := int(to.Sub(from).Hours() / 24); days != 7 {
		t.Fatalf("range days = %d, from=%s to=%s", days, form.collectFrom, form.collectTo)
	}
}

func TestTUIModel_수집폼_CtrlU로날짜를지우고다시입력한다(t *testing.T) {
	model := tuiModel{
		cfg:          config.Config{Targets: testTUITargets},
		mode:         tuiModeCollect,
		collectFocus: 0,
		collectFrom:  "2026-07-21",
	}

	cleared, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	typed, _ := cleared.(tuiModel).Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("2026-07-01"),
	})

	if got := typed.(tuiModel).collectFrom; got != "2026-07-01" {
		t.Fatalf("collectFrom = %q", got)
	}
}

func TestTUIModel_수집폼_숫자입력시기본날짜를교체하고하이픈을넣는다(t *testing.T) {
	model := tuiModel{
		cfg:          config.Config{Targets: testTUITargets},
		mode:         tuiModeCollect,
		collectFocus: 0,
		collectFrom:  "2026-07-21",
	}

	updated, _ := model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("20260701"),
	})

	got := updated.(tuiModel)
	if got.collectFrom != "2026-07-01" || !got.collectEditing {
		t.Fatalf("collectFrom=%q editing=%t", got.collectFrom, got.collectEditing)
	}
}

func TestTUIModel_수집폼_방향키로필드와날짜를조정한다(t *testing.T) {
	model := tuiModel{
		cfg:          config.Config{Targets: testTUITargets},
		mode:         tuiModeCollect,
		collectFocus: 0,
		collectFrom:  "2026-07-21",
		collectTo:    "2026-07-28",
	}

	previousDay, _ := model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	endField, _ := previousDay.(tuiModel).Update(tea.KeyMsg{Type: tea.KeyDown})
	nextDay, _ := endField.(tuiModel).Update(tea.KeyMsg{Type: tea.KeyRight})

	got := nextDay.(tuiModel)
	if got.collectFocus != 1 || got.collectFrom != "2026-07-20" || got.collectTo != "2026-07-29" {
		t.Fatalf(
			"focus=%d from=%s to=%s",
			got.collectFocus,
			got.collectFrom,
			got.collectTo,
		)
	}
}

func TestTUIModel_수집폼_잘못된날짜는실행하지않는다(t *testing.T) {
	model := tuiModel{
		cfg:            config.Config{Targets: testTUITargets},
		mode:           tuiModeCollect,
		collectFocus:   2,
		collectFrom:    "2026-99-99",
		collectTo:      "2026-07-28",
		collectWorkers: 1,
	}

	updated, commandFn := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if commandFn != nil {
		t.Fatal("command != nil")
	}
	got := updated.(tuiModel)
	if got.mode != tuiModeCollect || !strings.Contains(got.message, "YYYY-MM-DD") {
		t.Fatalf("model = %#v", got)
	}
}

func TestTUIModel_수집폼_시작일이종료일보다늦으면실행하지않는다(t *testing.T) {
	model := tuiModel{
		cfg:            config.Config{Targets: testTUITargets},
		mode:           tuiModeCollect,
		collectFocus:   2,
		collectFrom:    "2026-07-29",
		collectTo:      "2026-07-28",
		collectWorkers: 1,
	}

	updated, commandFn := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if commandFn != nil {
		t.Fatal("command != nil")
	}
	got := updated.(tuiModel)
	if !strings.Contains(got.message, "시작일은 종료일보다 늦을 수 없습니다") {
		t.Fatalf("message = %q", got.message)
	}
}

func TestTUIModel_EventReconnect_마지막Cursor이후만추가한다(t *testing.T) {
	model := tuiModel{eventRunID: 42, activeRunID: 42}
	updated, _ := model.Update(liveResultMsg{
		runID: 42,
		page: operator.EventPage{
			Events:      []operator.Event{{ID: 1}, {ID: 2}},
			NextAfterID: 2,
		},
	})
	model = updated.(tuiModel)

	updated, _ = model.Update(liveResultMsg{
		runID: 42,
		page: operator.EventPage{
			Events:      []operator.Event{{ID: 2}, {ID: 3}},
			NextAfterID: 3,
		},
	})
	model = updated.(tuiModel)

	if model.lastEventID != 3 || len(model.events) != 3 {
		t.Fatalf("lastEventID=%d events=%+v", model.lastEventID, model.events)
	}
}

func TestTUIModel_EventBuffer_최신이벤트만상한까지유지한다(t *testing.T) {
	model := tuiModel{eventCursor: maxTUIEvents - 1}
	events := make([]operator.Event, maxTUIEvents+2)
	for index := range events {
		events[index].ID = uint64(index + 1)
	}

	model.appendEventPage(operator.EventPage{
		Events: events, NextAfterID: uint64(len(events)),
	})

	if len(model.events) != maxTUIEvents || model.events[0].ID != 3 ||
		model.lastEventID != uint64(maxTUIEvents+2) ||
		model.eventCursor != maxTUIEvents-3 {
		t.Fatalf(
			"len=%d first=%d last=%d cursor=%d",
			len(model.events), model.events[0].ID, model.lastEventID, model.eventCursor,
		)
	}
}

func TestTUIModel_LoadLive_보관한Cursor부터재개한다(t *testing.T) {
	database := &tuiDatabase{
		detail: overview.RunDetail{RunID: 42},
		eventPage: operator.EventPage{
			Events: []operator.Event{{ID: 13}}, NextAfterID: 13,
		},
	}
	model := tuiModel{
		ctx: context.Background(), activeRunID: 42, eventRunID: 42, lastEventID: 12,
		openDatabase: func(config.DatabaseConfig) (Database, error) {
			return database, nil
		},
	}

	message := model.loadLive(42)().(liveResultMsg)

	if message.err != nil || len(database.eventAfterIDs) != 1 ||
		database.eventAfterIDs[0] != 12 || message.page.NextAfterID != 13 {
		t.Fatalf("message=%+v afterIDs=%v", message, database.eventAfterIDs)
	}
}

func TestTUIModel_역할별화면_DispatchLogsLedger를분리해표시한다(t *testing.T) {
	processDate := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	model := tuiModel{
		width: 120, height: 32, activeRunID: 42,
		runDetail: overview.RunDetail{
			RunID: 42, Status: "LISTING", TotalPartitions: 4, CompletedPartitions: 2,
			Pages: []overview.Page{{
				ID: 91, ItemName: "위스키", ProcessDate: processDate, PageNo: 2,
				Status: "LEASED", WorkerID: "job-42/collector-01",
			}},
		},
		events: []operator.Event{{
			ID: 3, RunID: 42, PageID: 91, WorkerID: "job-42/collector-01",
			Level: "INFO", Phase: "REQUESTING", Message: "위스키 page 2 요청", CreatedAt: processDate,
		}},
		ledger: ledger.Page{Items: []ledger.Observation{{
			ID: 7, RunID: 42, PartitionID: 8, PageID: 91, FetchID: 10,
			RCNO: "202600000001", ItemName: "위스키", ProductNameKO: "테스트 위스키",
			ProcessedDate: &processDate,
		}}},
	}

	model.mode = tuiModeRuns
	dispatch := model.View()
	for _, expected := range []string{"DISPATCH  ·  JOB 42", "WORKERS", "TASK QUEUE", "job-42/collector-01"} {
		if !strings.Contains(dispatch, expected) {
			t.Fatalf("Dispatch에 %q 없음:\n%s", expected, dispatch)
		}
	}

	model.mode = tuiModeLogs
	logs := model.View()
	for _, expected := range []string{"LIVE LOGS  ·  JOB 42", "ALL", "REQUESTING", "위스키 page 2 요청", "EVENT DETAIL"} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("Logs에 %q 없음:\n%s", expected, logs)
		}
	}

	model.mode = tuiModeLedger
	ledger := model.View()
	for _, expected := range []string{"LEDGER", "202600000001", "테스트 위스키", "PROVENANCE", "Fetch 91"} {
		if !strings.Contains(ledger, expected) {
			t.Fatalf("Ledger에 %q 없음:\n%s", expected, ledger)
		}
	}
}

func TestTUIModel_Job집계결과를표시한다(t *testing.T) {
	model := tuiModel{}

	updated, commandFn := model.Update(jobResultMsg{
		result: weblist.JobResult{
			RunID: 9, Status: weblist.RunStatusCompleted,
			TotalPartitions: 4, CompletedPartitions: 4, ParsedRows: 12, NewRCNOCount: 3,
		},
	})
	if commandFn == nil {
		t.Fatal("refresh command = nil")
	}
	got := updated.(tuiModel)
	if !strings.Contains(got.message, "Task 4/4") ||
		!strings.Contains(got.message, "Item 12행") {
		t.Fatalf("message = %q", got.message)
	}
}
