package welcome

import (
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

const bannerArt = `
 ███╗   ███╗ █████╗ ████████╗██╗  ██╗██╗███████╗
 ████╗ ████║██╔══██╗╚══██╔══╝██║  ██║██║╚══███╔╝
 ██╔████╔██║███████║   ██║   ███████║██║  ███╔╝
 ██║╚██╔╝██║██╔══██║   ██║   ██╔══██║██║ ███╔╝
 ██║ ╚═╝ ██║██║  ██║   ██║   ██║  ██║██║███████╗
 ╚═╝     ╚═╝╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝╚═╝╚══════╝`

const bannerCompact = "M A T H I Z"

// RenderBanner returns the MATHIZ banner styled in the primary color.
// Uses a compact fallback for terminals narrower than 52 columns.
func RenderBanner(width int) string {
	style := lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true)

	if width < 52 {
		return style.Render(bannerCompact)
	}
	return style.Render(bannerArt)
}
