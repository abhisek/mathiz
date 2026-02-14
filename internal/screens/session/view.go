package session

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	sess "github.com/abhisek/mathiz/internal/session"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// renderQuestionView renders the active question display.
func (s *SessionScreen) renderQuestionView(width, height int) string {
	state := s.state
	if state == nil || state.CurrentQuestion == nil {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Foreground(theme.TextDim).
			Render("\n\n  Generating question...")
	}

	slot := sess.CurrentSlot(state)
	var skillName string
	if slot != nil {
		skillName = slot.Skill.Name
	}

	var b strings.Builder

	// Skill info line.
	remaining := state.Plan.Duration - state.Elapsed
	if remaining < 0 {
		remaining = 0
	}
	mins := int(remaining.Minutes())
	secs := int(remaining.Seconds()) % 60
	timerStr := fmt.Sprintf("%d:%02d", mins, secs)

	infoLeft := lipgloss.NewStyle().
		Foreground(theme.Secondary).
		Bold(true).
		Render(fmt.Sprintf("  Skill: %s", skillName))

	infoRight := lipgloss.NewStyle().
		Foreground(theme.TextDim).
		Render(fmt.Sprintf("Q %d/~15  %s %d  %s %s",
			state.TotalQuestions+1,
			lipgloss.NewStyle().Foreground(theme.Success).Render("*"),
			state.TotalCorrect,
			lipgloss.NewStyle().Foreground(theme.Accent).Render("T"),
			timerStr,
		))

	infoLine := infoLeft
	rightPad := width - lipgloss.Width(infoLeft) - lipgloss.Width(infoRight) - 4
	if rightPad > 0 {
		infoLine += strings.Repeat(" ", rightPad) + infoRight
	}

	b.WriteString(infoLine)
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", width-4)))
	b.WriteString("\n\n")

	q := state.CurrentQuestion

	// Question text (centered).
	questionStyle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Text).
		Bold(true)
	b.WriteString(questionStyle.Render(q.Text))
	b.WriteString("\n\n")

	// Input area.
	if s.mcActive {
		b.WriteString(s.renderMultipleChoice(width))
	} else {
		answerLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render("Answer: " + s.input.View())
		b.WriteString(answerLine)
	}

	return b.String()
}

// renderMultipleChoice renders multiple choice options.
func (s *SessionScreen) renderMultipleChoice(width int) string {
	q := s.state.CurrentQuestion
	if q == nil {
		return ""
	}

	var b strings.Builder
	for i, choice := range q.Choices {
		prefix := "  "
		if i == s.mcSelected {
			prefix = "> "
		}
		line := fmt.Sprintf("%s%d) %s", prefix, i+1, choice)

		if i == s.mcSelected {
			b.WriteString(lipgloss.NewStyle().Foreground(theme.Primary).Bold(true).Render(line))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(theme.Text).Render(line))
		}
		b.WriteString("\n")
	}

	selectLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.TextDim).
		Render("\nSelect (1-4) or use arrows + Enter")
	b.WriteString(selectLine)

	// Center the whole block.
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, b.String())
}

// renderFeedback renders the feedback overlay.
func (s *SessionScreen) renderFeedback(width, height int) string {
	state := s.state
	q := state.CurrentQuestion

	var b strings.Builder
	b.WriteString("\n\n")

	if state.LastAnswerCorrect {
		b.WriteString(lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Foreground(theme.Success).
			Bold(true).
			Render("Correct!"))
	} else {
		b.WriteString(lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Foreground(theme.Error).
			Bold(true).
			Render("Not quite"))
		if q != nil {
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.TextDim).
				Render(fmt.Sprintf("Correct answer: %s", q.Answer)))
		}
	}

	b.WriteString("\n\n")

	// Explanation.
	if q != nil && q.Explanation != "" {
		expStyle := lipgloss.NewStyle().
			Width(min(width-8, 70)).
			Foreground(theme.Text)
		exp := expStyle.Render(q.Explanation)
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, exp))
		b.WriteString("\n\n")
	}

	// Mastery transition / tier advancement notification.
	if state.MasteryTransition != nil {
		t := state.MasteryTransition
		switch t.Trigger {
		case "prove-complete":
			fluencyStr := ""
			if state.MasteryService != nil {
				sm := state.MasteryService.GetMastery(t.SkillID)
				fluencyStr = fmt.Sprintf(" Fluency: %.2f", sm.FluencyScore())
			}
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Accent).
				Bold(true).
				Render("Skill mastered!"))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Text).
				Render(fmt.Sprintf("\"%s\" — mastered!%s", t.SkillName, fluencyStr)))
			b.WriteString("\n\n")
		case "recovery-complete":
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Success).
				Bold(true).
				Render("Skill recovered!"))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Text).
				Render(fmt.Sprintf("\"%s\" — back to mastered!", t.SkillName)))
			b.WriteString("\n\n")
		case "tier-complete":
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Accent).
				Bold(true).
				Render("Level up!"))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Text).
				Render(fmt.Sprintf("\"%s\" — Prove tier unlocked!", t.SkillName)))
			b.WriteString("\n\n")
		}
	} else if state.TierAdvanced != nil {
		// Legacy tier advancement notification.
		adv := state.TierAdvanced
		if adv.Mastered {
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Accent).
				Bold(true).
				Render("Skill mastered!"))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Text).
				Render(fmt.Sprintf("\"%s\" — fully mastered!", adv.SkillName)))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Accent).
				Bold(true).
				Render("Level up!"))
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Foreground(theme.Text).
				Render(fmt.Sprintf("\"%s\" — Prove tier unlocked!", adv.SkillName)))
		}
		b.WriteString("\n\n")
	}

	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.TextDim).
		Render("Press any key to continue..."))

	return b.String()
}

// renderQuitConfirm renders the quit confirmation dialog.
func renderQuitConfirm(width, height int) string {
	var b strings.Builder
	b.WriteString("\n\n\n")

	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Text).
		Bold(true).
		Render("End session early?"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.TextDim).
		Render("Your progress will be saved."))
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Success).
		Render("[Y] Yes, end session"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Primary).
		Render("[N] No, keep going"))

	return b.String()
}

// renderLoading renders the loading state.
func renderLoading(width, height int) string {
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.TextDim).
		Render("\n\n\n  Preparing your session...")
}

// renderError renders an error message.
func renderError(width, height int, errMsg string) string {
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Foreground(theme.Error).
		Render(fmt.Sprintf("\n\n\n  Error: %s\n\n  Press any key to go back.", errMsg))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
