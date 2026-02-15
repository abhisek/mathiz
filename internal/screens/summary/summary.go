package summary

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/ui/layout"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// SummaryScreen displays the session summary.
type SummaryScreen struct {
	summary *session.SessionSummary
}

var _ screen.Screen = (*SummaryScreen)(nil)
var _ screen.KeyHintProvider = (*SummaryScreen)(nil)

// New creates a new SummaryScreen.
func New(summary *session.SessionSummary) *SummaryScreen {
	return &SummaryScreen{summary: summary}
}

func (s *SummaryScreen) Init() tea.Cmd {
	return nil
}

func (s *SummaryScreen) Title() string {
	return "Session Summary"
}

func (s *SummaryScreen) KeyHints() []layout.KeyHint {
	return []layout.KeyHint{
		{Key: "Enter", Description: "Continue"},
		{Key: "Esc", Description: "Home"},
	}
}

func (s *SummaryScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		switch kmsg.String() {
		case "enter", "esc":
			// Pop both summary and session screens to get back to home.
			return s, func() tea.Msg { return router.PopScreenMsg{} }
		}
	}
	return s, nil
}

func (s *SummaryScreen) View(width, height int) string {
	sum := s.summary
	if sum == nil {
		return ""
	}

	var b strings.Builder

	// Title.
	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Primary).
		Bold(true).
		Render("Session complete!"))
	b.WriteString("\n\n")

	// Duration.
	mins := int(sum.Duration.Minutes())
	secs := int(sum.Duration.Seconds()) % 60
	durationStr := fmt.Sprintf("%d:%02d", mins, secs)
	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.TextDim).
		Render(fmt.Sprintf("Duration: %s", durationStr)))
	b.WriteString("\n\n")

	// Stats line.
	accuracy := fmt.Sprintf("%.0f%%", sum.Accuracy*100)
	statsLine := fmt.Sprintf("Questions: %d        Correct: %d        Accuracy: %s",
		sum.TotalQuestions, sum.TotalCorrect, accuracy)
	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Text).
		Render(statsLine))
	b.WriteString("\n\n")

	// Skills divider.
	divider := lipgloss.NewStyle().Foreground(theme.Border).Render(
		strings.Repeat("─", min(width-8, 60)))
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
		lipgloss.NewStyle().Foreground(theme.TextDim).Render("Skills")))
	b.WriteString("\n")
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, divider))
	b.WriteString("\n\n")

	// Per-skill results.
	for _, sr := range sum.SkillResults {
		if sr.Attempted == 0 {
			continue
		}
		catStr := string(sr.Category)
		scoreStr := fmt.Sprintf("%d/%d correct", sr.Correct, sr.Attempted)

		tierStr := session.TierString(sr.TierAfter)
		if sr.TierBefore != sr.TierAfter {
			tierStr = fmt.Sprintf("%s > %s",
				session.TierString(sr.TierBefore),
				session.TierString(sr.TierAfter))
		}

		fluencyStr := ""
		if sr.FluencyScore >= 0 {
			fluencyStr = fmt.Sprintf("   %.2f", sr.FluencyScore)
		}

		line := fmt.Sprintf("  %s (%s)    %s    %s%s",
			sr.SkillName, catStr, scoreStr, tierStr, fluencyStr)

		style := lipgloss.NewStyle().Foreground(theme.Text)
		if sr.TierBefore != sr.TierAfter {
			style = style.Foreground(theme.Success)
		}
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
			style.Render(line)))
		b.WriteString("\n")
	}

	// Gems section.
	if len(sum.GemsEarned) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
			lipgloss.NewStyle().Foreground(theme.TextDim).Render("Gems")))
		b.WriteString("\n")
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, divider))
		b.WriteString("\n\n")

		for _, gem := range sum.GemsEarned {
			line := fmt.Sprintf("  %s %s %s Gem — %s",
				gem.Type.Icon(),
				gem.Rarity.DisplayName(),
				gem.Type.DisplayName(),
				gem.Reason)
			style := lipgloss.NewStyle().Foreground(rarityColor(gem.Rarity))
			b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
				style.Render(line)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// rarityColor returns the theme color for a gem rarity level.
func rarityColor(r gems.Rarity) color.Color {
	switch r {
	case gems.RarityCommon:
		return theme.Text
	case gems.RarityRare:
		return theme.Secondary
	case gems.RarityEpic:
		return theme.Primary
	case gems.RarityLegendary:
		return theme.Accent
	default:
		return theme.Text
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
