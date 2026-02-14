package screen

import tea "charm.land/bubbletea/v2"

// Screen defines the interface for all application screens.
type Screen interface {
	// Init returns an initial command when the screen is first created.
	Init() tea.Cmd

	// Update handles messages and returns updated screen + command.
	Update(msg tea.Msg) (Screen, tea.Cmd)

	// View renders the screen content (excluding header/footer).
	View(width, height int) string

	// Title returns the screen name for the header.
	Title() string
}
