package components

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// MenuItem represents a single item in a navigation menu.
type MenuItem struct {
	Label    string
	Action   func() tea.Cmd
	Disabled bool
}

// Menu is a vertical navigation menu.
type Menu struct {
	Items    []MenuItem
	Selected int
}

// NewMenu creates a new menu with the given items.
func NewMenu(items []MenuItem) Menu {
	selected := 0
	for i, item := range items {
		if !item.Disabled {
			selected = i
			break
		}
	}
	return Menu{
		Items:    items,
		Selected: selected,
	}
}

// Init returns nil (no initial command).
func (m Menu) Init() tea.Cmd {
	return nil
}

// Update handles keyboard navigation.
func (m Menu) Update(msg tea.Msg) (Menu, tea.Cmd) {
	kmsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch kmsg.String() {
	case "up", "k":
		for i := m.Selected - 1; i >= 0; i-- {
			if !m.Items[i].Disabled {
				m.Selected = i
				break
			}
		}
	case "down", "j":
		for i := m.Selected + 1; i < len(m.Items); i++ {
			if !m.Items[i].Disabled {
				m.Selected = i
				break
			}
		}
	case "enter":
		if m.Selected >= 0 && m.Selected < len(m.Items) {
			item := m.Items[m.Selected]
			if item.Action != nil && !item.Disabled {
				return m, item.Action()
			}
		}
	}

	return m, nil
}

// View renders the menu.
func (m Menu) View() string {
	var s string
	for i, item := range m.Items {
		if i == m.Selected {
			s += lipgloss.NewStyle().
				Foreground(theme.Primary).
				Bold(true).
				Render("  ▸ "+item.Label) + "\n"
		} else {
			s += lipgloss.NewStyle().
				Foreground(theme.Text).
				Render("    "+item.Label) + "\n"
		}
	}
	return s
}
