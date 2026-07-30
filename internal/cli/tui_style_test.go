package cli

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestConfigureTUIColor_일반터미널은TrueColor를사용한다(t *testing.T) {
	original := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("MFDS_TUI_NO_COLOR", "")

	configureTUIColor()

	if profile := lipgloss.ColorProfile(); profile != termenv.TrueColor {
		t.Fatalf("ColorProfile() = %v", profile)
	}
}

func TestConfigureTUIColor_명시적비활성화는Ascii를사용한다(t *testing.T) {
	original := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("MFDS_TUI_NO_COLOR", "1")

	configureTUIColor()

	if profile := lipgloss.ColorProfile(); profile != termenv.Ascii {
		t.Fatalf("ColorProfile() = %v", profile)
	}
}
