package home

import (
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// MascotVariant selects which mascot art to display.
type MascotVariant int

const (
	MascotIdle        MascotVariant = iota // Default purple
	MascotCelebrating                      // Gold, star eyes — recent mastery
	MascotAlert                            // Orange, exclamation — reviews due
)

const mascotIdle = `┌─────┐
│ ◉ ◉ │
│  ▽  │
│ ±×÷ │
└─────┘`

const mascotCelebrating = `┌─────┐
│ ★ ★ │
│  ▿  │
│ ±×÷ │
└─╥═╥─┘
  ╚═╝`

const mascotAlert = `┌─────┐
│ ◉ ◉ │ !
│  ▽  │
│ ±×÷ │
└─────┘`

// RenderMascot returns the mascot ASCII art for the given variant.
func RenderMascot(variant ...MascotVariant) string {
	v := MascotIdle
	if len(variant) > 0 {
		v = variant[0]
	}

	var art string
	var fg = theme.Primary

	switch v {
	case MascotCelebrating:
		art = mascotCelebrating
		fg = theme.ArcadeYellow
	case MascotAlert:
		art = mascotAlert
		fg = theme.Accent
	default:
		art = mascotIdle
	}

	return lipgloss.NewStyle().
		Foreground(fg).
		Render(art)
}
