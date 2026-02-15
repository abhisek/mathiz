package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// isDark tracks whether the terminal has a dark background.
var isDark = true

// IsDark returns true if the theme is set to dark mode.
func IsDark() bool { return isDark }

// SetDark switches between dark and light palettes, then rebuilds all styles.
func SetDark(dark bool) {
	isDark = dark
	if dark {
		setDarkColors()
	} else {
		setLightColors()
	}
	rebuildStyles()
}

// Color palette — kid-friendly, bright but not garish
var (
	Primary      color.Color
	Secondary    color.Color
	Accent       color.Color
	Success      color.Color
	Error        color.Color
	Text         color.Color
	TextDim      color.Color
	BgDark       color.Color
	BgCard       color.Color
	Border       color.Color
	ArcadeYellow color.Color
	ArcadeCyan   color.Color
)

func setDarkColors() {
	Primary = lipgloss.Color("#8B5CF6")
	Secondary = lipgloss.Color("#14B8A6")
	Accent = lipgloss.Color("#F97316")
	Success = lipgloss.Color("#22C55E")
	Error = lipgloss.Color("#F43F5E")
	Text = lipgloss.Color("#F8FAFC")
	TextDim = lipgloss.Color("#94A3B8")
	BgDark = lipgloss.Color("#0F172A")
	BgCard = lipgloss.Color("#1E293B")
	Border = lipgloss.Color("#334155")
	ArcadeYellow = lipgloss.Color("#FFD700")
	ArcadeCyan = lipgloss.Color("#00FFFF")
}

func setLightColors() {
	Primary = lipgloss.Color("#7C3AED")
	Secondary = lipgloss.Color("#0D9488")
	Accent = lipgloss.Color("#EA580C")
	Success = lipgloss.Color("#16A34A")
	Error = lipgloss.Color("#E11D48")
	Text = lipgloss.Color("#1E293B")
	TextDim = lipgloss.Color("#64748B")
	BgDark = lipgloss.Color("#F1F5F9")
	BgCard = lipgloss.Color("#E2E8F0")
	Border = lipgloss.Color("#CBD5E1")
	ArcadeYellow = lipgloss.Color("#B45309")
	ArcadeCyan = lipgloss.Color("#0891B2")
}

// Typography
var (
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Body     lipgloss.Style
	Hint     lipgloss.Style
)

// Layout
var (
	Header lipgloss.Style
	Footer lipgloss.Style
	Card   lipgloss.Style
)

// States
var (
	Selected   lipgloss.Style
	Unselected lipgloss.Style
	Correct    lipgloss.Style
	Incorrect  lipgloss.Style
)

// Components
var (
	ProgressFilled lipgloss.Style
	ProgressEmpty  lipgloss.Style
	ButtonActive   lipgloss.Style
	ButtonInactive lipgloss.Style
)

func rebuildStyles() {
	// Typography
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

	// Layout
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

	// States
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

	// Components
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
}

func init() {
	setDarkColors()
	rebuildStyles()
}
