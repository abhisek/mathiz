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

const mascotArt = `  ╭───────────╮
  │  ┌─────┐  │
  │  │ ◉ ◉ │  │
  │  │  ▽  │  │
  │  ├─────┤  │
  │  │ ±×÷ │  │
  │  └─────┘  │
  ╰───────────╯`

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
		// Only transition once the full animation has played.
		if w.elapsed >= totalDur {
			return w, w.transition()
		}
		return w, nil
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

func (w *WelcomeScreen) View(width, height int) string {
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
		if len(lines) > 1 {
			lines[0] = s1 + "  " + lines[0] + "  " + s2
		}
		if len(lines) > 3 {
			lines[3] = s2 + "  " + lines[3] + "  " + s1
		}
		if len(lines) > 6 {
			lines[6] = s1 + "  " + lines[6] + "  " + s2
		}
		rendered = strings.Join(lines, "\n")
	}

	sections = append(sections, rendered)

	// Phase 3+: banner + tagline
	if w.elapsed >= phase2End {
		sections = append(sections, "")
		sections = append(sections, RenderBanner(width))
		sections = append(sections, "")

		tagline := lipgloss.NewStyle().
			Foreground(theme.Text).
			Bold(true).
			Render("Let's make math fun!")
		sections = append(sections, tagline)
	}

	// "press any key" hint
	if w.elapsed >= phase2End {
		sections = append(sections, "")
		hint := lipgloss.NewStyle().
			Foreground(theme.TextDim).
			Italic(true).
			Render("press any key to continue")
		sections = append(sections, hint)
	}

	content := strings.Join(sections, "\n")

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
