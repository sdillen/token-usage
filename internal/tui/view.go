package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const maxCardWidth = 76

// layout re-renders the body into the viewport for the current size.
func (m *Model) layout() {
	if m.width == 0 {
		return
	}
	bodyHeight := max(1, m.height-lipgloss.Height(m.statusBar()))
	m.viewport.Width, m.viewport.Height = m.width, bodyHeight
	m.viewport.SetContent(m.body())
}

func (m Model) body() string {
	r := m.current()
	var card string
	if r.UpdatedAt.IsZero() && r.Err == nil {
		card = box([]string{
			lipgloss.NewStyle().Foreground(accentFor(r.Name)).Bold(true).Render("◆ " + r.Name),
			"",
			styleDim.Render("Collecting usage…"),
		}, min(m.width, maxCardWidth), accentFor(r.Name))
	} else {
		card = renderCard(r, min(m.width, maxCardWidth), m.now)
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, card)
}

// onBar is the style of everything on the status bar: the bar's background
// with the given foreground.
func onBar(fg lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg).Background(colBar)
}

// tabs renders the tab titles as pills: the selected one filled in its
// agent color with half-block caps, the others dimmed. Every tab is the same
// width whether selected or not, so the bar does not shift when switching.
func (m Model) tabs() string {
	var out []string
	for i, name := range m.tabNames() {
		label := strconv.Itoa(i+1) + " " + name
		if i == m.tab {
			accent := accentFor(name)
			out = append(out,
				onBar(accent).Render("▐")+
					lipgloss.NewStyle().Foreground(colOnAccent).Background(accent).Bold(true).Render(" "+label+" ")+
					onBar(accent).Render("▌"))
			continue
		}
		out = append(out,
			onBar(colDim).Render("  ")+
				onBar(accentFor(name)).Render(strconv.Itoa(i+1))+
				onBar(colDim).Render(" "+name+"  "))
	}
	return strings.Join(out, onBar(colDim).Render(" "))
}

// hints renders the key hints, a space-separated list of "key label" pairs.
func hints() string {
	pairs := [][2]string{{"tab", "switch"}, {"r", "refresh"}, {"↑↓", "scroll"}, {"q", "quit"}}
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = onBar(colText).Bold(true).Render(p[0]) + onBar(colDim).Render(" "+p[1])
	}
	return strings.Join(parts, onBar(colDim).Render("  "))
}

// status renders the refresh state.
func (m Model) status() string {
	if m.loading > 0 {
		return m.spinner.View() + onBar(colDim).Render(" refreshing")
	}
	if t := m.lastUpdate(); !t.IsZero() {
		return onBar(colDim).Render("updated " + ago(m.now.Sub(t)))
	}
	return ""
}

// statusBar renders the full-width bar at the bottom: tabs on the left, the
// refresh state and key hints on the right. Whatever does not fit is dropped,
// the hints first.
func (m Model) statusBar() string {
	tabs := m.tabs()
	gap := onBar(colDim).Render("   ")
	for _, right := range []string{
		m.status() + gap + hints(),
		m.status(),
		"",
	} {
		if lipgloss.Width(tabs)+lipgloss.Width(right)+2 <= m.width {
			fill := onBar(colDim).Render(strings.Repeat(" ", m.width-lipgloss.Width(tabs)-lipgloss.Width(right)-1))
			return tabs + fill + right + onBar(colDim).Render(" ")
		}
	}
	return lipgloss.NewStyle().Background(colBar).Width(m.width).MaxWidth(m.width).Render(tabs)
}

func (m Model) lastUpdate() time.Time {
	var latest time.Time
	for _, r := range m.reports {
		if r.UpdatedAt.After(latest) {
			latest = r.UpdatedAt
		}
	}
	return latest
}

func ago(d time.Duration) string {
	if d < 5*time.Second {
		return "just now"
	}
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + "s ago"
	}
	return humanDuration(d) + " ago"
}
