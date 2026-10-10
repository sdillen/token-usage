// Package tui is the Bubble Tea interface that displays agent usage.
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sdillen/token-usage/internal/usage"
)

const (
	refreshInterval = 30 * time.Second
	collectTimeout  = 30 * time.Second
)

// Source collects the report of one agent. force bypasses any throttling.
type Source struct {
	Name    string
	Collect func(ctx context.Context, force bool) usage.Report
}

// totalName names the tab that sums up all sources.
const totalName = "Total"

type keyMap struct {
	Refresh, Quit, Up, Down, Next, Prev key.Binding
}

var keys = keyMap{
	Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Quit:    key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Next:    key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab", "next tab")),
	Prev:    key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "previous tab")),
}

// Model is the Bubble Tea model.
type Model struct {
	sources []Source
	reports []usage.Report
	tab     int // 0 is the total, i+1 is sources[i]
	loading int // number of collections in flight
	now     time.Time

	spinner  spinner.Model
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

type (
	reportMsg struct {
		index  int
		report usage.Report
	}
	tickMsg    time.Time
	refreshMsg struct{}
)

// New returns a Model that displays the given sources, in order.
func New(sources []Source) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = lipgloss.NewStyle().Foreground(colClaude).Background(colBar)
	reports := make([]usage.Report, len(sources))
	for i, s := range sources {
		reports[i] = usage.Report{Name: s.Name}
	}
	vp := viewport.New(0, 0)
	vp.KeyMap.Left.SetEnabled(false) // left/right switch tabs
	vp.KeyMap.Right.SetEnabled(false)
	// loading starts at len(sources): Init, which has a value receiver, starts
	// one collection per source.
	return Model{sources: sources, reports: reports, spinner: sp, viewport: vp, now: time.Now(), loading: len(sources)}
}

// Init starts the first collection and the clock.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tick(), m.collectAll(false), refreshAfter())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func refreshAfter() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} })
}

// collectAll starts one collection per source. The caller counts them in
// m.loading: Init has a value receiver, so it could not do so itself.
func (m Model) collectAll(force bool) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.sources))
	for i, s := range m.sources {
		cmds[i] = func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
			defer cancel()
			return reportMsg{index: i, report: s.Collect(ctx, force)}
		}
	}
	return tea.Batch(cmds...)
}

// tabNames lists the tab titles in display order.
func (m Model) tabNames() []string {
	names := []string{totalName}
	for _, s := range m.sources {
		names = append(names, s.Name)
	}
	return names
}

// selectTab switches tabs, wrapping around, and scrolls back to the top.
func (m *Model) selectTab(i int) {
	n := len(m.sources) + 1
	m.tab = (i%n + n) % n
	m.layout()
	m.viewport.GotoTop()
}

// current returns the report of the selected tab.
func (m Model) current() usage.Report {
	if m.tab > 0 {
		return m.reports[m.tab-1]
	}
	return m.total()
}

// total sums up all reports that have arrived. A source that failed only
// drops out of the sum; the error shows on its own tab.
func (m Model) total() usage.Report {
	total := usage.Report{Name: totalName}
	var sums []usage.Summary
	for _, r := range m.reports {
		if r.UpdatedAt.IsZero() || r.Err != nil {
			continue
		}
		sums = append(sums, r.Summary)
		total.Parts = append(total.Parts, usage.Part{Name: r.Name, Tokens: r.Summary.MonthTokens.Total()})
		total.CostEstimated = total.CostEstimated || r.CostEstimated
		total.Unpriced = append(total.Unpriced, r.Unpriced...)
		if r.UpdatedAt.After(total.UpdatedAt) {
			total.UpdatedAt = r.UpdatedAt
		}
	}
	total.Summary = usage.Merge(sums...)
	return total
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.ready = true

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Next):
			m.selectTab(m.tab + 1)
		case key.Matches(msg, keys.Prev):
			m.selectTab(m.tab - 1)
		case len(msg.String()) == 1 && msg.String() >= "1" && msg.String() <= "9" && int(msg.String()[0]-'1') <= len(m.sources):
			m.selectTab(int(msg.String()[0] - '1'))
		case key.Matches(msg, keys.Refresh):
			if m.loading == 0 {
				m.loading = len(m.sources)
				cmds = append(cmds, m.collectAll(true))
			}
		}

	case reportMsg:
		m.loading--
		m.reports[msg.index] = msg.report
		m.layout()

	case tickMsg:
		m.now = time.Time(msg)
		m.layout()
		cmds = append(cmds, tick())

	case refreshMsg:
		if m.loading == 0 {
			m.loading = len(m.sources)
			cmds = append(cmds, m.collectAll(false))
		}
		cmds = append(cmds, refreshAfter())

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Scrolling for terminals shorter than the content.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// View renders the interface.
func (m Model) View() string {
	if !m.ready {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), m.statusBar())
}
