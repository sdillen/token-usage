package tui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sdillen/token-usage/internal/usage"
)

func sampleReports(now time.Time) []usage.Report {
	day := func(back int) time.Time { return now.AddDate(0, 0, -back) }
	records := []usage.Record{
		{Key: "a", Model: "claude-opus-5-5", Session: "s1", Time: now, Tokens: usage.Tokens{Input: 10, Output: 4_000, CacheRead: 900_000, CacheWrite: 50_000}},
		{Key: "b", Model: "claude-sonnet-5-5", Session: "s2", Time: day(1), Tokens: usage.Tokens{Input: 10, Output: 9_000, CacheRead: 2_000_000}},
		{Key: "c", Model: "claude-sonnet-5-5", Session: "s2", Time: day(3), Tokens: usage.Tokens{Input: 10, Output: 9_000, CacheRead: 5_000_000}},
		{Key: "d", Model: "FIT/deepseek/deepseek-v4-flash", Session: "s3", Time: day(2), Tokens: usage.Tokens{Input: 1_000, Output: 100}, Cost: 0.42},
	}
	return []usage.Report{
		{
			Name: "Claude Code", Plan: "Team 5x", UpdatedAt: now,
			Summary: usage.Summarize(records[:3], now),
			Limits: []usage.Limit{
				{Label: "Session (5h)", Percent: 0.03, ResetsAt: now.Add(3*time.Hour + 20*time.Minute)},
				{Label: "Weekly (7d)", Percent: 0.67, ResetsAt: now.Add(60 * time.Hour)},
				{Label: "Opus Weekly", Percent: 0.93, ResetsAt: now.Add(60 * time.Hour)},
			},
		},
		{Name: "Pi Coding Agent", UpdatedAt: now, Summary: usage.Summarize(records[3:], now)},
	}
}

func render(t *testing.T, w, h, tab int) string {
	t.Helper()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	reports := sampleReports(now)
	sources := make([]Source, len(reports))
	for i, r := range reports {
		sources[i] = Source{Name: r.Name, Collect: func(context.Context, bool) usage.Report { return r }}
	}
	var m tea.Model = New(sources)
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	for i, r := range reports {
		m, _ = m.Update(reportMsg{index: i, report: r})
	}
	m, _ = m.Update(tickMsg(now))
	for range tab {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	return m.View()
}

func TestTabs(t *testing.T) {
	for _, tc := range []struct {
		tab   int
		wants []string
	}{
		{0, []string{"Total", "By agent", "Claude Code", "Pi Coding Agent", "This month · October", "$0.42", "Last 7 days"}},
		{1, []string{"Team 5x", "Session (5h)", "resets in 3h 20m", "93%", "sonnet-5-5", "This month"}},
		{2, []string{"Pi Coding Agent", "FIT/deepseek/deepseek-v4-flash", "$0.42", "This month"}},
	} {
		out := render(t, 100, 120, tc.tab)
		for _, want := range tc.wants {
			if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) { // headings are upper-cased
				t.Errorf("tab %d lacks %q", tc.tab, want)
			}
		}
		if strings.Contains(strings.ToLower(out), "all time") {
			t.Errorf("tab %d still shows all-time data", tc.tab)
		}
		if os.Getenv("TUI_DUMP") != "" {
			t.Logf("\n%s", out)
		}
	}
}

func TestTabWrapsAround(t *testing.T) {
	if out := render(t, 100, 120, 3); !strings.Contains(strings.ToLower(out), "by agent") {
		t.Errorf("tab 3 must wrap to the total, got:\n%s", out)
	}
}

func TestViewFitsWidth(t *testing.T) {
	for _, w := range []int{120, 80, 50} {
		for i, line := range strings.Split(render(t, w, 120, 1), "\n") {
			if got := lipglossWidth(line); got > w {
				t.Errorf("width %d: line %d is %d cells wide", w, i, got)
			}
		}
	}
}

func TestLoadingView(t *testing.T) {
	var m tea.Model = New([]Source{{Name: "Pi Coding Agent"}})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if out := m.View(); !strings.Contains(out, "Collecting usage") {
		t.Errorf("loading view lacks placeholder:\n%s", out)
	}
}

// collectReports runs cmd, and for batches all of its children, and returns
// the reports among the messages. Timers (ticks) are given up on after a
// short while instead of being waited for.
func collectReports(cmd tea.Cmd) []reportMsg {
	if cmd == nil {
		return nil
	}
	msgs := make(chan tea.Msg, 16)
	go func() { msgs <- cmd() }()

	var out []reportMsg
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case msg := <-msgs:
			switch msg := msg.(type) {
			case reportMsg:
				out = append(out, msg)
			case tea.BatchMsg:
				for _, c := range msg {
					out = append(out, collectReports(c)...)
				}
			}
			return out
		case <-deadline:
			return out
		}
	}
}

func TestRefreshKeyCollectsAgain(t *testing.T) {
	var calls int
	var forced []bool
	src := func(name string) Source {
		return Source{Name: name, Collect: func(_ context.Context, force bool) usage.Report {
			calls++
			forced = append(forced, force)
			return usage.Report{Name: name, UpdatedAt: time.Now()}
		}}
	}
	var m tea.Model = New([]Source{src("a"), src("b")})

	// The initial collection, as Bubble Tea runs it: Init's model is discarded.
	for _, r := range collectReports(m.Init()) {
		m, _ = m.Update(r)
	}
	if got := m.(Model).loading; got != 0 {
		t.Fatalf("loading after the first reports = %d, want 0", got)
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if got := m.(Model).loading; got != 2 {
		t.Fatalf("loading after r = %d, want 2", got)
	}
	reports := collectReports(cmd)
	if len(reports) != 2 || calls != 4 || !forced[3] {
		t.Fatalf("r started %d collections (%d calls in total, forced %v)", len(reports), calls, forced)
	}

	// While a refresh is running, r must not start another one.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}); len(collectReports(cmd)) != 0 {
		t.Error("r during a running refresh started another collection")
	}
	for _, r := range reports {
		m, _ = m.Update(r)
	}
	if got := m.(Model).loading; got != 0 {
		t.Errorf("loading after the refresh = %d, want 0", got)
	}

	// The periodic refresh takes the same path.
	_, cmd = m.Update(refreshMsg{})
	if got := len(collectReports(cmd)); got != 2 {
		t.Errorf("periodic refresh started %d collections, want 2", got)
	}
}

func TestEstimatedCostAndUnpriced(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	rec := usage.Record{Key: "a", Model: "claude-opus-5-5", Session: "s", Time: now, Tokens: usage.Tokens{Output: 1000}, Cost: 12.5}
	r := usage.Report{Name: "Claude Code", UpdatedAt: now, CostEstimated: true, Unpriced: []string{"claude-sonnet-5"}, Summary: usage.Summarize([]usage.Record{rec}, now)}

	out := renderCard(r, 76, now)
	for _, want := range []string{"~$12.50", "no price for claude-sonnet-5"} {
		if !strings.Contains(out, want) {
			t.Errorf("card lacks %q:\n%s", want, out)
		}
	}
}

func TestStatusBar(t *testing.T) {
	for _, w := range []int{120, 80, 60, 40, 20} {
		out := render(t, w, 40, 1)
		lines := strings.Split(out, "\n")
		bar := lines[len(lines)-1]
		if got := lipglossWidth(bar); got != w {
			t.Errorf("width %d: status bar is %d cells wide", w, got)
		}
		if w >= 80 {
			for _, want := range []string{"1 Total", "2 Claude Code", "3 Pi Coding Agent", "updated"} {
				if !strings.Contains(bar, want) {
					t.Errorf("width %d: status bar lacks %q: %q", w, want, bar)
				}
			}
		}
	}
	if bar := render(t, 120, 40, 0); !strings.Contains(bar, "refresh") || !strings.Contains(bar, "quit") {
		t.Error("wide status bar lacks the key hints")
	}
}
