package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/sdillen/token-usage/internal/claude"
	"github.com/sdillen/token-usage/internal/pi"
)

// Palette (Catppuccin-inspired, readable on dark and light terminals).
var (
	colText     = lipgloss.AdaptiveColor{Light: "#4c4f69", Dark: "#cdd6f4"}
	colDim      = lipgloss.AdaptiveColor{Light: "#7c7f93", Dark: "#7f849c"}
	colTrack    = lipgloss.AdaptiveColor{Light: "#ccd0da", Dark: "#313244"}
	colBar      = lipgloss.AdaptiveColor{Light: "#e6e9ef", Dark: "#181825"}
	colOnAccent = lipgloss.Color("#1e1e2e")
	colBorder   = lipgloss.AdaptiveColor{Light: "#acb0be", Dark: "#45475a"}

	colGood = lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#a6e3a1"}
	colWarn = lipgloss.AdaptiveColor{Light: "#df8e1d", Dark: "#f9e2af"}
	colBad  = lipgloss.AdaptiveColor{Light: "#d20f39", Dark: "#f38ba8"}

	colClaude = lipgloss.AdaptiveColor{Light: "#c8643c", Dark: "#d97757"}
	colPi     = lipgloss.AdaptiveColor{Light: "#7287fd", Dark: "#b4befe"}
	colTotal  = lipgloss.AdaptiveColor{Light: "#179299", Dark: "#94e2d5"}
)

var (
	styleText = lipgloss.NewStyle().Foreground(colText)
	styleDim  = lipgloss.NewStyle().Foreground(colDim)
	styleBad  = lipgloss.NewStyle().Foreground(colBad)
	styleWarn = lipgloss.NewStyle().Foreground(colWarn)
	styleRule = lipgloss.NewStyle().Foreground(colBorder)
)

// accentFor returns the brand color of an agent.
func accentFor(name string) lipgloss.TerminalColor {
	switch name {
	case claude.Name:
		return colClaude
	case pi.Name:
		return colPi
	case totalName:
		return colTotal
	}
	return colText
}

// levelColor colors a utilization: green while there is room, red near the cap.
func levelColor(p float64) lipgloss.TerminalColor {
	switch {
	case p >= 0.85:
		return colBad
	case p >= 0.6:
		return colWarn
	}
	return colGood
}
