package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bottle-note/mfds-crawler/internal/config"
	"github.com/bottle-note/mfds-crawler/internal/usecase/ledger"
	"github.com/bottle-note/mfds-crawler/internal/usecase/operator"
	"github.com/bottle-note/mfds-crawler/internal/usecase/overview"
	"github.com/bottle-note/mfds-crawler/internal/usecase/weblist"
)

type tuiMode uint8

const (
	tuiModeOverview tuiMode = iota
	tuiModeRuns
	tuiModeRunDetail
	tuiModeCollect
	tuiModeLogs
	tuiModeLedger
	tuiModePageItems
	maxTUIEvents = 2000
)

type TUIDependencies struct {
	OpenDatabase  func(config.DatabaseConfig) (Database, error)
	RunWebListJob RunWebListJobFunc
}

type databaseResultMsg struct {
	migrationVersion int64
	snapshot         overview.Snapshot
	err              error
}

type migrationResultMsg struct {
	err error
}

type runDetailResultMsg struct {
	detail overview.RunDetail
	err    error
}

type jobPublishedMsg struct {
	runID uint64
}

type jobResultMsg struct {
	result weblist.JobResult
	err    error
}

type liveResultMsg struct {
	runID    uint64
	workerID string
	detail   overview.RunDetail
	page     operator.EventPage
	err      error
}

type ledgerResultMsg struct {
	ledger ledger.Page
	err    error
}

type pageItemsResultMsg struct {
	pageID uint64
	items  []operator.Item
	err    error
}

type refreshTickMsg time.Time

type tuiModel struct {
	ctx            context.Context
	cfg            config.Config
	openDatabase   func(config.DatabaseConfig) (Database, error)
	runJob         RunWebListJobFunc
	dashboard      overview.Snapshot
	runDetail      overview.RunDetail
	events         []operator.Event
	pageItems      []operator.Item
	ledger         ledger.Page
	dbState        string
	message        string
	busy           bool
	mode           tuiMode
	cursor         int
	pageCursor     int
	collectFocus   int
	collectTarget  int
	collectTargets []bool
	collectWorkers int
	collectFrom    string
	collectTo      string
	collectEditing bool
	eventScope     int
	eventCursor    int
	ledgerCursor   int
	itemCursor     int
	activeRunID    uint64
	eventRunID     uint64
	eventWorkerID  string
	lastEventID    uint64
	ledgerBeforeID uint64
	jobUpdates     chan tea.Msg
	width          int
	height         int
}

func RunTUI(
	ctx context.Context,
	cfg config.Config,
	deps TUIDependencies,
	out io.Writer,
) error {
	if deps.OpenDatabase == nil || deps.RunWebListJob == nil {
		return fmt.Errorf("TUI 의존성이 모두 필요합니다")
	}
	configureTUIColor()
	model := tuiModel{
		ctx:            ctx,
		cfg:            cfg,
		openDatabase:   deps.OpenDatabase,
		runJob:         deps.RunWebListJob,
		dbState:        "확인 중",
		busy:           true,
		width:          100,
		height:         24,
		collectWorkers: max(1, cfg.Web.ListWorkers),
		ledgerBeforeID: math.MaxUint64,
		jobUpdates:     make(chan tea.Msg, 4),
	}
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(out))
	_, err := program.Run()
	if err != nil {
		return fmt.Errorf("TUI 실행 실패: %w", err)
	}
	return nil
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.checkDatabase(), refreshTick())
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch msg.String() {
		case "1":
			m.mode = tuiModeOverview
			m.message = ""
			return m, nil
		case "2":
			m.mode = tuiModeRuns
			m.message = ""
			return m.openDispatch()
		case "3":
			return m.openCollect()
		case "4":
			m.mode = tuiModeLogs
			m.message = ""
			return m.openLive()
		case "5":
			m.mode = tuiModeLedger
			m.message = ""
			m.ledgerBeforeID = math.MaxUint64
			return m, m.loadLedger(m.ledgerBeforeID)
		}
		switch m.mode {
		case tuiModeRunDetail:
			return m.updateRunDetail(msg)
		case tuiModeCollect:
			return m.updateCollect(msg)
		case tuiModeLogs:
			return m.updateLogs(msg)
		case tuiModeLedger:
			return m.updateLedger(msg)
		case tuiModePageItems:
			return m.updatePageItems(msg)
		case tuiModeRuns:
			return m.updateDispatch(msg)
		default:
			return m.updateOverview(msg)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case databaseResultMsg:
		m.busy = false
		if msg.err != nil {
			m.dbState = "연결 실패"
			m.message = msg.err.Error()
		} else {
			m.dbState = fmt.Sprintf("정상 · schema v%d", msg.migrationVersion)
			m.dashboard = msg.snapshot
			if m.cursor >= len(msg.snapshot.RecentRuns) {
				m.cursor = max(0, len(msg.snapshot.RecentRuns)-1)
			}
			if m.activeRunID == 0 && len(msg.snapshot.RecentRuns) > 0 {
				m.activeRunID = msg.snapshot.RecentRuns[0].ID
			}
		}
	case migrationResultMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.message = "migration 적용 완료"
			return m, m.checkDatabase()
		}
	case runDetailResultMsg:
		m.busy = false
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.runDetail = msg.detail
			m.pageCursor = 0
			m.mode = tuiModeRunDetail
			m.message = ""
		}
	case jobPublishedMsg:
		m.events = nil
		m.eventRunID = msg.runID
		m.eventWorkerID = ""
		m.lastEventID = 0
		m.activeRunID = msg.runID
		m.mode = tuiModeRuns
		m.busy = false
		m.message = fmt.Sprintf("Job %d 발행 완료 · worker %d개 실행 중", msg.runID, m.collectWorkers)
		return m, tea.Batch(m.loadLive(msg.runID), m.waitJobUpdate())
	case jobResultMsg:
		m.busy = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Job %d 종료 · %v", msg.result.RunID, msg.err)
		} else {
			m.message = fmt.Sprintf(
				"Job %d %s · Task %d/%d · Item %d행 · 신규 RCNO %d개",
				msg.result.RunID, msg.result.Status, msg.result.CompletedPartitions,
				msg.result.TotalPartitions, msg.result.ParsedRows, msg.result.NewRCNOCount,
			)
		}
		return m, tea.Batch(m.checkDatabase(), m.loadLive(msg.result.RunID))
	case liveResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			break
		}
		if m.eventRunID != msg.runID || m.eventWorkerID != msg.workerID {
			m.events = nil
			m.lastEventID = 0
			m.eventRunID = msg.runID
			m.eventWorkerID = msg.workerID
		}
		m.activeRunID = msg.runID
		m.runDetail = msg.detail
		m.appendEventPage(msg.page)
		if m.eventScope >= m.workerScopeCount() {
			m.eventScope = 0
		}
		if m.eventCursor >= len(m.events) {
			m.eventCursor = max(0, len(m.events)-1)
		}
		if msg.page.HasMore {
			return m, m.loadLive(msg.runID)
		}
	case ledgerResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.ledger = msg.ledger
			if m.ledgerCursor >= len(m.ledger.Items) {
				m.ledgerCursor = max(0, len(m.ledger.Items)-1)
			}
		}
	case pageItemsResultMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.pageItems = msg.items
			m.itemCursor = 0
			m.mode = tuiModePageItems
			m.message = ""
		}
	case refreshTickMsg:
		var commands []tea.Cmd
		commands = append(commands, refreshTick())
		if m.activeRunID != 0 && (m.mode == tuiModeRuns || m.mode == tuiModeRunDetail || m.mode == tuiModeLogs) {
			commands = append(commands, m.loadLive(m.activeRunID))
		}
		return m, tea.Batch(commands...)
	}
	return m, nil
}

func (m *tuiModel) appendEventPage(page operator.EventPage) {
	for _, event := range page.Events {
		if event.ID <= m.lastEventID {
			continue
		}
		m.events = append(m.events, event)
		m.lastEventID = event.ID
	}
	if page.NextAfterID > m.lastEventID {
		m.lastEventID = page.NextAfterID
	}
	if overflow := len(m.events) - maxTUIEvents; overflow > 0 {
		m.events = append([]operator.Event(nil), m.events[overflow:]...)
		m.eventCursor = max(0, m.eventCursor-overflow)
	}
}

func (m *tuiModel) resetEventCursor() {
	m.events = nil
	m.eventCursor = 0
	m.eventWorkerID = ""
	m.lastEventID = 0
}

func (m tuiModel) updateOverview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.dashboard.RecentRuns)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if selected, ok := m.selectedRun(); ok {
			m.busy = true
			m.message = fmt.Sprintf("run %d 상세 조회 중", selected.ID)
			return m, m.loadRunDetail(selected.ID)
		}
	case "c":
		return m.openCollect()
	case "d":
		return m.openDispatch()
	case "l":
		m.mode = tuiModeLogs
		return m.openLive()
	case "g":
		m.mode = tuiModeLedger
		m.ledgerBeforeID = math.MaxUint64
		return m, m.loadLedger(m.ledgerBeforeID)
	case "r":
		m.busy = true
		m.dbState = "확인 중"
		m.message = ""
		return m, m.checkDatabase()
	case "m":
		m.busy = true
		m.message = "migration 적용 중"
		return m, m.migrate()
	}
	return m, nil
}

func (m tuiModel) openDispatch() (tea.Model, tea.Cmd) {
	runID := m.selectedOrActiveRunID()
	m.mode = tuiModeRuns
	if runID == 0 {
		m.message = "Dispatch에서 확인할 Job이 없습니다"
		return m, nil
	}
	m.activeRunID = runID
	return m, m.loadLive(runID)
}

func (m tuiModel) openLive() (tea.Model, tea.Cmd) {
	runID := m.selectedOrActiveRunID()
	if runID == 0 {
		m.message = "로그를 확인할 Job이 없습니다"
		return m, nil
	}
	m.activeRunID = runID
	return m, m.loadLive(runID)
}

func (m tuiModel) updateDispatch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.pageCursor < len(m.runDetail.Pages)-1 {
			m.pageCursor++
		}
	case "k", "up":
		if m.pageCursor > 0 {
			m.pageCursor--
		}
	case "enter":
		m.mode = tuiModeRunDetail
	case "l":
		m.mode = tuiModeLogs
	case "r":
		return m, m.loadLive(m.activeRunID)
	}
	return m, nil
}

func (m tuiModel) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	scopeCount := m.workerScopeCount()
	switch msg.String() {
	case "left", "h":
		m.eventScope = (m.eventScope - 1 + scopeCount) % scopeCount
		m.resetEventCursor()
		return m, m.loadLive(m.activeRunID)
	case "right", "l":
		m.eventScope = (m.eventScope + 1) % scopeCount
		m.resetEventCursor()
		return m, m.loadLive(m.activeRunID)
	case "j", "down":
		if m.eventCursor < len(m.events)-1 {
			m.eventCursor++
		}
	case "k", "up":
		if m.eventCursor > 0 {
			m.eventCursor--
		}
	case "r":
		return m, m.loadLive(m.activeRunID)
	}
	return m, nil
}

func (m tuiModel) updateLedger(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.ledgerCursor < len(m.ledger.Items)-1 {
			m.ledgerCursor++
		}
	case "k", "up":
		if m.ledgerCursor > 0 {
			m.ledgerCursor--
		}
	case "r":
		m.ledgerBeforeID = math.MaxUint64
		return m, m.loadLedger(m.ledgerBeforeID)
	case "n":
		if m.ledger.HasMore {
			m.ledgerBeforeID = m.ledger.NextBeforeID
			return m, m.loadLedger(m.ledgerBeforeID)
		}
	}
	return m, nil
}

func (m tuiModel) updatePageItems(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.mode = tuiModeRunDetail
	case "j", "down":
		if m.itemCursor < len(m.pageItems)-1 {
			m.itemCursor++
		}
	case "k", "up":
		if m.itemCursor > 0 {
			m.itemCursor--
		}
	}
	return m, nil
}

func (m tuiModel) updateRuns(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.dashboard.RecentRuns)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if selected, ok := m.selectedRun(); ok {
			m.busy = true
			m.message = fmt.Sprintf("run %d 상세 조회 중", selected.ID)
			return m, m.loadRunDetail(selected.ID)
		}
	case "c":
		return m.openCollect()
	case "r":
		m.busy = true
		m.dbState = "확인 중"
		m.message = ""
		return m, m.checkDatabase()
	}
	return m, nil
}

func (m tuiModel) openCollect() (tea.Model, tea.Cmd) {
	if len(m.cfg.Targets) != 4 {
		m.message = "고정 수집 대상 4개가 필요합니다"
		return m, nil
	}
	m.mode = tuiModeCollect
	m.collectFocus = 0
	m.collectTo = m.defaultCollectionDate()
	to, err := time.Parse(time.DateOnly, m.collectTo)
	if err != nil {
		m.message = "기본 수집 날짜 계산에 실패했습니다"
		return m, nil
	}
	m.collectFrom = to.AddDate(0, 0, -7).Format(time.DateOnly)
	m.collectEditing = false
	m.message = ""
	return m, nil
}

func (m tuiModel) updateRunDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.mode = tuiModeOverview
		m.message = ""
	case "j", "down":
		if m.pageCursor < len(m.runDetail.Pages)-1 {
			m.pageCursor++
		}
	case "k", "up":
		if m.pageCursor > 0 {
			m.pageCursor--
		}
	case "r":
		m.busy = true
		m.message = fmt.Sprintf("run %d 상세 새로고침 중", m.runDetail.RunID)
		return m, m.loadRunDetail(m.runDetail.RunID)
	case "enter":
		if selected, ok := m.selectedPage(); ok {
			return m, m.loadPageItems(selected.ID)
		}
	}
	return m, nil
}

func (m tuiModel) updateCollect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	const fieldCount = 3
	const fromIndex = 0
	const toIndex = 1
	const workersIndex = 2
	switch msg.String() {
	case "esc":
		m.mode = tuiModeOverview
		m.message = ""
	case "tab", "shift+tab":
		if msg.String() == "shift+tab" {
			m.collectFocus = (m.collectFocus - 1 + fieldCount) % fieldCount
		} else {
			m.collectFocus = (m.collectFocus + 1) % fieldCount
		}
		m.collectEditing = false
	case "up", "k":
		m.collectFocus = (m.collectFocus - 1 + fieldCount) % fieldCount
		m.collectEditing = false
	case "down", "j":
		m.collectFocus = (m.collectFocus + 1) % fieldCount
		m.collectEditing = false
	case "left", "h":
		if m.collectFocus == workersIndex {
			m.collectWorkers = max(1, m.collectWorkers-1)
		} else if m.collectFocus == fromIndex || m.collectFocus == toIndex {
			m.shiftFocusedDate(-1)
		}
	case "right", "l":
		if m.collectFocus == workersIndex {
			m.collectWorkers++
		} else if m.collectFocus == fromIndex || m.collectFocus == toIndex {
			m.shiftFocusedDate(1)
		}
	case "backspace":
		if field := m.focusedDate(); field != nil {
			if !m.collectEditing {
				*field = ""
				m.collectEditing = true
			} else if *field != "" {
				runes := []rune(*field)
				*field = string(runes[:len(runes)-1])
			}
		}
	case "ctrl+u":
		if field := m.focusedDate(); field != nil {
			*field = ""
			m.collectEditing = true
		}
	case "enter":
		if m.collectFocus < workersIndex {
			m.collectFocus++
			m.collectEditing = false
			return m, nil
		}
		from, to, err := validateCollectionRange(m.collectFrom, m.collectTo)
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		m.busy = true
		m.message = fmt.Sprintf("고정 4개 품목 · %s ~ %s Job 발행 중", m.collectFrom, m.collectTo)
		return m, m.publishJob(weblist.JobCommand{
			FromDate: from.Format(time.DateOnly), ToDate: to.Format(time.DateOnly),
			Workers: m.collectWorkers,
		})
	default:
		if (m.collectFocus == fromIndex || m.collectFocus == toIndex) && msg.Type == tea.KeyRunes {
			field := m.focusedDate()
			if !m.collectEditing {
				*field = ""
				m.collectEditing = true
			}
			for _, value := range msg.Runes {
				if unicode.IsDigit(value) || value == '-' {
					*field = appendDateInput(*field, value)
				}
			}
			m.message = ""
		}
	}
	return m, nil
}

func (m tuiModel) View() string {
	mainWidth := m.mainContentWidth()
	var content string
	switch m.mode {
	case tuiModeRunDetail:
		content = m.viewRunDetail(mainWidth)
	case tuiModeCollect:
		content = m.viewCollect(mainWidth)
	case tuiModeLogs:
		content = m.viewLogs(mainWidth)
	case tuiModeLedger:
		content = m.viewLedger(mainWidth)
	case tuiModePageItems:
		content = m.viewPageItems(mainWidth)
	case tuiModeRuns:
		content = m.viewDispatch(mainWidth)
	default:
		content = m.viewOverview(mainWidth)
	}
	return m.viewShell(content)
}

func (m tuiModel) viewShell(content string) string {
	width := m.screenWidth()
	height := m.screenHeight()
	sidebarWidth := m.sidebarWidth()
	mainWidth := width - sidebarWidth
	bodyHeight := max(12, height-2)

	top := m.viewTopBar(width)
	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth-3).
		Height(bodyHeight).
		Padding(1).
		Foreground(tuiText).
		Background(tuiSurface).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(tuiRaised).
		Render(m.viewSidebar())
	main := lipgloss.NewStyle().
		Width(mainWidth-4).
		Height(bodyHeight).
		Padding(1, 2).
		Foreground(tuiText).
		Background(tuiCanvas).
		Render(content)
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
	bottom := m.viewStatusBar(width)
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Foreground(tuiText).
		Background(tuiCanvas).
		Render(lipgloss.JoinVertical(lipgloss.Left, top, body, bottom))
}

func (m tuiModel) viewTopBar(width int) string {
	brandWidth := m.sidebarWidth()
	brand := lipgloss.NewStyle().
		Width(brandWidth-2).
		Padding(0, 1).
		Foreground(tuiText).
		Background(tuiRaised).
		Bold(true).
		Render("MFDS  CONTROL")
	labels := []string{"Jobs", "Dispatch", "New Job", "Live Logs", "Ledger"}
	if width < 110 {
		labels = []string{"Jobs", "Work", "New", "Logs", "Raw"}
	}
	tabs := []string{
		renderTopTab("1", labels[0], m.mode == tuiModeOverview || m.mode == tuiModeRunDetail || m.mode == tuiModePageItems),
		renderTopTab("2", labels[1], m.mode == tuiModeRuns),
		renderTopTab("3", labels[2], m.mode == tuiModeCollect),
		renderTopTab("4", labels[3], m.mode == tuiModeLogs),
		renderTopTab("5", labels[4], m.mode == tuiModeLedger),
	}
	tabBar := lipgloss.NewStyle().
		Width(max(1, width-brandWidth)).
		Background(tuiSurface).
		Render(strings.Join(tabs, " "))
	return lipgloss.JoinHorizontal(lipgloss.Top, brand, tabBar)
}

func renderTopTab(key, label string, active bool) string {
	style := lipgloss.NewStyle().
		Foreground(tuiMuted).
		Background(tuiSurface).
		Padding(0, 1)
	if active {
		style = style.Foreground(tuiText).
			Background(tuiRaised).
			Bold(true)
		label = "▌ " + label
	}
	return style.Render(key + " " + label)
}

func (m tuiModel) viewSidebar() string {
	var builder strings.Builder
	fmt.Fprintln(&builder, lipgloss.NewStyle().Foreground(tuiMuted).Bold(true).Render("WORKSPACE"))
	fmt.Fprintln(&builder, lipgloss.NewStyle().Foreground(tuiText).Bold(true).Render("MFDS"))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, renderSidebarItem("Jobs", m.mode == tuiModeOverview || m.mode == tuiModeRunDetail || m.mode == tuiModePageItems))
	fmt.Fprintln(&builder, renderSidebarItem(
		"Dispatch",
		m.mode == tuiModeRuns,
	))
	fmt.Fprintln(&builder, renderSidebarItem("New Job", m.mode == tuiModeCollect))
	fmt.Fprintln(&builder, renderSidebarItem("Live Logs", m.mode == tuiModeLogs))
	fmt.Fprintln(&builder, renderSidebarItem(
		fmt.Sprintf("Ledger  %d", m.dashboard.ListRawRows),
		m.mode == tuiModeLedger,
	))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, lipgloss.NewStyle().Foreground(tuiMuted).Bold(true).Render("PIPELINE"))
	fmt.Fprintln(&builder, renderPipelineStep(tuiCyan, "01", "목록 수집", m.dashboard.ActiveRuns > 0))
	fmt.Fprintln(&builder, renderPipelineStep(tuiBlue, "02", "상세 수집", m.dashboard.DetailWorking > 0))
	fmt.Fprintln(&builder, renderPipelineStep(tuiViolet, "03", "정제 대기", false))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, lipgloss.NewStyle().Foreground(tuiMuted).Bold(true).Render("DATABASE"))
	stateColor := tuiGreen
	stateLabel := "ONLINE"
	if m.dbState == "확인 중" {
		stateColor = tuiAmber
		stateLabel = "CHECKING"
	}
	if m.dbState == "연결 실패" {
		stateColor = tuiRed
		stateLabel = "OFFLINE"
	}
	fmt.Fprint(&builder, lipgloss.NewStyle().Foreground(stateColor).Bold(true).Render("● "))
	fmt.Fprint(&builder, stateLabel)
	return builder.String()
}

func renderSidebarItem(label string, active bool) string {
	style := lipgloss.NewStyle().Foreground(tuiMuted).PaddingLeft(2)
	prefix := "  "
	if active {
		style = style.Foreground(tuiText).Background(tuiRaised).Bold(true)
		prefix = lipgloss.NewStyle().Foreground(tuiCyan).Render("▌ ")
	}
	return style.Width(17).Render(prefix + label)
}

func renderPipelineStep(color lipgloss.Color, index, label string, active bool) string {
	marker := "│"
	if active {
		marker = "●"
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(marker+" "+index) +
		" " + lipgloss.NewStyle().Foreground(tuiMuted).Render(label)
}

func (m tuiModel) viewStatusBar(width int) string {
	left := fmt.Sprintf(
		" DB %s  RAW %d  RCNO %d ",
		m.dbState,
		m.dashboard.ListRawRows,
		m.dashboard.UniqueRCNO,
	)
	keys := "1 Jobs  2 Dispatch  3 New Job  4 Logs  5 Ledger  q 종료"
	switch m.mode {
	case tuiModeRuns:
		keys = "j/k Task  enter Task 목록  l 로그  r 새로고침"
	case tuiModeRunDetail:
		keys = "j/k Fetch  esc Jobs  r 새로고침  q 종료"
	case tuiModeCollect:
		keys = "tab 이동  ←/→ 값  enter 발행"
	case tuiModeLogs:
		keys = "←/→ worker  j/k 이벤트  r 새로고침"
	case tuiModeLedger:
		keys = "j/k 원장 선택  r 새로고침"
	case tuiModePageItems:
		keys = "j/k Item  esc Task 목록"
	default:
		keys = "j/k Job  enter Task 목록  d Dispatch  l Logs"
	}
	if width < 90 {
		switch m.mode {
		case tuiModeRuns:
			keys = "j/k Task  enter 목록  l Logs"
		case tuiModeRunDetail:
			keys = "j/k 이동  esc 뒤로  r 갱신"
		case tuiModeCollect:
			keys = "↑/↓ 이동  space 선택  enter 발행"
		case tuiModeLogs:
			keys = "←/→ worker  j/k 로그"
		case tuiModeLedger:
			keys = "j/k 원장  r 갱신"
		case tuiModePageItems:
			keys = "j/k Item  esc 뒤로"
		default:
			keys = "j/k Job  enter Task  3 New"
		}
	}
	leftStyle := lipgloss.NewStyle().
		Foreground(tuiCanvas).
		Background(tuiCyan).
		Bold(true)
	leftRendered := leftStyle.Render(left)
	right := lipgloss.NewStyle().
		Width(max(1, width-lipgloss.Width(leftRendered)-2)).
		Padding(0, 1).
		Foreground(tuiMuted).
		Background(tuiRaised).
		Align(lipgloss.Right).
		Render(keys)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, right)
}

func (m tuiModel) screenWidth() int {
	if m.width <= 0 {
		return 100
	}
	return max(60, m.width)
}

func (m tuiModel) screenHeight() int {
	if m.height <= 0 {
		return 24
	}
	return max(18, m.height)
}

func (m tuiModel) sidebarWidth() int {
	if m.screenWidth() < 88 {
		return 20
	}
	return 24
}

func (m tuiModel) mainContentWidth() int {
	return max(54, m.screenWidth()-m.sidebarWidth()-4)
}

func (m tuiModel) viewOverview(width int) string {
	var builder strings.Builder
	snapshot := m.dashboard
	theme := newTUITheme(width)

	fmt.Fprintln(&builder, theme.header(
		"JOBS",
		formatLoadedAt(snapshot.LoadedAt),
	))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.pipeline(
		theme.stageCard(
			tuiCyan,
			"01",
			"수집",
			fmt.Sprintf("R%d C%d D%d",
				snapshot.ListRawRows,
				snapshot.UniqueRCNO,
				snapshot.DirtyPartitions,
			),
			fmt.Sprintf("OK%d L%d F%d",
				snapshot.CompletedRuns,
				snapshot.ActiveRuns,
				snapshot.FailedRuns,
			),
		),
		theme.stageCard(
			tuiBlue,
			"02",
			"상세",
			fmt.Sprintf("S%d  P%d", snapshot.DetailStored, snapshot.GlobalPendingDetails),
			fmt.Sprintf("W%d  D%d  R%d",
				snapshot.DetailWorking,
				snapshot.DetailDead,
				snapshot.DetailRawRows,
			),
		),
		theme.stageCard(
			tuiViolet,
			"03",
			"정제",
			"LOCKED",
			"검증 대기",
		),
	))

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.section("JOB HISTORY", len(snapshot.RecentRuns)))
	header := runTableHeader(theme.width)
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(header))
	if len(snapshot.RecentRuns) == 0 {
		fmt.Fprintln(&builder, theme.muted.Render("   아직 Job이 없습니다. 3을 눌러 첫 Job을 발행하세요."))
	}
	end := min(4, len(snapshot.RecentRuns))
	for index := 0; index < end; index++ {
		fmt.Fprintln(&builder, renderRunRow(theme, snapshot.RecentRuns[index], index == m.cursor))
	}
	if selected, ok := m.selectedRun(); ok && selected.LastError != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.errorPanel(
			"선택 오류",
			truncateText(selected.LastError, max(24, width-18)),
		))
	}
	if m.message != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.notice(m.message, m.busy))
	}
	return theme.render(builder.String())
}

func (m tuiModel) viewRuns(width int) string {
	var builder strings.Builder
	theme := newTUITheme(width)
	fmt.Fprintln(&builder, theme.header(
		"RUN HISTORY",
		fmt.Sprintf("%d RUNS", len(m.dashboard.RecentRuns)),
	))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.muted.Render(
		"Job을 선택해 날짜 Task와 품목·페이지별 Fetch를 확인합니다.",
	))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(runTableHeader(theme.width)))
	if len(m.dashboard.RecentRuns) == 0 {
		fmt.Fprintln(&builder, theme.muted.Render("   실행 기록이 없습니다. 3을 눌러 수집을 시작하세요."))
	}
	start, end := m.visibleRunRange()
	for index := start; index < end; index++ {
		fmt.Fprintln(&builder, renderRunRow(theme, m.dashboard.RecentRuns[index], index == m.cursor))
	}
	if selected, ok := m.selectedRun(); ok && selected.LastError != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.errorPanel(
			"선택 오류",
			truncateText(selected.LastError, max(24, width-18)),
		))
	}
	if m.message != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.notice(m.message, m.busy))
	}
	return theme.render(builder.String())
}

func renderRunRow(theme tuiTheme, run overview.Run, selected bool) string {
	if theme.width < 76 {
		plain := fmt.Sprintf(
			"   %-4d %-10s %-14s %d/%d %4d %4d",
			run.ID,
			formatRunDate(run),
			run.Status,
			run.CompletedPartitions,
			run.TotalPartitions,
			run.ParsedRows,
			run.NewRCNOCount,
		)
		if selected {
			return theme.selected.Width(theme.width).Render("▸" + plain[1:])
		}
		status := theme.status(fmt.Sprintf("%-14s", run.Status))
		return fmt.Sprintf(
			"   %-4d %-10s %s %d/%d %4d %4d",
			run.ID,
			formatRunDate(run),
			status,
			run.CompletedPartitions,
			run.TotalPartitions,
			run.ParsedRows,
			run.NewRCNOCount,
		)
	}
	plain := fmt.Sprintf(
		"   %-6d %-12s %-18s %3d/%-3d %9d %11d",
		run.ID,
		formatRunDate(run),
		run.Status,
		run.CompletedPartitions,
		run.TotalPartitions,
		run.ParsedRows,
		run.NewRCNOCount,
	)
	if selected {
		return theme.selected.Width(theme.width).Render("▸" + plain[1:])
	}
	status := theme.status(fmt.Sprintf("%-18s", run.Status))
	return fmt.Sprintf(
		"   %-6d %-12s %s %3d/%-3d %9d %11d",
		run.ID,
		formatRunDate(run),
		status,
		run.CompletedPartitions,
		run.TotalPartitions,
		run.ParsedRows,
		run.NewRCNOCount,
	)
}

func runTableHeader(width int) string {
	if width < 76 {
		return "   ID   DATE       STATUS         PART ROWS RCNO"
	}
	return "   ID     처리일       상태               파티션     원장행    신규 RCNO"
}

func (m tuiModel) viewCollect(width int) string {
	var builder strings.Builder
	theme := newTUITheme(width)
	fmt.Fprintln(&builder, theme.header(
		"NEW JOB",
		fmt.Sprintf("%d WORKERS", m.collectWorkers),
	))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.eyebrow.Render("고정 품목 그룹과 기간을 한 Job으로 발행"))
	description := "날짜마다 Task 1개를 만들고 Task 안에서 4개 품목과 페이지를 순차 조회합니다."
	if width < 70 {
		description = "날짜 Task 안에서 고정 4개 품목을 순차 조회합니다."
	}
	fmt.Fprintln(&builder, theme.muted.Render(description))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.muted.Render("고정 품목 그룹"))
	fmt.Fprintln(&builder, lipgloss.NewStyle().Foreground(tuiText).Padding(0, 1).
		Width(max(30, width-4)).Render("위스키 · 브랜디 · 일반증류주 · 리큐르"))
	const fromIndex = 0
	const toIndex = 1
	const workersIndex = 2
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.input(
		"시작일",
		renderDateField(m.collectFrom, m.collectFocus == fromIndex),
		m.collectFocus == fromIndex,
	))
	fmt.Fprintln(&builder, theme.input(
		"종료일",
		renderDateField(m.collectTo, m.collectFocus == toIndex),
		m.collectFocus == toIndex,
	))
	fmt.Fprintln(&builder, theme.input(
		"Workers",
		fmt.Sprintf("◀  %d  ▶    Enter로 Job 발행", m.collectWorkers),
		m.collectFocus == workersIndex,
	))
	if m.message != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.errorPanel("입력 확인", m.message))
	}
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.muted.Render(
		"날짜 숫자 직접 입력 · ←/→: 날짜/worker 조절",
	))
	return theme.render(builder.String())
}

func (m tuiModel) viewDispatch(width int) string {
	var builder strings.Builder
	theme := newTUITheme(width)
	detail := m.runDetail
	fmt.Fprintln(&builder, theme.header(
		fmt.Sprintf("DISPATCH  ·  JOB %d", detail.RunID),
		theme.status(detail.Status),
	))
	fmt.Fprintln(&builder, theme.muted.Render(
		fmt.Sprintf("Tasks %d/%d  ·  Fetches %d  ·  Items %d",
			detail.CompletedPartitions, detail.TotalPartitions, detail.FetchedRequests, detail.ParsedRows),
	))
	fmt.Fprintln(&builder)
	workerIDs := m.workerIDs(detail.Pages)
	fmt.Fprintln(&builder, theme.section("WORKERS", len(workerIDs)))
	if len(workerIDs) == 0 {
		message := "worker가 날짜 Task를 할당받기 전입니다."
		if detail.Status == "COMPLETED" || detail.Status == "PARTIAL_FAILED" {
			message = "이 Job은 worker 이력 저장 기능 적용 전에 실행되었습니다."
		}
		fmt.Fprintln(&builder, theme.muted.Render("  "+message))
	}
	for _, workerID := range workerIDs {
		var done, active, failed int
		var current string
		for _, page := range detail.Pages {
			if page.WorkerID != workerID {
				continue
			}
			switch page.Status {
			case "DONE":
				done++
			case "FAILED", "PARSE_FAILED":
				failed++
			default:
				active++
				current = fmt.Sprintf("%s %s · Fetch page %d", page.ItemName, page.ProcessDate.Format(time.DateOnly), page.PageNo)
			}
		}
		if current == "" {
			current = "대기 중"
		}
		status := "IDLE"
		color := tuiMuted
		if active > 0 {
			status, color = "RUNNING", tuiBlue
		} else if failed > 0 {
			status, color = "FAILED", tuiRed
		}
		line := fmt.Sprintf("%-14s %-8s  %-34s  done %d  failed %d",
			workerID, status, truncateText(current, 34), done, failed)
		if width < 82 {
			line = fmt.Sprintf("%-12s %-8s  done %d  fail %d",
				truncateText(workerID, 12), status, done, failed)
		}
		fmt.Fprintln(&builder, lipgloss.NewStyle().
			Foreground(tuiText).Background(tuiSurface).
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(color).Padding(0, 1).Width(max(40, width-4)).Render(line))
	}
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.section("TASK QUEUE", len(detail.Pages)))
	dispatchHeader := "   FETCH    WORKER         품목          처리일       PAGE  STATUS            ITEMS"
	if width < 82 {
		dispatchHeader = "   FETCH  WORKER      PAGE  STATUS          ITEMS"
	}
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(dispatchHeader))
	start, end := m.dispatchPageRange()
	for index := start; index < end; index++ {
		page := detail.Pages[index]
		worker := page.WorkerID
		if worker == "" {
			worker = "unassigned"
		}
		var row string
		if width < 82 {
			row = fmt.Sprintf("   %-6d %-11s %4d  %-15s %5s",
				page.ID, truncateText(worker, 11), page.PageNo, page.Status,
				formatOptionalInt32(page.RowCount))
		} else {
			row = fmt.Sprintf("   %-8d %-14s %-12s %-12s %4d  %-16s %5s",
				page.ID, truncateText(worker, 14), truncateText(page.ItemName, 12),
				page.ProcessDate.Format(time.DateOnly), page.PageNo, page.Status,
				formatOptionalInt32(page.RowCount))
		}
		if index == m.pageCursor {
			fmt.Fprintln(&builder, theme.selected.Width(theme.width).Render("▸"+row[1:]))
		} else {
			fmt.Fprintln(&builder, row)
		}
	}
	if m.message != "" {
		fmt.Fprintln(&builder, theme.notice(m.message, m.busy))
	}
	return theme.render(builder.String())
}

func (m tuiModel) viewLogs(width int) string {
	var builder strings.Builder
	theme := newTUITheme(width)
	fmt.Fprintln(&builder, theme.header(
		fmt.Sprintf("LIVE LOGS  ·  JOB %d", m.activeRunID),
		fmt.Sprintf("%d EVENTS", len(m.events)),
	))
	workers := m.workerIDs(m.runDetail.Pages)
	scopes := []string{"ALL"}
	scopes = append(scopes, workers...)
	var scopeLine []string
	for index, scope := range scopes {
		style := lipgloss.NewStyle().Foreground(tuiMuted).Background(tuiSurface).Padding(0, 1)
		if index == m.eventScope {
			style = style.Foreground(tuiCanvas).Background(tuiCyan).Bold(true)
		}
		scopeLine = append(scopeLine, style.Render(scope))
	}
	fmt.Fprintln(&builder, strings.Join(scopeLine, " "))
	fmt.Fprintln(&builder)
	logHeader := "   TIME          WORKER         PHASE          TASK     MESSAGE"
	if width < 82 {
		logHeader = "   TIME      WORKER      PHASE        MESSAGE"
	}
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(logHeader))
	start, end := m.eventRange()
	for index := start; index < end; index++ {
		event := m.events[index]
		worker := event.WorkerID
		if worker == "" {
			worker = "system"
		}
		var row string
		if width < 82 {
			row = fmt.Sprintf("   %-9s %-11s %-12s %s",
				event.CreatedAt.Format("15:04:05"), truncateText(worker, 11),
				truncateText(event.Phase, 12), truncateText(event.Message, max(8, width-40)))
		} else {
			row = fmt.Sprintf("   %-13s %-14s %-14s %-8d %s",
				event.CreatedAt.Format("15:04:05.000"), truncateText(worker, 14),
				truncateText(event.Phase, 14), event.PageID,
				truncateText(event.Message, max(12, width-62)))
		}
		if index == m.eventCursor {
			fmt.Fprintln(&builder, theme.selected.Width(theme.width).Render("▸"+row[1:]))
		} else {
			fmt.Fprintln(&builder, row)
		}
	}
	if len(m.events) == 0 {
		fmt.Fprintln(&builder, theme.muted.Render("  아직 기록된 이벤트가 없습니다."))
	}
	if event, ok := m.selectedEvent(); ok {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.eyebrow.Render("EVENT DETAIL"))
		fmt.Fprintln(&builder, fmt.Sprintf("ID %d  ·  %s  ·  Task %d  ·  Fetch %d",
			event.ID, event.Level, event.PartitionID, event.PageID))
		fmt.Fprintln(&builder, lipgloss.NewStyle().
			Foreground(tuiText).Background(tuiSurface).Padding(0, 1).
			Width(max(30, width-4)).Render(event.Message))
	}
	return theme.render(builder.String())
}

func (m tuiModel) viewLedger(width int) string {
	var builder strings.Builder
	theme := newTUITheme(width)
	pageStatus := fmt.Sprintf("%d ROWS", len(m.ledger.Items))
	if m.ledger.HasMore {
		pageStatus += " · MORE"
	}
	fmt.Fprintln(&builder, theme.header("LEDGER", pageStatus))
	fmt.Fprintln(&builder, theme.muted.Render("모든 Job과 Task에서 수집된 원장입니다. 최신 관측 순으로 표시합니다."))
	fmt.Fprintln(&builder)
	ledgerHeader := "   ID       RCNO             품목          처리일       제품명                         수입사"
	if width < 82 {
		ledgerHeader = "   ID     RCNO           품목      처리일       제품명"
	}
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(ledgerHeader))
	start, end := m.ledgerRange()
	for index := start; index < end; index++ {
		item := m.ledger.Items[index]
		var row string
		if width < 82 {
			row = fmt.Sprintf("   %-6d %-14s %-9s %-12s %s",
				item.ID, truncateText(item.RCNO, 14), truncateText(item.ItemName, 9),
				formatItemDate(item.ProcessedDate), truncateText(item.ProductNameKO, max(8, width-48)))
		} else {
			row = fmt.Sprintf("   %-8d %-16s %-12s %-12s %-30s %s",
				item.ID, truncateText(item.RCNO, 16), truncateText(item.ItemName, 12),
				formatItemDate(item.ProcessedDate), truncateText(item.ProductNameKO, 30),
				truncateText(item.ImporterName, max(10, width-86)))
		}
		if index == m.ledgerCursor {
			fmt.Fprintln(&builder, theme.selected.Width(theme.width).Render("▸"+row[1:]))
		} else {
			fmt.Fprintln(&builder, row)
		}
	}
	if item, ok := m.selectedLedgerItem(); ok {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.eyebrow.Render("PROVENANCE"))
		fmt.Fprintln(&builder, fmt.Sprintf("Job %d · Task %d · Fetch %d", item.RunID, item.PartitionID, item.PageID))
		fmt.Fprintln(&builder, fmt.Sprintf("Fetch %d · %s", item.FetchID, item.ImporterName))
		fmt.Fprintln(&builder, theme.muted.Render(
			fmt.Sprintf("%s · %s · %s", item.ProductNameEN, item.CountryName, item.ObservedAt.Format(time.RFC3339)),
		))
	}
	return theme.render(builder.String())
}

func (m tuiModel) viewPageItems(width int) string {
	var builder strings.Builder
	theme := newTUITheme(width)
	pageID := uint64(0)
	if selected, ok := m.selectedPage(); ok {
		pageID = selected.ID
	}
	fmt.Fprintln(&builder, theme.header(
		fmt.Sprintf("TASK %d  ·  EXTRACTED ITEMS", pageID),
		fmt.Sprintf("%d ITEMS", len(m.pageItems)),
	))
	fmt.Fprintln(&builder, theme.muted.Render("이 Fetch 응답에서 추출되어 원장에 반영된 전체 Item입니다."))
	fmt.Fprintln(&builder)
	itemHeader := "   ROW   RCNO             제품명                               수입사                         국가"
	if width < 82 {
		itemHeader = "   ROW  RCNO           제품명                     국가"
	}
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(itemHeader))
	for index, item := range m.pageItems {
		var row string
		if width < 82 {
			row = fmt.Sprintf("   %-4d %-14s %-26s %s",
				item.RowNo, truncateText(item.RCNO, 14), truncateText(item.ProductNameKO, 26),
				truncateText(item.CountryName, 10))
		} else {
			row = fmt.Sprintf("   %-5d %-16s %-36s %-30s %s",
				item.RowNo, truncateText(item.RCNO, 16), truncateText(item.ProductNameKO, 36),
				truncateText(item.ImporterName, 30), item.CountryName)
		}
		if index == m.itemCursor {
			fmt.Fprintln(&builder, theme.selected.Width(theme.width).Render("▸"+row[1:]))
		} else {
			fmt.Fprintln(&builder, row)
		}
	}
	return theme.render(builder.String())
}

func (m tuiModel) viewRunDetail(width int) string {
	var builder strings.Builder
	detail := m.runDetail
	theme := newTUITheme(width)
	fmt.Fprintln(&builder, theme.header(
		fmt.Sprintf("RUN %d", detail.RunID),
		"RUN INSPECTOR",
	))
	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.pipeline(
		theme.stageCard(
			tuiCyan,
			"RUN",
			"실행",
			compactRunStatus(detail.Status),
			fmt.Sprintf("P %d/%d", detail.CompletedPartitions, detail.TotalPartitions),
		),
		theme.stageCard(
			tuiBlue,
			"RAW",
			"수집",
			fmt.Sprintf("REQ %d", detail.FetchedRequests),
			fmt.Sprintf("ROWS %d", detail.ParsedRows),
		),
		theme.stageCard(
			tuiViolet,
			"QA",
			"검증",
			fmt.Sprintf("DIRTY %d", detail.DirtyPartitions),
			fmt.Sprintf("PENDING %d", detail.PendingPages),
		),
	))

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.section("DATE TASKS", len(detail.Partitions)))
	partitionHeader := "ID     품목             처리일       상태               원장/RCNO   시도"
	if width < 76 {
		partitionHeader = "ID   ITEM       DATE       STATUS       RAW/RCNO TRY"
	}
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(partitionHeader))
	partitionLimit := min(4, len(detail.Partitions))
	for _, partition := range detail.Partitions[:partitionLimit] {
		if width < 76 {
			fmt.Fprintf(&builder, "%-4d %-10s %-10s %s %3d/%-3d %d\n",
				partition.ID,
				truncateText(partition.ItemName, 10),
				partition.ProcessDate.Format("2006-01-02"),
				theme.status(fmt.Sprintf("%-12s", partition.Status)),
				partition.ParsedRows,
				partition.UniqueRCNOCount,
				partition.Attempts,
			)
		} else {
			fmt.Fprintf(&builder, "%-6d %-16s %-12s %s %5d/%-5d %d\n",
				partition.ID,
				truncateText(partition.ItemName, 16),
				partition.ProcessDate.Format("2006-01-02"),
				theme.status(fmt.Sprintf("%-18s", partition.Status)),
				partition.ParsedRows,
				partition.UniqueRCNOCount,
				partition.Attempts,
			)
		}
	}
	if len(detail.Partitions) == 0 {
		fmt.Fprintln(&builder, theme.muted.Render("날짜 Task가 없습니다."))
	}
	if int(detail.TotalPartitions) > partitionLimit {
		fmt.Fprintln(&builder, theme.muted.Render(
			fmt.Sprintf("외 %d개 날짜 Task", int(detail.TotalPartitions)-partitionLimit),
		))
	}
	if partition, ok := m.firstFailedPartition(); ok {
		fmt.Fprintln(&builder, theme.errorPanel(
			fmt.Sprintf("Task %d 오류", partition.ID),
			truncateText(partition.LastError, max(24, width-24)),
		))
	}

	fmt.Fprintln(&builder)
	fmt.Fprintln(&builder, theme.section("FETCHES", len(detail.Pages)))
	pageHeader := "   page   Fetch 상태            total    rows    RCNO   시도"
	if width < 76 {
		pageHeader = "   PAGE  FETCH STATUS    TOTAL  ROWS RCNO TRY"
	}
	fmt.Fprintln(&builder, theme.tableHeader.Width(theme.width).Render(pageHeader))
	start, end := m.visiblePageRange()
	for index := start; index < end; index++ {
		page := detail.Pages[index]
		fmt.Fprintln(&builder, renderPageRow(theme, page, index == m.pageCursor))
	}
	if len(detail.Pages) == 0 {
		fmt.Fprintln(&builder, theme.muted.Render("Fetch가 없습니다."))
	}
	if selected, ok := m.selectedPage(); ok && selected.LastError != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.errorPanel(
			"선택 Fetch 오류",
			truncateText(selected.LastError, max(24, width-22)),
		))
	}
	if m.message != "" {
		fmt.Fprintln(&builder)
		fmt.Fprintln(&builder, theme.notice(m.message, m.busy))
	}
	return theme.render(builder.String())
}

func compactRunStatus(status string) string {
	if status == "PARTIAL_FAILED" {
		return "PART_FAIL"
	}
	return truncateText(status, 10)
}

func renderPageRow(theme tuiTheme, page overview.Page, selected bool) string {
	if theme.width < 76 {
		plain := fmt.Sprintf(
			"   %-5d %-15s %5s %5s %4s %3d",
			page.PageNo,
			page.Status,
			formatOptionalInt64(page.TotalSnapshot),
			formatOptionalInt32(page.RowCount),
			formatOptionalInt32(page.UniqueRCNOCount),
			page.Attempts,
		)
		if selected {
			return theme.selected.Width(theme.width).Render("▸" + plain[1:])
		}
		status := theme.status(fmt.Sprintf("%-15s", page.Status))
		return fmt.Sprintf(
			"   %-5d %s %5s %5s %4s %3d",
			page.PageNo,
			status,
			formatOptionalInt64(page.TotalSnapshot),
			formatOptionalInt32(page.RowCount),
			formatOptionalInt32(page.UniqueRCNOCount),
			page.Attempts,
		)
	}
	plain := fmt.Sprintf(
		"   %-5d  %-20s %7s %7s %7s %5d",
		page.PageNo,
		page.Status,
		formatOptionalInt64(page.TotalSnapshot),
		formatOptionalInt32(page.RowCount),
		formatOptionalInt32(page.UniqueRCNOCount),
		page.Attempts,
	)
	if selected {
		return theme.selected.Width(theme.width).Render("▸" + plain[1:])
	}
	status := theme.status(fmt.Sprintf("%-20s", page.Status))
	return fmt.Sprintf(
		"   %-5d  %s %7s %7s %7s %5d",
		page.PageNo,
		status,
		formatOptionalInt64(page.TotalSnapshot),
		formatOptionalInt32(page.RowCount),
		formatOptionalInt32(page.UniqueRCNOCount),
		page.Attempts,
	)
}

func (m tuiModel) checkDatabase() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		db, err := m.openDatabase(m.cfg.Database)
		if err != nil {
			return databaseResultMsg{err: err}
		}
		defer db.Close()
		if err := db.Ping(ctx); err != nil {
			return databaseResultMsg{err: err}
		}
		statuses, err := db.MigrationStatus(ctx)
		if err != nil {
			return databaseResultMsg{err: err}
		}
		var migrationVersion int64
		for _, status := range statuses {
			if status.Applied && status.Version > migrationVersion {
				migrationVersion = status.Version
			}
		}
		service, err := overview.NewService(db)
		if err != nil {
			return databaseResultMsg{err: err}
		}
		snapshot, err := service.Load(ctx)
		return databaseResultMsg{
			migrationVersion: migrationVersion,
			snapshot:         snapshot,
			err:              err,
		}
	}
}

func (m tuiModel) migrate() tea.Cmd {
	return func() tea.Msg {
		db, err := m.openDatabase(m.cfg.Database)
		if err != nil {
			return migrationResultMsg{err: err}
		}
		defer db.Close()
		if err := db.Ping(m.ctx); err != nil {
			return migrationResultMsg{err: err}
		}
		return migrationResultMsg{err: db.Migrate(m.ctx)}
	}
}

func (m tuiModel) loadRunDetail(runID uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		db, err := m.openDatabase(m.cfg.Database)
		if err != nil {
			return runDetailResultMsg{err: err}
		}
		defer db.Close()
		if err := db.Ping(ctx); err != nil {
			return runDetailResultMsg{err: err}
		}
		service, err := overview.NewService(db)
		if err != nil {
			return runDetailResultMsg{err: err}
		}
		detail, err := service.LoadRunDetail(ctx, runID)
		return runDetailResultMsg{detail: detail, err: err}
	}
}

func (m tuiModel) loadLive(runID uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		db, err := m.openDatabase(m.cfg.Database)
		if err != nil {
			return liveResultMsg{runID: runID, err: err}
		}
		defer db.Close()
		overviewService, err := overview.NewService(db)
		if err != nil {
			return liveResultMsg{runID: runID, err: err}
		}
		detail, err := overviewService.LoadRunDetail(ctx, runID)
		if err != nil {
			return liveResultMsg{runID: runID, err: err}
		}
		reader, ok := db.(operator.Reader)
		if !ok {
			return liveResultMsg{runID: runID, detail: detail}
		}
		operatorService, err := operator.NewService(reader)
		if err != nil {
			return liveResultMsg{runID: runID, err: err}
		}
		workerID := m.selectedWorkerID(detail.Pages)
		afterID := m.lastEventID
		if runID != m.eventRunID || workerID != m.eventWorkerID {
			afterID = 0
		}
		page, err := operatorService.Events(ctx, runID, workerID, afterID)
		return liveResultMsg{
			runID: runID, workerID: workerID, detail: detail, page: page, err: err,
		}
	}
}

func (m tuiModel) loadLedger(beforeID uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		db, err := m.openDatabase(m.cfg.Database)
		if err != nil {
			return ledgerResultMsg{err: err}
		}
		defer db.Close()
		reader, ok := db.(ledger.Reader)
		if !ok {
			return ledgerResultMsg{err: fmt.Errorf("원장 조회 기능을 지원하지 않는 database입니다")}
		}
		service, err := ledger.NewService(reader)
		if err != nil {
			return ledgerResultMsg{err: err}
		}
		page, err := service.Observations(ctx, ledger.Filters{}, beforeID)
		return ledgerResultMsg{ledger: page, err: err}
	}
}

func (m tuiModel) loadPageItems(pageID uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		db, err := m.openDatabase(m.cfg.Database)
		if err != nil {
			return pageItemsResultMsg{pageID: pageID, err: err}
		}
		defer db.Close()
		reader, ok := db.(operator.Reader)
		if !ok {
			return pageItemsResultMsg{pageID: pageID, err: fmt.Errorf("Task 원장 조회 기능을 지원하지 않는 database입니다")}
		}
		service, err := operator.NewService(reader)
		if err != nil {
			return pageItemsResultMsg{pageID: pageID, err: err}
		}
		items, err := service.PageItems(ctx, pageID)
		return pageItemsResultMsg{pageID: pageID, items: items, err: err}
	}
}

func (m tuiModel) publishJob(command weblist.JobCommand) tea.Cmd {
	return func() tea.Msg {
		go func() {
			command.OnStarted = func(runID uint64) {
				m.jobUpdates <- jobPublishedMsg{runID: runID}
			}
			result, err := m.runJob(m.ctx, m.cfg, command)
			m.jobUpdates <- jobResultMsg{result: result, err: err}
		}()
		return <-m.jobUpdates
	}
}

func (m tuiModel) waitJobUpdate() tea.Cmd {
	return func() tea.Msg {
		return <-m.jobUpdates
	}
}

func refreshTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(now time.Time) tea.Msg {
		return refreshTickMsg(now)
	})
}

func (m *tuiModel) focusedDate() *string {
	switch m.collectFocus {
	case 0:
		return &m.collectFrom
	case 1:
		return &m.collectTo
	default:
		return nil
	}
}

func (m *tuiModel) shiftFocusedDate(days int) {
	field := m.focusedDate()
	if field == nil {
		return
	}
	value, err := time.Parse(time.DateOnly, *field)
	if err != nil {
		m.message = "날짜를 YYYY-MM-DD 형식으로 입력해주세요"
		return
	}
	*field = value.AddDate(0, 0, days).Format(time.DateOnly)
	m.collectEditing = false
	m.message = ""
}

func appendDateInput(current string, value rune) string {
	if value == '-' {
		if (len(current) == 4 || len(current) == 7) && len(current) < len(time.DateOnly) {
			return current + string(value)
		}
		return current
	}
	if !unicode.IsDigit(value) || len(current) >= len(time.DateOnly) {
		return current
	}
	if len(current) == 4 || len(current) == 7 {
		current += "-"
	}
	return current + string(value)
}

func renderDateField(value string, focused bool) string {
	if focused {
		return value + "  ▏"
	}
	return value
}

func validateCollectionRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	from, err := time.Parse(time.DateOnly, fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("시작일은 YYYY-MM-DD 형식이어야 합니다")
	}
	to, err := time.Parse(time.DateOnly, toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("종료일은 YYYY-MM-DD 형식이어야 합니다")
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("시작일은 종료일보다 늦을 수 없습니다")
	}
	return from, to, nil
}

func (m tuiModel) defaultCollectionDate() string {
	location, err := time.LoadLocation(m.cfg.Timezone)
	if err != nil {
		location = time.Local
	}
	return time.Now().In(location).Format("2006-01-02")
}

func (m tuiModel) selectedRun() (overview.Run, bool) {
	if m.cursor < 0 || m.cursor >= len(m.dashboard.RecentRuns) {
		return overview.Run{}, false
	}
	return m.dashboard.RecentRuns[m.cursor], true
}

func (m tuiModel) selectedOrActiveRunID() uint64 {
	if selected, ok := m.selectedRun(); ok {
		return selected.ID
	}
	return m.activeRunID
}

func (m tuiModel) selectedTargetNames() []string {
	names := make([]string, 0, len(m.collectTargets))
	for index, selected := range m.collectTargets {
		if selected && index < len(m.cfg.Targets) {
			names = append(names, m.cfg.Targets[index].Name)
		}
	}
	return names
}

func (m tuiModel) workerIDs(pages []overview.Page) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0, m.collectWorkers)
	for _, page := range pages {
		if page.WorkerID == "" {
			continue
		}
		if _, exists := seen[page.WorkerID]; exists {
			continue
		}
		seen[page.WorkerID] = struct{}{}
		ids = append(ids, page.WorkerID)
	}
	sort.Strings(ids)
	return ids
}

func (m tuiModel) workerScopeCount() int {
	return max(1, len(m.workerIDs(m.runDetail.Pages))+1)
}

func (m tuiModel) selectedWorkerID(pages []overview.Page) string {
	ids := m.workerIDs(pages)
	if m.eventScope <= 0 || m.eventScope > len(ids) {
		return ""
	}
	return ids[m.eventScope-1]
}

func (m tuiModel) selectedPage() (overview.Page, bool) {
	if m.pageCursor < 0 || m.pageCursor >= len(m.runDetail.Pages) {
		return overview.Page{}, false
	}
	return m.runDetail.Pages[m.pageCursor], true
}

func (m tuiModel) visiblePageRange() (int, int) {
	total := len(m.runDetail.Pages)
	if total == 0 {
		return 0, 0
	}
	height := m.height
	if height <= 0 {
		height = 24
	}
	reserved := 14 + min(4, len(m.runDetail.Partitions))
	if _, ok := m.firstFailedPartition(); ok {
		reserved++
	}
	if selected, ok := m.selectedPage(); ok && selected.LastError != "" {
		reserved += 2
	}
	if m.message != "" {
		reserved += 2
	}
	if m.busy {
		reserved += 2
	}
	visible := max(3, height-reserved)
	visible = min(visible, total)
	start := m.pageCursor - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > total {
		start = total - visible
	}
	return start, start + visible
}

func (m tuiModel) dispatchPageRange() (int, int) {
	return boundedRange(len(m.runDetail.Pages), m.pageCursor, max(3, m.screenHeight()-17))
}

func (m tuiModel) eventRange() (int, int) {
	return boundedRange(len(m.events), m.eventCursor, max(4, m.screenHeight()-14))
}

func (m tuiModel) ledgerRange() (int, int) {
	return boundedRange(len(m.ledger.Items), m.ledgerCursor, max(4, m.screenHeight()-13))
}

func boundedRange(total, cursor, visible int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	visible = min(visible, total)
	start := cursor - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > total {
		start = total - visible
	}
	return start, start + visible
}

func (m tuiModel) selectedEvent() (operator.Event, bool) {
	if m.eventCursor < 0 || m.eventCursor >= len(m.events) {
		return operator.Event{}, false
	}
	return m.events[m.eventCursor], true
}

func (m tuiModel) selectedLedgerItem() (ledger.Observation, bool) {
	if m.ledgerCursor < 0 || m.ledgerCursor >= len(m.ledger.Items) {
		return ledger.Observation{}, false
	}
	return m.ledger.Items[m.ledgerCursor], true
}

func (m tuiModel) firstFailedPartition() (overview.Partition, bool) {
	for _, partition := range m.runDetail.Partitions {
		if partition.LastError != "" {
			return partition, true
		}
	}
	return overview.Partition{}, false
}

func (m tuiModel) visibleRunRange() (int, int) {
	total := len(m.dashboard.RecentRuns)
	if total == 0 {
		return 0, 0
	}
	height := m.height
	if height <= 0 {
		height = 24
	}
	reserved := 9
	if selected, ok := m.selectedRun(); ok && selected.LastError != "" {
		reserved += 2
	}
	if m.message != "" {
		reserved += 2
	}
	if m.busy {
		reserved += 2
	}
	visible := max(3, height-reserved)
	visible = min(visible, total)
	start := m.cursor - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > total {
		start = total - visible
	}
	return start, start + visible
}

func formatLoadedAt(loadedAt time.Time) string {
	if loadedAt.IsZero() {
		return "-"
	}
	return loadedAt.Format("15:04:05")
}

func formatRunDate(run overview.Run) string {
	from := run.RequestedFromDate.Format("2006-01-02")
	to := run.RequestedToDate.Format("2006-01-02")
	if from == to {
		return from
	}
	return from + "~" + to
}

func truncateText(value string, limit int) string {
	runes := []rune(strings.ReplaceAll(value, "\n", " "))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit <= 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatOptionalInt64(value *int64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func formatOptionalInt32(value *int32) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func formatItemDate(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.Format(time.DateOnly)
}
