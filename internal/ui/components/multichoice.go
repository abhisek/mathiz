package components

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// MultiChoice is a multiple-choice selector component.
type MultiChoice struct {
	Question     string
	Options      []string
	CorrectIndex int
	Selected     int
	Submitted    bool
	ChosenIndex  int
}

// NewMultiChoice creates a new multiple-choice component.
func NewMultiChoice(question string, options []string, correctIndex int) MultiChoice {
	return MultiChoice{
		Question:     question,
		Options:      options,
		CorrectIndex: correctIndex,
		Selected:     0,
		Submitted:    false,
		ChosenIndex:  -1,
	}
}

// Init returns nil.
func (m MultiChoice) Init() tea.Cmd {
	return nil
}

// Update handles keyboard navigation and selection.
func (m MultiChoice) Update(msg tea.Msg) (MultiChoice, tea.Cmd) {
	if m.Submitted {
		return m, nil
	}

	kmsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch kmsg.String() {
	case "up", "k":
		if m.Selected > 0 {
			m.Selected--
		}
	case "down", "j":
		if m.Selected < len(m.Options)-1 {
			m.Selected++
		}
	case "enter":
		m.Submitted = true
		m.ChosenIndex = m.Selected
	}

	return m, nil
}

// View renders the multiple-choice component.
func (m MultiChoice) View() string {
	questionStyle := lipgloss.NewStyle().Foreground(theme.Text).Bold(true)
	s := questionStyle.Render(m.Question) + "\n\n"

	labels := []string{"A", "B", "C", "D"}

	for i, opt := range m.Options {
		label := labels[i]
		prefix := "  "
		if i == m.Selected && !m.Submitted {
			prefix = "▸ "
		}

		line := fmt.Sprintf("%s%s)  %s", prefix, label, opt)

		if m.Submitted {
			if i == m.CorrectIndex {
				s += lipgloss.NewStyle().Foreground(theme.Success).Bold(true).Render(line) + "\n"
			} else if i == m.ChosenIndex {
				s += lipgloss.NewStyle().Foreground(theme.Error).Bold(true).Render(line) + "\n"
			} else {
				s += lipgloss.NewStyle().Foreground(theme.TextDim).Render(line) + "\n"
			}
		} else {
			if i == m.Selected {
				s += lipgloss.NewStyle().Foreground(theme.Primary).Bold(true).Render(line) + "\n"
			} else {
				s += lipgloss.NewStyle().Foreground(theme.Text).Render(line) + "\n"
			}
		}
	}

	return s
}

// IsCorrect returns true if the user chose the correct answer.
func (m MultiChoice) IsCorrect() bool {
	return m.Submitted && m.ChosenIndex == m.CorrectIndex
}
