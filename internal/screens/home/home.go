package home

import (
	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/screens/placeholder"
	"github.com/abhisek/mathiz/internal/ui/components"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// HomeScreen is the main home screen of the application.
type HomeScreen struct {
	menu components.Menu
}

var _ screen.Screen = (*HomeScreen)(nil)

// New creates a new HomeScreen.
func New() *HomeScreen {
	items := []components.MenuItem{
		{Label: "Start Practice", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: placeholder.New("Start Practice")}
			}
		}},
		{Label: "Skill Map", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: placeholder.New("Skill Map")}
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
	compact := height < 30 || width < 100

	var content string

	if !compact {
		// Full layout with mascot
		mascot := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(RenderMascot())
		content += "\n" + mascot + "\n\n"
	}

	greeting := lipgloss.NewStyle().
		Width(width).
		Foreground(theme.Text).
		Align(lipgloss.Center).
		Render("Hey there, math explorer!\nReady to level up today? 🚀")

	if compact {
		greeting = lipgloss.NewStyle().
			Width(width).
			Foreground(theme.Text).
			Align(lipgloss.Center).
			Render("Hey there, math explorer! 🚀")
	}

	content += "\n" + greeting + "\n\n"

	menuView := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(h.menu.View())

	content += menuView

	return content
}

func (h *HomeScreen) Title() string {
	return "Home"
}
