package home

import (
	"strings"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/screens/placeholder"
	sessionscreen "github.com/abhisek/mathiz/internal/screens/session"
	"github.com/abhisek/mathiz/internal/screens/skillmap"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/abhisek/mathiz/internal/ui/components"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// HomeScreen is the main home screen of the application.
type HomeScreen struct {
	menu components.Menu
}

var _ screen.Screen = (*HomeScreen)(nil)

// New creates a new HomeScreen.
func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service, lessonService *lessons.Service, compressor *lessons.Compressor) *HomeScreen {
	items := []components.MenuItem{
		{Label: "Start Practice", Action: func() tea.Cmd {
			if generator == nil || eventRepo == nil || snapRepo == nil {
				return func() tea.Msg {
					return router.PushScreenMsg{Screen: placeholder.New("Start Practice")}
				}
			}
			return func() tea.Msg {
				return router.PushScreenMsg{
					Screen: sessionscreen.New(generator, eventRepo, snapRepo, diagService, lessonService, compressor),
				}
			}
		}},
		{Label: "Skill Map", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: skillmap.New()}
			}
		}},
		{Label: "Gem Vault", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: placeholder.New("Gem Vault")}
			}
		}},
		{Label: "History", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: placeholder.New("History")}
			}
		}},
		{Label: "Settings", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: placeholder.New("Settings")}
			}
		}},
	}

	return &HomeScreen{
		menu: components.NewMenu(items),
	}
}

func (h *HomeScreen) Init() tea.Cmd {
	return nil
}

func (h *HomeScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	h.menu, cmd = h.menu.Update(msg)
	return h, cmd
}

func (h *HomeScreen) View(width, height int) string {
	// height is the content area; estimate full terminal height
	// by adding back header (3) + footer (3) + frame gaps
	termHeight := height + 8
	compact := termHeight < 30 || width < 100

	var sections []string

	if !compact {
		mascot := lipgloss.PlaceHorizontal(width, lipgloss.Center, RenderMascot())
		sections = append(sections, mascot)
	}

	var greetingText string
	if compact {
		greetingText = "Hey there, math explorer! ✦"
	} else {
		greetingText = "Hey there, math explorer!\nReady to level up today? ✦"
	}

	greeting := lipgloss.NewStyle().
		Width(width).
		Foreground(theme.Text).
		Align(lipgloss.Center).
		Render(greetingText)
	sections = append(sections, greeting)

	// Render the menu as a left-aligned block, then center the whole block
	menuBlock := h.menu.View()
	centeredMenu := lipgloss.PlaceHorizontal(width, lipgloss.Center, menuBlock)
	sections = append(sections, centeredMenu)

	return "\n" + strings.Join(sections, "\n\n")
}

func (h *HomeScreen) Title() string {
	return "Home"
}
