package home

import (
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

const mascotArt = `  ╭───────────╮
  │  ┌─────┐  │
  │  │ ◉ ◉ │  │
  │  │  ▽  │  │
  │  ├─────┤  │
  │  │ ±×÷ │  │
  │  └─────┘  │
  ╰───────────╯`

// RenderMascot returns the mascot ASCII art styled in the primary color.
func RenderMascot() string {
	return lipgloss.NewStyle().
		Foreground(theme.Primary).
		Render(mascotArt)
}
