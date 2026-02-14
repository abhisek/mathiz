package theme

import (
	"charm.land/lipgloss/v2"
)

// Color palette — kid-friendly, bright but not garish
var (
	Primary   = lipgloss.Color("#8B5CF6") // Vivid Purple
	Secondary = lipgloss.Color("#14B8A6") // Teal
	Accent    = lipgloss.Color("#F97316") // Orange
	Success   = lipgloss.Color("#22C55E") // Green
	Error     = lipgloss.Color("#F43F5E") // Rose
	Text      = lipgloss.Color("#F8FAFC") // White
	TextDim   = lipgloss.Color("#94A3B8") // Slate
	BgDark    = lipgloss.Color("#0F172A") // Deep Navy
	BgCard    = lipgloss.Color("#1E293B") // Dark Slate
	Border    = lipgloss.Color("#334155") // Slate
)

// Typography
var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary).
		Align(lipgloss.Center)

	Subtitle = lipgloss.NewStyle().
			Foreground(TextDim).
			Align(lipgloss.Center)

	Body = lipgloss.NewStyle().
		Foreground(Text)

	Hint = lipgloss.NewStyle().
		Foreground(TextDim).
		Italic(true)
)

// Layout
var (
	Header = lipgloss.NewStyle().
		Background(BgCard).
		Padding(0, 2)

	Footer = lipgloss.NewStyle().
		Background(BgCard).
		Padding(0, 2)

	Card = lipgloss.NewStyle().
		Background(BgCard).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(1, 2)
)

// States
var (
	Selected = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	Unselected = lipgloss.NewStyle().
			Foreground(Text)

	Correct = lipgloss.NewStyle().
		Foreground(Success).
		Bold(true)

	Incorrect = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)
)

// Components
var (
	ProgressFilled = lipgloss.NewStyle().
			Background(Secondary)

	ProgressEmpty = lipgloss.NewStyle().
			Background(Border)

	ButtonActive = lipgloss.NewStyle().
			Background(Primary).
			Foreground(Text).
			Bold(true).
			Padding(0, 2)

	ButtonInactive = lipgloss.NewStyle().
			Background(BgCard).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Border).
			Padding(0, 2)
)
