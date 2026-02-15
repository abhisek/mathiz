package skillmap

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/ui/layout"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

type rowKind int

const (
	rowStrandHeader rowKind = iota
	rowSkill
)

type row struct {
	kind   rowKind
	strand skillgraph.Strand
	skill  *skillgraph.Skill
}

// SkillMapScreen displays the skill graph organized by strand.
type SkillMapScreen struct {
	rows         []row
	cursor       int
	scrollOffset int
	mastered     map[string]bool
}

var _ screen.Screen = (*SkillMapScreen)(nil)

// New creates a new SkillMapScreen.
func New() *SkillMapScreen {
	mastered := make(map[string]bool)

	var rows []row
	for _, strand := range skillgraph.AllStrands() {
		rows = append(rows, row{kind: rowStrandHeader, strand: strand})
		skills := skillgraph.ByStrand(strand)
		for i := range skills {
			rows = append(rows, row{kind: rowSkill, strand: strand, skill: &skills[i]})
		}
	}

	s := &SkillMapScreen{
		rows:     rows,
		mastered: mastered,
	}

	// Set cursor to first skill row
	for i, r := range s.rows {
		if r.kind == rowSkill {
			s.cursor = i
			break
		}
	}

	return s
}

func (s *SkillMapScreen) Init() tea.Cmd {
	return nil
}

func (s *SkillMapScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			s.moveCursor(-1)
		case "down", "j":
			s.moveCursor(1)
		case "tab":
			s.nextStrand()
		case "shift+tab":
			s.prevStrand()
		case "enter":
			return s, s.selectSkill()
		case "q":
			return s, func() tea.Msg { return router.PopScreenMsg{} }
		}
	}
	return s, nil
}

func (s *SkillMapScreen) View(width, height int) string {
	if len(s.rows) == 0 {
		return ""
	}

	// Ensure cursor is visible within the scroll window
	s.adjustScroll(height)

	// Render all visible rows
	var lines []string
	visible := 0
	for i, r := range s.rows {
		if i < s.scrollOffset {
			continue
		}
		if visible >= height {
			break
		}

		switch r.kind {
		case rowStrandHeader:
			lines = append(lines, s.renderStrandHeader(r.strand, width))
		case rowSkill:
			lines = append(lines, s.renderSkillRow(r, i == s.cursor, width))
		}
		visible++
	}

	return strings.Join(lines, "\n")
}

func (s *SkillMapScreen) Title() string {
	return "Skill Map"
}

// KeyHints returns the key binding hints for the footer.
func (s *SkillMapScreen) KeyHints() []layout.KeyHint {
	return []layout.KeyHint{
		{Key: "↑↓", Description: "Navigate"},
		{Key: "Tab", Description: "Strand"},
		{Key: "Enter", Description: "Select"},
		{Key: "Esc", Description: "Back"},
	}
}

// moveCursor moves the cursor by delta, skipping strand headers.
func (s *SkillMapScreen) moveCursor(delta int) {
	next := s.cursor + delta
	for next >= 0 && next < len(s.rows) {
		if s.rows[next].kind == rowSkill {
			s.cursor = next
			return
		}
		next += delta
	}
}

// nextStrand jumps the cursor to the first skill in the next strand.
func (s *SkillMapScreen) nextStrand() {
	currentStrand := s.rows[s.cursor].strand
	for i := s.cursor + 1; i < len(s.rows); i++ {
		if s.rows[i].kind == rowSkill && s.rows[i].strand != currentStrand {
			s.cursor = i
			return
		}
	}
}

// prevStrand jumps the cursor to the first skill in the previous strand.
func (s *SkillMapScreen) prevStrand() {
	currentStrand := s.rows[s.cursor].strand

	// Find the start of the previous strand
	prevStrandStart := -1
	var prevStrand skillgraph.Strand
	for i := s.cursor - 1; i >= 0; i-- {
		if s.rows[i].kind == rowSkill && s.rows[i].strand != currentStrand {
			prevStrand = s.rows[i].strand
			prevStrandStart = i
			break
		}
	}
	if prevStrandStart < 0 {
		return
	}

	// Go to the first skill of that strand
	for i := prevStrandStart; i >= 0; i-- {
		if s.rows[i].kind != rowSkill || s.rows[i].strand != prevStrand {
			s.cursor = i + 1
			return
		}
	}
	s.cursor = 0
	if s.rows[0].kind != rowSkill {
		s.moveCursor(1)
	}
}

// adjustScroll ensures the cursor is visible within the viewport.
func (s *SkillMapScreen) adjustScroll(height int) {
	if height <= 0 {
		return
	}
	// Also show the strand header above the cursor if possible
	headerRow := s.cursor
	for headerRow > 0 && s.rows[headerRow-1].kind == rowStrandHeader {
		headerRow--
	}

	if headerRow < s.scrollOffset {
		s.scrollOffset = headerRow
	}
	if s.cursor >= s.scrollOffset+height {
		s.scrollOffset = s.cursor - height + 1
	}
}

// selectSkill handles enter on the current skill.
func (s *SkillMapScreen) selectSkill() tea.Cmd {
	r := s.rows[s.cursor]
	if r.kind != rowSkill || r.skill == nil {
		return nil
	}

	state := s.skillState(r.skill.ID)
	detail := newSkillDetail(*r.skill, state, s.mastered)
	return func() tea.Msg {
		return router.PushScreenMsg{Screen: detail}
	}
}

// skillState computes the display state for a skill.
func (s *SkillMapScreen) skillState(id string) skillgraph.SkillState {
	if s.mastered[id] {
		return skillgraph.StateMastered
	}
	if skillgraph.IsUnlocked(id, s.mastered) {
		return skillgraph.StateAvailable
	}
	return skillgraph.StateLocked
}

// renderStrandHeader renders a strand section header.
func (s *SkillMapScreen) renderStrandHeader(strand skillgraph.Strand, width int) string {
	name := strings.ToUpper(skillgraph.StrandDisplayName(strand))
	styled := lipgloss.NewStyle().
		Foreground(theme.Secondary).
		Bold(true).
		Width(width).
		Padding(1, 0, 0, 2).
		Render(name)
	return styled
}

// renderSkillRow renders a single skill row.
func (s *SkillMapScreen) renderSkillRow(r row, selected bool, width int) string {
	if r.skill == nil {
		return ""
	}

	state := s.skillState(r.skill.ID)
	icon := state.Icon()
	label := state.Label()
	grade := fmt.Sprintf("Grade %d", r.skill.GradeLevel)

	// Calculate column widths
	padding := 4 // left indent
	iconWidth := 3
	gradeWidth := 8
	labelWidth := 10
	spacing := 4
	nameWidth := width - padding - iconWidth - gradeWidth - labelWidth - spacing
	if nameWidth < 10 {
		nameWidth = 10
	}

	// Truncate name if needed
	name := r.skill.Name
	if len(name) > nameWidth {
		name = name[:nameWidth-1] + "…"
	}

	// Build the row
	var nameStyle, gradeStyle, labelStyle lipgloss.Style
	if selected {
		nameStyle = lipgloss.NewStyle().Foreground(theme.Primary).Bold(true)
		gradeStyle = lipgloss.NewStyle().Foreground(theme.Primary)
		labelStyle = lipgloss.NewStyle().Foreground(theme.Primary)
	} else {
		switch state {
		case skillgraph.StateMastered:
			nameStyle = lipgloss.NewStyle().Foreground(theme.Success)
			gradeStyle = lipgloss.NewStyle().Foreground(theme.TextDim)
			labelStyle = lipgloss.NewStyle().Foreground(theme.Success)
		case skillgraph.StateAvailable:
			nameStyle = lipgloss.NewStyle().Foreground(theme.Text)
			gradeStyle = lipgloss.NewStyle().Foreground(theme.TextDim)
			labelStyle = lipgloss.NewStyle().Foreground(theme.Secondary)
		case skillgraph.StateLocked:
			nameStyle = lipgloss.NewStyle().Foreground(theme.TextDim)
			gradeStyle = lipgloss.NewStyle().Foreground(theme.TextDim)
			labelStyle = lipgloss.NewStyle().Foreground(theme.TextDim)
		default:
			nameStyle = lipgloss.NewStyle().Foreground(theme.Text)
			gradeStyle = lipgloss.NewStyle().Foreground(theme.TextDim)
			labelStyle = lipgloss.NewStyle().Foreground(theme.Text)
		}
	}

	// Cursor indicator
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Format with padding
	namePadded := fmt.Sprintf("%-*s", nameWidth, name)
	row := fmt.Sprintf("  %s%s %s  %s  %s",
		cursor,
		icon,
		nameStyle.Render(namePadded),
		gradeStyle.Render(grade),
		labelStyle.Render(fmt.Sprintf("%9s", label)),
	)

	return row
}
