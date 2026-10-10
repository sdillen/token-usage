package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sdillen/token-usage/internal/usage"
)

const chartHeight = 5

// section renders a small-caps style heading with a rule to the edge.
func section(title string, w int, accent lipgloss.TerminalColor) string {
	t := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(strings.ToUpper(title))
	rule := styleRule.Render(strings.Repeat("─", max(0, w-lipgloss.Width(t)-1)))
	return t + " " + rule
}

// spread puts left and right at the opposite edges of a w-cell line.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderCard draws the whole panel of one agent into a rounded box of outer
// width w.
func renderCard(r usage.Report, w int, now time.Time) string {
	accent := accentFor(r.Name)
	inner := w - 4 // border + padding
	var lines []string

	title := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("◆ " + r.Name)
	if r.Plan != "" {
		badge := lipgloss.NewStyle().Foreground(lipgloss.Color("#1e1e2e")).Background(accent).
			Padding(0, 1).Bold(true).Render(r.Plan)
		title = spread(title, badge, inner)
	}
	lines = append(lines, title, "")

	if r.Err != nil {
		lines = append(lines, styleBad.Render(truncate("error: "+r.Err.Error(), inner)))
		return box(lines, w, accent)
	}

	// Limits first: the question is how much room is left.
	if len(r.Limits) > 0 || r.LimitsNote != "" {
		lines = append(lines, section("Limits", inner, accent))
		for i, l := range r.Limits {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, limitRow(l, inner, now)...)
		}
		if r.LimitsNote != "" {
			if len(r.Limits) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, wrap(styleWarn.Render("! "+r.LimitsNote), inner)...)
		}
		lines = append(lines, "")
	}

	s := r.Summary
	lines = append(lines, section("Today", inner, accent))
	big := lipgloss.NewStyle().Foreground(colText).Bold(true).Render(humanTokens(s.TodayTokens.Total()))
	sub := styleDim.Render("tokens · " + plural(s.TodayMessages, "message") + " · " + plural(s.TodaySessions, "session"))
	lines = append(lines, big+" "+sub)
	lines = append(lines, tokenSplit(s.TodayTokens, inner, s.TodayCost, r.CostEstimated)...)
	lines = append(lines, "")

	lines = append(lines, section("Last 7 days", inner, accent))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if chart := columnChart(s.Recent, inner, chartHeight, accent, today); chart != nil {
		lines = append(lines, chart...)
	}
	lines = append(lines, "")

	lines = append(lines, section("This month · "+now.Format("January"), inner, accent))
	bigMonth := lipgloss.NewStyle().Foreground(colText).Bold(true).Render(humanTokens(s.MonthTokens.Total()))
	subMonth := styleDim.Render("tokens · " + plural(s.MonthMessages, "message") + " · " + plural(s.MonthSessions, "session"))
	lines = append(lines, bigMonth+" "+subMonth)
	lines = append(lines, tokenSplit(s.MonthTokens, inner, s.MonthCost, r.CostEstimated)...)
	lines = append(lines, "")
	if len(r.Unpriced) > 0 {
		lines = append(lines, wrap(styleWarn.Render("! no price for "+strings.Join(r.Unpriced, ", ")+", missing from the cost"), inner)...)
		lines = append(lines, "")
	}
	lines = append(lines, monthChart(s.MonthDays, inner, accent, today)...)
	lines = append(lines, "")

	if len(r.Parts) > 0 {
		lines = append(lines, section("By agent", inner, accent))
		lines = append(lines, partRows(r.Parts, s.MonthTokens.Total(), inner)...)
		lines = append(lines, "")
	}

	if len(s.Models) > 0 {
		lines = append(lines, section("Models", inner, accent))
		lines = append(lines, modelRows(s, inner, accent)...)
	} else if n := len(lines); lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return box(lines, w, accent)
}

// tokenSplit renders the in / out / cache breakdown of a token total.
func tokenSplit(t usage.Tokens, w int, cost float64, estimated bool) []string {
	parts := []string{
		styleDim.Render("in ") + styleText.Render(humanTokens(t.Input)),
		styleDim.Render("out ") + styleText.Render(humanTokens(t.Output)),
		styleDim.Render("cache ") + styleText.Render(humanTokens(t.CacheRead+t.CacheWrite)),
	}
	if cost > 0 {
		parts = append(parts, styleDim.Render("cost ")+styleText.Render(humanCost(cost, estimated)))
	}
	return wrap(strings.Join(parts, styleDim.Render("  ·  ")), w)
}

// partRows renders every agent's share of the month's tokens.
func partRows(parts []usage.Part, total int64, w int) []string {
	var rows []string
	for _, p := range parts {
		share := 0.0
		if total > 0 {
			share = float64(p.Tokens) / float64(total)
		}
		value := humanTokens(p.Tokens) + " " + styleDim.Render(padLeft(percentText(share), 4))
		rows = append(rows, spread(styleText.Render(p.Name), value, w), bar(share, w, accentFor(p.Name)))
	}
	return rows
}

// modelRows renders the share of every model in the month's tokens.
func modelRows(s usage.Summary, w int, accent lipgloss.TerminalColor) []string {
	const maxRows = 5
	total := s.MonthTokens.Total()
	var rows []string
	shown := s.Models
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for _, m := range shown {
		share := 0.0
		if total > 0 {
			share = float64(m.Tokens.Total()) / float64(total)
		}
		value := humanTokens(m.Tokens.Total()) + " " + styleDim.Render(padLeft(percentText(share), 4))
		name := truncate(shortModel(m.Model), w-lipgloss.Width(value)-1)
		rows = append(rows, spread(styleText.Render(name), value, w))
		rows = append(rows, bar(share, w, accent))
	}
	if extra := len(s.Models) - len(shown); extra > 0 {
		rows = append(rows, styleDim.Render("+ "+plural(extra, "more model")))
	}
	return rows
}

// shortModel drops vendor prefixes that only add noise.
func shortModel(m string) string {
	return strings.TrimPrefix(m, "claude-")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return humanTokens(int64(n)) + " " + noun + "s"
}

// wrap breaks a styled line into lines of at most w cells, on separators.
func wrap(s string, w int) []string {
	if lipgloss.Width(s) <= w {
		return []string{s}
	}
	return strings.Split(lipgloss.NewStyle().Width(w).Render(s), "\n")
}

func box(lines []string, w int, accent lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1).
		Width(w - 2).
		Render(strings.Join(lines, "\n"))
}
