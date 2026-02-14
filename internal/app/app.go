package app

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/screens/home"
	"github.com/abhisek/mathiz/internal/ui/layout"
)

// Options holds dependencies injected into the app.
type Options struct {
	// LLMProvider is the LLM provider for AI features. May be nil if
	// no API key is configured (AI features will be unavailable).
	LLMProvider llm.Provider
}

// AppModel is the root Bubble Tea model.
type AppModel struct {
	router *router.Router
	width  int
	height int
}

// newAppModel creates a new AppModel with the home screen.
func newAppModel() AppModel {
	homeScreen := home.New()
	return AppModel{
		router: router.New(homeScreen),
	}
}

func (m AppModel) Init() tea.Cmd {
	return nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.router.Depth() > 1 {
				return m, func() tea.Msg { return router.PopScreenMsg{} }
			}
			return m, nil
		}
	}

	cmd := m.router.Update(msg)
	return m, cmd
}

func (m AppModel) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true

	if m.width == 0 || m.height == 0 {
		return v
	}

	if layout.IsTooSmall(m.width, m.height) {
		v.SetContent(layout.RenderMinSizeMessage(m.width, m.height))
		return v
	}

	active := m.router.Active()
	title := ""
	if active != nil {
		title = active.Title()
	}

	header := layout.RenderHeader(title, 0, 0, m.width)

	var footerHints []layout.KeyHint
	if provider, ok := active.(screen.KeyHintProvider); ok {
		footerHints = provider.KeyHints()
	} else if m.router.Depth() > 1 {
		footerHints = []layout.KeyHint{
			{Key: "Esc", Description: "Back"},
			{Key: "Ctrl+C", Description: "Quit"},
		}
	} else {
		footerHints = []layout.KeyHint{
			{Key: "↑↓", Description: "Navigate"},
			{Key: "Enter", Description: "Select"},
			{Key: "Ctrl+C", Description: "Quit"},
		}
	}

	footer := layout.RenderFooter(footerHints, m.width)

	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	contentHeight := m.height - headerHeight - footerHeight
	if contentHeight < 0 {
		contentHeight = 0
	}

	content := m.router.View(m.width, contentHeight)
	frame := layout.RenderFrame(header, content, footer, m.width, m.height)

	v.SetContent(frame)
	return v
}

// Run starts the Bubble Tea program.
func Run(opts Options) error {
	_ = opts // Options will be consumed by future specs (05, 09, 10).
	p := tea.NewProgram(newAppModel())
	_, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error running program:", err)
		return err
	}
	return nil
}
