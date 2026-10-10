package tui

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sdillen/token-usage/internal/usage"
)

var (
	hBlocks = []rune("▏▎▍▌▋▊▉█")
	vBlocks = []rune("▁▂▃▄▅▆▇█")
)

// bar draws a horizontal progress bar of w cells with eighth-cell precision.
func bar(frac float64, w int, fill lipgloss.TerminalColor) string {
	if w < 1 {
		return ""
	}
	frac = math.Max(0, math.Min(1, frac))
	eighths := int(math.Round(frac * float64(w) * 8))
	full := eighths / 8
	rem := eighths % 8

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(fill).Render(strings.Repeat("█", full)))
	used := full
	if rem > 0 && used < w {
		b.WriteString(lipgloss.NewStyle().Foreground(fill).Render(string(hBlocks[rem-1])))
		used++
	}
	b.WriteString(lipgloss.NewStyle().Foreground(colTrack).Render(strings.Repeat("▬", w-used)))
	return b.String()
}

// limitRow renders one rate-limit window: label, bar, percentage, reset.
func limitRow(l usage.Limit, w int, now time.Time) []string {
	pct := lipgloss.NewStyle().Foreground(levelColor(l.Percent)).Bold(true).
		Render(padLeft(percentText(l.Percent), 4))
	reset := ""
	if !l.ResetsAt.IsZero() {
		reset = "resets in " + humanDuration(l.ResetsAt.Sub(now))
	}
	head := padRight(styleText.Render(truncate(l.Label, w-lipgloss.Width(reset)-1)), w-lipgloss.Width(reset)) +
		styleDim.Render(reset)
	barLine := bar(l.Percent, w-5, levelColor(l.Percent)) + " " + pct
	return []string{head, barLine}
}

func percentText(p float64) string {
	v := int(math.Round(p * 100))
	return strconv.Itoa(v) + "%"
}

// columnChart draws the daily totals as vertical bars, h rows tall.
func columnChart(days []usage.Day, w, h int, accent lipgloss.TerminalColor, today time.Time) []string {
	n := len(days)
	if n == 0 || w < n*2 {
		return nil
	}
	var peak int64
	for _, d := range days {
		if d.Tokens > peak {
			peak = d.Tokens
		}
	}
	colW := w / n
	if colW > 7 {
		colW = 7
	}
	barW := colW - 1
	if barW < 1 {
		barW = 1
	}
	pad := (w - colW*n) / 2

	rows := make([]string, 0, h+1)
	for r := h - 1; r >= 0; r-- {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", pad))
		for _, d := range days {
			units := 0.0
			if peak > 0 {
				units = float64(d.Tokens) / float64(peak) * float64(h)
			}
			cell := " "
			switch {
			case units >= float64(r+1):
				cell = "█"
			case units > float64(r):
				idx := int(math.Round((units - float64(r)) * 8))
				if idx < 1 {
					idx = 1
				}
				cell = string(vBlocks[idx-1])
			case r == 0 && d.Tokens == 0:
				cell = "▁"
			}
			style := lipgloss.NewStyle().Foreground(accent)
			if d.Tokens == 0 {
				style = lipgloss.NewStyle().Foreground(colTrack)
			} else if !d.Date.Equal(today) {
				style = lipgloss.NewStyle().Foreground(accent).Faint(true)
			}
			b.WriteString(style.Render(strings.Repeat(cell, barW)) + strings.Repeat(" ", colW-barW))
		}
		rows = append(rows, b.String())
	}

	var lab strings.Builder
	lab.WriteString(strings.Repeat(" ", pad))
	for _, d := range days {
		name := d.Date.Format("Mon")
		style := styleDim
		if d.Date.Equal(today) {
			style = lipgloss.NewStyle().Foreground(accent).Bold(true)
		}
		lab.WriteString(style.Render(padRight(truncate(name, colW-1), colW)))
	}
	return append(rows, lab.String())
}

// monthChart draws one column per day of the month as a single-row
// sparkline, with the first and last day and the busiest day underneath.
func monthChart(days []usage.Day, w int, accent lipgloss.TerminalColor, today time.Time) []string {
	n := len(days)
	if n == 0 || w < n {
		return nil
	}
	colW := min(w/n, 2)
	var peak int64
	var peakDay time.Time
	for _, d := range days {
		if d.Tokens > peak {
			peak, peakDay = d.Tokens, d.Date
		}
	}
	var b strings.Builder
	for _, d := range days {
		style := lipgloss.NewStyle().Foreground(accent)
		cell := "▁"
		switch {
		case d.Date.After(today):
			style, cell = lipgloss.NewStyle().Foreground(colTrack), "·"
		case d.Tokens == 0:
			style = lipgloss.NewStyle().Foreground(colTrack)
		default:
			idx := int(math.Ceil(float64(d.Tokens) / float64(peak) * 8))
			cell = string(vBlocks[max(idx, 1)-1])
			if !d.Date.Equal(today) {
				style = style.Faint(true)
			}
		}
		b.WriteString(style.Render(strings.Repeat(cell, colW)))
	}
	spark := b.String()
	width := lipgloss.Width(spark)

	caption := ""
	if peak > 0 {
		caption = "peak " + humanTokens(peak) + " on " + peakDay.Format("Jan 2")
	}
	first := days[0].Date.Format("Jan 2")
	last := days[n-1].Date.Format("Jan 2")
	axis := spread(styleDim.Render(first), styleDim.Render(last), width)
	return []string{spark, axis, styleDim.Render(caption)}
}
