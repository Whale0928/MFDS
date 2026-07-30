package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var (
	tuiCanvas  = lipgloss.Color("#0B1116")
	tuiSurface = lipgloss.Color("#121B22")
	tuiRaised  = lipgloss.Color("#18242D")
	tuiText    = lipgloss.Color("#D9E2E8")
	tuiMuted   = lipgloss.Color("#71828E")
	tuiCyan    = lipgloss.Color("#24C7B1")
	tuiBlue    = lipgloss.Color("#4E9DFF")
	tuiViolet  = lipgloss.Color("#A78BFA")
	tuiGreen   = lipgloss.Color("#55D187")
	tuiAmber   = lipgloss.Color("#F2C14E")
	tuiRed     = lipgloss.Color("#FF6B6B")
)

type tuiTheme struct {
	width       int
	screen      lipgloss.Style
	title       lipgloss.Style
	eyebrow     lipgloss.Style
	muted       lipgloss.Style
	tableHeader lipgloss.Style
	selected    lipgloss.Style
	footer      lipgloss.Style
}

func configureTUIColor() {
	if os.Getenv("TERM") == "dumb" || os.Getenv("MFDS_TUI_NO_COLOR") == "1" {
		lipgloss.SetColorProfile(termenv.Ascii)
		return
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func newTUITheme(width int) tuiTheme {
	if width <= 0 {
		width = 100
	}
	width = max(54, width)
	return tuiTheme{
		width: width,
		screen: lipgloss.NewStyle().
			Foreground(tuiText).
			Background(tuiCanvas).
			Width(width),
		title: lipgloss.NewStyle().
			Foreground(tuiText).
			Bold(true),
		eyebrow: lipgloss.NewStyle().
			Foreground(tuiCyan).
			Bold(true),
		muted: lipgloss.NewStyle().
			Foreground(tuiMuted),
		tableHeader: lipgloss.NewStyle().
			Foreground(tuiMuted).
			Background(tuiSurface).
			Bold(true),
		selected: lipgloss.NewStyle().
			Foreground(tuiCanvas).
			Background(tuiCyan).
			Bold(true),
		footer: lipgloss.NewStyle().
			Foreground(tuiMuted).
			Background(tuiSurface).
			Width(width),
	}
}

func (t tuiTheme) render(content string) string {
	return t.screen.Render(content)
}

func (t tuiTheme) header(title, meta string) string {
	left := t.title.Render(title)
	right := t.muted.Render(meta)
	return left + "   " + right
}

func (t tuiTheme) stageCard(
	accent lipgloss.Color,
	index, title, primary, secondary string,
) string {
	cardWidth := max(16, (t.width-4)/3)
	bodyWidth := max(12, cardWidth-4)
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Foreground(accent).Bold(true).Render(index+"  "+title),
		lipgloss.NewStyle().Foreground(tuiText).Bold(true).Render(primary),
		lipgloss.NewStyle().Foreground(tuiMuted).Render(secondary),
	)
	return lipgloss.NewStyle().
		Width(bodyWidth).
		Height(3).
		Padding(0, 1).
		Background(tuiSurface).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Render(body)
}

func (t tuiTheme) pipeline(cards ...string) string {
	if t.width < 48 {
		return lipgloss.JoinVertical(lipgloss.Left, cards...)
	}
	parts := make([]string, 0, len(cards)*2-1)
	for index, card := range cards {
		if index > 0 {
			parts = append(parts, " ")
		}
		parts = append(parts, card)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (t tuiTheme) section(label string, count int) string {
	title := lipgloss.NewStyle().Foreground(tuiText).Bold(true).Render(label)
	badge := lipgloss.NewStyle().
		Foreground(tuiCanvas).
		Background(tuiMuted).
		Bold(true).
		Padding(0, 1).
		Render(fmt.Sprintf("%d", count))
	return title + " " + badge
}

func (t tuiTheme) status(value string) string {
	color := tuiAmber
	switch strings.TrimSpace(value) {
	case "COMPLETED", "DONE", "PARSED", "RAW_STORED":
		color = tuiGreen
	case "PARTIAL_FAILED", "FAILED", "PARSE_FAILED", "DIRTY", "DEAD":
		color = tuiRed
	case "CANCELLED":
		color = tuiMuted
	case "LISTING", "PAGING", "RECONCILING", "LEASED":
		color = tuiBlue
	case "QUEUED", "PENDING", "RETRY_WAIT":
		color = tuiAmber
	}
	return lipgloss.NewStyle().Foreground(color).Bold(true).Render(value)
}

func (t tuiTheme) errorPanel(label, value string) string {
	return lipgloss.NewStyle().
		Foreground(tuiRed).
		Background(tuiSurface).
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(tuiRed).
		Padding(0, 1).
		MaxWidth(max(20, t.width-4)).
		Render(label + "  " + value)
}

func (t tuiTheme) notice(value string, busy bool) string {
	color := tuiGreen
	label := "OK"
	if busy {
		color = tuiAmber
		label = "WORK"
	}
	return lipgloss.NewStyle().
		Foreground(color).
		Background(tuiSurface).
		Padding(0, 1).
		Render(label + "  " + value)
}

func (t tuiTheme) key(name, label string) string {
	key := lipgloss.NewStyle().
		Foreground(tuiCanvas).
		Background(tuiMuted).
		Bold(true).
		Padding(0, 1).
		Render(name)
	return key + " " + lipgloss.NewStyle().Foreground(tuiMuted).Render(label)
}

func (t tuiTheme) input(label, value string, focused bool) string {
	border := tuiMuted
	labelStyle := lipgloss.NewStyle().Foreground(tuiMuted)
	if focused {
		border = tuiCyan
		labelStyle = labelStyle.Foreground(tuiCyan).Bold(true)
	}
	return labelStyle.Width(10).Render(label) +
		lipgloss.NewStyle().
			Foreground(tuiText).
			Background(tuiRaised).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(border).
			Padding(0, 1).
			Width(max(24, t.width-16)).
			Render(value)
}
