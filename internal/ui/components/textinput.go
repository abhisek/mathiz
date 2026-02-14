package components

import (
	"strconv"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// TextInput wraps bubbles/textinput with Mathiz styling.
type TextInput struct {
	Model       textinput.Model
	NumericOnly bool
	MaxWidth    int
	submitted   bool
	valid       bool
}

// NewTextInput creates a new styled text input.
func NewTextInput(placeholder string, numericOnly bool, maxWidth int) TextInput {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()

	if maxWidth > 0 {
		ti.CharLimit = maxWidth
	}

	return TextInput{
		Model:       ti,
		NumericOnly: numericOnly,
		MaxWidth:    maxWidth,
	}
}

// Init returns the initial command.
func (t TextInput) Init() tea.Cmd {
	return t.Model.Focus()
}

// Update handles messages.
func (t TextInput) Update(msg tea.Msg) (TextInput, tea.Cmd) {
	if t.NumericOnly {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			key := kmsg.String()
			if len(key) == 1 {
				if key[0] < '0' || key[0] > '9' {
					return t, nil
				}
			}
		}
	}

	var cmd tea.Cmd
	t.Model, cmd = t.Model.Update(msg)
	return t, cmd
}

// View renders the text input.
func (t TextInput) View() string {
	view := t.Model.View()
	if t.submitted {
		if t.valid {
			view += " " + lipgloss.NewStyle().Foreground(theme.Success).Render("✓")
		} else {
			view += " " + lipgloss.NewStyle().Foreground(theme.Error).Render("✗")
		}
	}
	return view
}

// Value returns the current input value.
func (t TextInput) Value() string {
	return t.Model.Value()
}

// NumericValue returns the input value as an integer.
func (t TextInput) NumericValue() (int, error) {
	return strconv.Atoi(t.Model.Value())
}

// Submit marks the input as submitted with a validation result.
func (t *TextInput) Submit(valid bool) {
	t.submitted = true
	t.valid = valid
}
