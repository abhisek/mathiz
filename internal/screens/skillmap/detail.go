package skillmap

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/ui/layout"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// SkillDetailScreen shows details for a single skill.
type SkillDetailScreen struct {
	skill    skillgraph.Skill
	state    skillgraph.SkillState
	mastered map[string]bool
}

var _ screen.Screen = (*SkillDetailScreen)(nil)
var _ screen.KeyHintProvider = (*SkillDetailScreen)(nil)

func newSkillDetail(skill skillgraph.Skill, state skillgraph.SkillState, mastered map[string]bool) *SkillDetailScreen {
	return &SkillDetailScreen{skill: skill, state: state, mastered: mastered}
}

func (d *SkillDetailScreen) Init() tea.Cmd  { return nil }
func (d *SkillDetailScreen) Title() string  { return d.skill.Name }

func (d *SkillDetailScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	return d, nil
}

func (d *SkillDetailScreen) KeyHints() []layout.KeyHint {
	return []layout.KeyHint{
		{Key: "Esc", Description: "Back"},
	}
}

func (d *SkillDetailScreen) View(width, height int) string {
	sk := d.skill
	contentWidth := width - 8
	if contentWidth > 70 {
		contentWidth = 70
	}

	var b strings.Builder

	// Skill name + state.
	b.WriteString(lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true).
		Render(fmt.Sprintf("  %s  %s", d.state.Icon(), sk.Name)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Foreground(theme.TextDim).
		Render(fmt.Sprintf("  %s", d.state.Label())))
	b.WriteString("\n\n")

	// Description.
	if sk.Description != "" {
		b.WriteString(lipgloss.NewStyle().
			Width(contentWidth).
			Foreground(theme.Text).
			PaddingLeft(2).
			Render(sk.Description))
		b.WriteString("\n\n")
	}

	// Metadata.
	dimStyle := lipgloss.NewStyle().Foreground(theme.TextDim)
	valStyle := lipgloss.NewStyle().Foreground(theme.Text)

	b.WriteString(dimStyle.Render("  Strand:    ") + valStyle.Render(skillgraph.StrandDisplayName(sk.Strand)) + "\n")
	b.WriteString(dimStyle.Render("  Grade:     ") + valStyle.Render(fmt.Sprintf("%d", sk.GradeLevel)) + "\n")
	if sk.CommonCoreID != "" {
		b.WriteString(dimStyle.Render("  Standard:  ") + valStyle.Render(sk.CommonCoreID) + "\n")
	}
	b.WriteString("\n")

	// Tier requirements.
	b.WriteString(lipgloss.NewStyle().
		Foreground(theme.Secondary).
		Bold(true).
		Render("  Tier Requirements"))
	b.WriteString("\n")

	for _, tier := range sk.Tiers {
		tierName := "Learn"
		if tier.Tier == skillgraph.TierProve {
			tierName = "Prove"
		}
		acc := fmt.Sprintf("%.0f%%", tier.AccuracyThreshold*100)
		line := fmt.Sprintf("  %-6s  %d questions, %s accuracy", tierName, tier.ProblemsRequired, acc)
		b.WriteString(dimStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Prerequisites.
	prereqs := skillgraph.Prerequisites(sk.ID)
	if len(prereqs) > 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(theme.Secondary).
			Bold(true).
			Render("  Prerequisites"))
		b.WriteString("\n")
		for _, p := range prereqs {
			icon := "○"
			style := dimStyle
			if d.mastered[p.ID] {
				icon = "●"
				style = lipgloss.NewStyle().Foreground(theme.Success)
			}
			b.WriteString(style.Render(fmt.Sprintf("  %s %s", icon, p.Name)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Dependents (what this skill unlocks).
	deps := skillgraph.Dependents(sk.ID)
	if len(deps) > 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(theme.Secondary).
			Bold(true).
			Render("  Unlocks"))
		b.WriteString("\n")
		for _, dep := range deps {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  → %s", dep.Name)))
			b.WriteString("\n")
		}
	}

	return lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top,
		"\n"+b.String())
}
