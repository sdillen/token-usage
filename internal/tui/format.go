package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// humanTokens renders a token count compactly: 950, 12.3k, 4.56M, 1.2B.
func humanTokens(n int64) string {
	f := float64(n)
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 100_000:
		return trimZero(fmt.Sprintf("%.1f", f/1e3)) + "k"
	case n < 1_000_000:
		return fmt.Sprintf("%.0fk", f/1e3)
	case n < 100_000_000:
		return trimZero(fmt.Sprintf("%.2f", f/1e6)) + "M"
	case n < 1_000_000_000:
		return fmt.Sprintf("%.0fM", f/1e6)
	default:
		return trimZero(fmt.Sprintf("%.2f", f/1e9)) + "B"
	}
}

func trimZero(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// humanDuration renders the time until a reset: "3d 4h", "2h 13m", "5m".
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	d = d.Round(time.Minute)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	}
	return "<1m"
}

// humanCost renders a USD amount; estimates are marked with a leading "~".
func humanCost(c float64, estimated bool) string {
	prefix := ""
	if estimated {
		prefix = "~"
	}
	switch {
	case c <= 0:
		return prefix + "$0"
	case c < 0.01:
		return prefix + "<$0.01"
	case c < 100:
		return prefix + fmt.Sprintf("$%.2f", c)
	}
	return prefix + fmt.Sprintf("$%.0f", c)
}

// truncate shortens s to at most w cells, ending in an ellipsis.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// padRight pads s with spaces to w cells.
func padRight(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// padLeft right-aligns s in w cells.
func padLeft(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}
