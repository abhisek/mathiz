package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

// ProgressBar displays a horizontal progress bar.
type ProgressBar struct {
	Label       string
	Percent     float64
	ShowPercent bool
	Width       int
}

// NewProgressBar creates a new progress bar.
func NewProgressBar(label string, percent float64, showPercent bool, width int) ProgressBar {
	return ProgressBar{
		Label:       label,
		Percent:     percent,
		ShowPercent: showPercent,
		Width:       width,
	}
}

// View renders the progress bar.
func (p ProgressBar) View() string {
	var result string

	if p.Label != "" {
		result += lipgloss.NewStyle().Foreground(theme.Text).Render(p.Label) + "  "
	}

	labelWidth := lipgloss.Width(result)
	percentWidth := 0
	if p.ShowPercent {
		percentWidth = 6 // " 100%"
	}

	barWidth := p.Width - labelWidth - percentWidth
	if barWidth < 4 {
		barWidth = 4
	}

	filled := int(float64(barWidth) * p.Percent)
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}
	empty := barWidth - filled

	filledStr := lipgloss.NewStyle().
		Background(theme.Secondary).
		Render(strings.Repeat(" ", filled))

	emptyStr := lipgloss.NewStyle().
		Background(theme.Border).
		Render(strings.Repeat(" ", empty))

	result += filledStr + emptyStr

	if p.ShowPercent {
		result += lipgloss.NewStyle().
			Foreground(theme.TextDim).
			Render(fmt.Sprintf("  %d%%", int(p.Percent*100)))
	}

	return result
}
