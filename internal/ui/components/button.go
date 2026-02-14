package components

import (
	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// Button is a styled button component.
type Button struct {
	Label   string
	Active  bool
	OnPress func() tea.Cmd
}

// NewButton creates a new button.
func NewButton(label string, active bool, onPress func() tea.Cmd) Button {
	return Button{
		Label:   label,
		Active:  active,
		OnPress: onPress,
	}
}

// Update handles key events.
func (b Button) Update(msg tea.Msg) (Button, tea.Cmd) {
	if !b.Active {
		return b, nil
	}

	if kmsg, ok := msg.(tea.KeyMsg); ok {
		if kmsg.String() == "enter" && b.OnPress != nil {
			return b, b.OnPress()
		}
	}

	return b, nil
}

// View renders the button.
func (b Button) View() string {
	label := "  ▸ " + b.Label + " "
	if b.Active {
		return theme.ButtonActive.Render(label)
	}
	return theme.ButtonInactive.Render(label)
}
