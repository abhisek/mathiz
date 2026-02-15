package gemvault

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/abhisek/mathiz/internal/ui/layout"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

type gemsLoadedMsg struct {
	Records []store.GemEventRecord
	Err     error
}

// GemVaultScreen displays the learner's gem collection.
type GemVaultScreen struct {
	eventRepo    store.EventRepo
	allGems      []store.GemEventRecord
	selectedType int // index into AllGemTypes
	scrollOffset int
	loaded       bool
	errMsg       string
}

var _ screen.Screen = (*GemVaultScreen)(nil)
var _ screen.KeyHintProvider = (*GemVaultScreen)(nil)

// New creates a new GemVaultScreen.
func New(eventRepo store.EventRepo) *GemVaultScreen {
	return &GemVaultScreen{
		eventRepo: eventRepo,
	}
}

func (s *GemVaultScreen) Init() tea.Cmd {
	return func() tea.Msg {
		records, err := s.eventRepo.QueryGemEvents(context.Background(), store.QueryOpts{})
		return gemsLoadedMsg{Records: records, Err: err}
	}
}

func (s *GemVaultScreen) Title() string {
	return "Gem Vault"
}

func (s *GemVaultScreen) KeyHints() []layout.KeyHint {
	return []layout.KeyHint{
		{Key: "Tab", Description: "Switch type"},
		{Key: "↑↓", Description: "Scroll"},
		{Key: "Esc", Description: "Back"},
	}
}

func (s *GemVaultScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case gemsLoadedMsg:
		if msg.Err != nil {
			s.errMsg = msg.Err.Error()
		} else {
			s.allGems = msg.Records
		}
		s.loaded = true
		return s, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return s, func() tea.Msg { return router.PopScreenMsg{} }
		case "tab":
			types := gems.AllGemTypes()
			s.selectedType = (s.selectedType + 1) % len(types)
			s.scrollOffset = 0
			return s, nil
		case "shift+tab":
			types := gems.AllGemTypes()
			s.selectedType = (s.selectedType - 1 + len(types)) % len(types)
			s.scrollOffset = 0
			return s, nil
		case "up", "k":
			if s.scrollOffset > 0 {
				s.scrollOffset--
			}
			return s, nil
		case "down", "j":
			filtered := s.filteredGems()
			if s.scrollOffset < len(filtered)-1 {
				s.scrollOffset++
			}
			return s, nil
		}
	}
	return s, nil
}

func (s *GemVaultScreen) View(width, height int) string {
	if s.errMsg != "" {
		return lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.Error).
			Render(fmt.Sprintf("\n\nError: %s", s.errMsg))
	}
	if !s.loaded {
		return lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.TextDim).
			Render("\n\n  Loading gems...")
	}

	var b strings.Builder

	// Total count.
	b.WriteString(lipgloss.NewStyle().
		Width(width).Align(lipgloss.Center).Foreground(theme.Text).
		Render(fmt.Sprintf("\nTotal: %d gems\n", len(s.allGems))))
	b.WriteString("\n")

	// Type tabs.
	types := gems.AllGemTypes()
	var tabs []string
	for i, t := range types {
		count := s.countByType(t)
		label := fmt.Sprintf("%s %s (%d)", t.Icon(), t.DisplayName(), count)
		if i == s.selectedType {
			tabs = append(tabs, lipgloss.NewStyle().Foreground(theme.Primary).Bold(true).Render(label))
		} else {
			tabs = append(tabs, lipgloss.NewStyle().Foreground(theme.TextDim).Render(label))
		}
	}
	tabLine := strings.Join(tabs, "     ")
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, tabLine))
	b.WriteString("\n\n")

	// Divider.
	divider := lipgloss.NewStyle().Foreground(theme.Border).Render(
		strings.Repeat("─", min(width-8, 60)))
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, divider))
	b.WriteString("\n\n")

	// Filtered gems list.
	filtered := s.filteredGems()
	if len(filtered) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.TextDim).Italic(true).
			Render("No gems of this type yet"))
		return b.String()
	}

	// Show visible items within height constraint.
	maxVisible := height - 10
	if maxVisible < 3 {
		maxVisible = 3
	}
	start := s.scrollOffset
	end := start + maxVisible
	if end > len(filtered) {
		end = len(filtered)
	}

	for i := start; i < end; i++ {
		rec := filtered[i]
		rarity := gems.Rarity(rec.Rarity)
		rarityLabel := rarity.DisplayName()
		dateStr := rec.Timestamp.Format("Jan 02, 2006")

		skillName := ""
		if rec.SkillName != nil {
			skillName = *rec.SkillName
		}

		var line string
		if skillName != "" {
			line = fmt.Sprintf("  %-10s %-30s %s", rarityLabel, skillName, dateStr)
		} else {
			line = fmt.Sprintf("  %-10s %-30s %s", rarityLabel, rec.Reason, dateStr)
		}

		style := lipgloss.NewStyle().Foreground(rarityColor(rarity))
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
			style.Render(line)))
		b.WriteString("\n")
	}

	if end < len(filtered) {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.TextDim).
			Render(fmt.Sprintf("... %d more", len(filtered)-end)))
	}

	return b.String()
}

func (s *GemVaultScreen) filteredGems() []store.GemEventRecord {
	types := gems.AllGemTypes()
	selectedType := string(types[s.selectedType])
	var filtered []store.GemEventRecord
	for _, g := range s.allGems {
		if g.GemType == selectedType {
			filtered = append(filtered, g)
		}
	}
	return filtered
}

func (s *GemVaultScreen) countByType(t gems.GemType) int {
	count := 0
	for _, g := range s.allGems {
		if g.GemType == string(t) {
			count++
		}
	}
	return count
}

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
