package welcome

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

const (
	tickInterval = 100 * time.Millisecond
	phase1End    = 500 * time.Millisecond
	phase2End    = 1500 * time.Millisecond
	totalDur     = 4500 * time.Millisecond
)

const mascotArt = `┌─────┐
│ ◉ ◉ │
│  ▽  │
│ ±×÷ │
└─────┘`

// sparkle frames cycle around the mascot
var sparkleFrames = []string{"★", "✦"}

type tickMsg time.Time

// WelcomeScreen shows a splash animation before transitioning to the home screen.
type WelcomeScreen struct {
	homeFactory  func() screen.Screen
	elapsed      time.Duration
	tickCount    int
	transitioned bool
}

var _ screen.Screen = (*WelcomeScreen)(nil)

// New creates a WelcomeScreen that will transition to the screen produced by homeFactory.
func New(homeFactory func() screen.Screen) *WelcomeScreen {
	return &WelcomeScreen{
		homeFactory: homeFactory,
	}
}

func (w *WelcomeScreen) Title() string {
	return ""
}

func (w *WelcomeScreen) Init() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (w *WelcomeScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg.(type) {
	case tickMsg:
		if w.elapsed < totalDur {
			w.elapsed += tickInterval
		}
		w.tickCount++
		return w, tea.Tick(tickInterval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})

	case tea.KeyPressMsg:
		// Any keypress skips the animation and transitions immediately.
		return w, w.transition()
	}

	return w, nil
}

func (w *WelcomeScreen) transition() tea.Cmd {
	if w.transitioned {
		return nil
	}
	w.transitioned = true
	homeScreen := w.homeFactory()
	return func() tea.Msg {
		return router.ReplaceScreenMsg{Screen: homeScreen}
	}
}

// welcomeContentWidth mirrors the home screen's content width logic.
func welcomeContentWidth(frameWidth int) int {
	cw := frameWidth - 6
	if cw > 60 {
		cw = 60
	}
	if cw < 20 {
		cw = 20
	}
	return cw
}

func (w *WelcomeScreen) View(width, height int) string {
	cw := welcomeContentWidth(width)
	centerBox := lipgloss.NewStyle().Width(cw).Align(lipgloss.Center)

	var sections []string

	mascotStyle := lipgloss.NewStyle().Foreground(theme.Primary)

	// Phase 1+: mascot
	rendered := mascotStyle.Render(mascotArt)

	// Phase 2+: sparkles around mascot
	if w.elapsed >= phase1End {
		frame := w.tickCount % len(sparkleFrames)
		sparkle := sparkleFrames[frame]

		accentStyle := lipgloss.NewStyle().Foreground(theme.Accent)
		secondaryStyle := lipgloss.NewStyle().Foreground(theme.Secondary)

		s1 := accentStyle.Render(sparkle)
		s2 := secondaryStyle.Render(sparkle)

		// Place sparkles on sides of mascot
		lines := strings.Split(rendered, "\n")
		if len(lines) > 0 {
			lines[0] = s1 + "  " + lines[0] + "  " + s2
		}
		if len(lines) > 2 {
			lines[2] = s2 + "  " + lines[2] + "  " + s1
		}
		if len(lines) > 4 {
			lines[4] = s1 + "  " + lines[4] + "  " + s2
		}
		rendered = strings.Join(lines, "\n")
	}

	sections = append(sections, centerBox.Render(rendered))

	// Phase 3+: banner + tagline
	if w.elapsed >= phase2End {
		sections = append(sections, centerBox.Render(RenderBanner(cw)))

		tagline := lipgloss.NewStyle().
			Foreground(theme.Text).
			Bold(true).
			Render("Let's make math fun!")
		sections = append(sections, centerBox.Render(tagline))
	}

	// "press any key" hint — styled as a button to match home screen
	if w.elapsed >= phase2End {
		hint := lipgloss.NewStyle().
			Width(22).
			Align(lipgloss.Center).
			Foreground(theme.TextDim).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Border).
			Padding(0, 1).
			Italic(true).
			Render("press any key")
		sections = append(sections, centerBox.Render(hint))
	}

	content := strings.Join(sections, "\n\n")

	// Cabinet frame matching the home screen
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Width(width - 2).
		Height(height - 2).
		Align(lipgloss.Center, lipgloss.Center).
		Render(content)
}
