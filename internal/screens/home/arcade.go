package home

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// Block-letter title (same art as welcome/banner.go).
const arcadeTitleFull = ` ███╗   ███╗ █████╗ ████████╗██╗  ██╗██╗███████╗
 ████╗ ████║██╔══██╗╚══██╔══╝██║  ██║██║╚══███╔╝
 ██╔████╔██║███████║   ██║   ███████║██║  ███╔╝
 ██║╚██╔╝██║██╔══██║   ██║   ██╔══██║██║ ███╔╝
 ██║ ╚═╝ ██║██║  ██║   ██║   ██║  ██║██║███████╗
 ╚═╝     ╚═╝╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝╚═╝╚══════╝`

const arcadeTitleCompact = "M · A · T · H · I · Z"

// contentWidth returns the uniform inner width used for all sections.
// All boxes are rendered at this width so they visually align.
func contentWidth(frameWidth int) int {
	// Leave room for cabinet border (2) + inner padding (4)
	w := frameWidth - 6
	// Cap so it doesn't stretch absurdly wide
	if w > 60 {
		w = 60
	}
	if w < 20 {
		w = 20
	}
	return w
}

// renderTitle returns the styled title block or compact fallback.
func renderTitle(cw int, compact bool) string {
	style := lipgloss.NewStyle().
		Foreground(theme.ArcadeYellow).
		Bold(true)

	if compact {
		return lipgloss.NewStyle().
			Width(cw).
			Align(lipgloss.Center).
			Render(style.Render(arcadeTitleCompact))
	}
	return lipgloss.NewStyle().
		Width(cw).
		Align(lipgloss.Center).
		Render(style.Render(arcadeTitleFull))
}

// renderStatsBar renders the dashboard stats in a bordered box matching content width.
func renderStatsBar(mastered, gems, reviewsDue, cw int, compact bool) string {
	masteredStyle := lipgloss.NewStyle().Foreground(theme.ArcadeYellow).Bold(true)
	gemStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	reviewStyle := lipgloss.NewStyle().Foreground(theme.ArcadeCyan).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(theme.TextDim)

	var stats string
	if compact {
		stats = fmt.Sprintf("%s %s %s",
			masteredStyle.Render(fmt.Sprintf("★%d", mastered)),
			gemStyle.Render(fmt.Sprintf("◆%d", gems)),
			reviewText(reviewsDue, true, reviewStyle, dimStyle),
		)
	} else {
		stats = fmt.Sprintf("%s  %s  %s",
			masteredStyle.Render(fmt.Sprintf("★ %d MASTERED", mastered)),
			gemStyle.Render(fmt.Sprintf("◆ %d GEMS", gems)),
			reviewText(reviewsDue, false, reviewStyle, dimStyle),
		)
	}

	// Wrap in a double-border box at the same content width
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.ArcadeCyan).
		Width(cw - 2). // account for border chars
		Align(lipgloss.Center).
		Padding(0, 1).
		Render(stats)
}

func reviewText(due int, compact bool, active, dim lipgloss.Style) string {
	if due == 0 {
		if compact {
			return dim.Render("⚡0")
		}
		return dim.Render("⚡ NONE DUE")
	}
	if compact {
		return active.Render(fmt.Sprintf("⚡%d", due))
	}
	return active.Render(fmt.Sprintf("⚡ %d DUE", due))
}

// buttonWidth is the fixed width for menu buttons.
const buttonWidth = 22

// renderArcadeMenu renders each menu item as a fixed-width button.
func renderArcadeMenu(items []string, selected int, cw int) string {
	selectedBtn := lipgloss.NewStyle().
		Width(buttonWidth).
		Align(lipgloss.Center).
		Bold(true).
		Foreground(theme.BgDark).
		Background(theme.ArcadeYellow).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ArcadeYellow).
		Padding(0, 1)

	normalBtn := lipgloss.NewStyle().
		Width(buttonWidth).
		Align(lipgloss.Center).
		Foreground(theme.Text).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(0, 1)

	var buttons []string
	for i, label := range items {
		if i == selected {
			buttons = append(buttons, selectedBtn.Render("▸ "+label))
		} else {
			buttons = append(buttons, normalBtn.Render(label))
		}
	}
	block := strings.Join(buttons, "\n")

	return lipgloss.NewStyle().
		Width(cw).
		Align(lipgloss.Center).
		Render(block)
}

// renderMascotBox renders the mascot centered in a box matching content width.
func renderMascotBox(variant MascotVariant, cw int) string {
	return lipgloss.NewStyle().
		Width(cw).
		Align(lipgloss.Center).
		Render(RenderMascot(variant))
}

// renderCabinetFrame wraps content in a double-border cabinet frame,
// centering vertically and horizontally within the given dimensions.
func renderCabinetFrame(content string, width, height int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Width(width - 2).   // account for border chars
		Height(height - 2). // account for border chars
		Align(lipgloss.Center, lipgloss.Center).
		Render(content)
}
