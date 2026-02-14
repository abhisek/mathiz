package placeholder

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// PlaceholderScreen is a generic "coming soon" screen.
type PlaceholderScreen struct {
	title string
}

var _ screen.Screen = (*PlaceholderScreen)(nil)

// New creates a new PlaceholderScreen with the given title.
func New(title string) *PlaceholderScreen {
	return &PlaceholderScreen{title: title}
}

func (p *PlaceholderScreen) Init() tea.Cmd {
	return nil
}

func (p *PlaceholderScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	return p, nil
}

func (p *PlaceholderScreen) View(width, height int) string {
	content := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center).
		Foreground(theme.Text).
		Render("╌╌ Coming Soon ╌╌\n\nThis feature is being built.\nCheck back later!")

	return content
}

func (p *PlaceholderScreen) Title() string {
	return p.title
}
