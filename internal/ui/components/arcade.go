package components

import (
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// ContentWidth returns the uniform inner width used for all arcade sections.
// All boxes are rendered at this width so they visually align.
func ContentWidth(frameWidth int) int {
	// Leave room for cabinet border (2) + inner padding (4)
	w := frameWidth - 6
	if w > 60 {
		w = 60
	}
	if w < 20 {
		w = 20
	}
	return w
}

// CabinetFrame wraps content in a double-border cabinet frame,
// centering vertically and horizontally within the given dimensions.
func CabinetFrame(content string, width, height int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Width(width - 2).
		Height(height - 2).
		Align(lipgloss.Center, lipgloss.Center).
		Render(content)
}

// ArcadeCard wraps content in a rounded-border card at the given content width.
func ArcadeCard(content string, cw int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Width(cw - 2).
		Align(lipgloss.Center).
		Padding(1, 2).
		Render(content)
}

// ArcadeButton renders a styled button matching the home menu style.
func ArcadeButton(label string, selected bool, width int) string {
	if selected {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Bold(true).
			Foreground(theme.BgDark).
			Background(theme.ArcadeYellow).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.ArcadeYellow).
			Padding(0, 1).
			Render("▸ " + label)
	}
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1).
		Render(label)
}
