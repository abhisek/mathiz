package history

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

type historyLoadedMsg struct {
	Sessions []store.SessionSummaryRecord
	Gems     map[string][]store.GemEventRecord // sessionID → gems
	Err      error
}

// HistoryScreen displays past sessions and gem awards.
type HistoryScreen struct {
	eventRepo store.EventRepo
	sessions  []store.SessionSummaryRecord
	gems      map[string][]store.GemEventRecord
	selected  int
	expanded  map[int]bool
	loaded    bool
	errMsg    string
}

var _ screen.Screen = (*HistoryScreen)(nil)
var _ screen.KeyHintProvider = (*HistoryScreen)(nil)

// New creates a new HistoryScreen.
func New(eventRepo store.EventRepo) *HistoryScreen {
	return &HistoryScreen{
		eventRepo: eventRepo,
		expanded:  make(map[int]bool),
	}
}

func (s *HistoryScreen) Init() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		sessions, err := s.eventRepo.QuerySessionSummaries(ctx, store.QueryOpts{Limit: 50})
		if err != nil {
			return historyLoadedMsg{Err: err}
		}

		// Load all gem events and group by session.
		allGems, err := s.eventRepo.QueryGemEvents(ctx, store.QueryOpts{})
		if err != nil {
			return historyLoadedMsg{Sessions: sessions, Gems: make(map[string][]store.GemEventRecord)}
		}

		gemsBySession := make(map[string][]store.GemEventRecord)
		for _, g := range allGems {
			gemsBySession[g.SessionID] = append(gemsBySession[g.SessionID], g)
		}

		return historyLoadedMsg{Sessions: sessions, Gems: gemsBySession}
	}
}

func (s *HistoryScreen) Title() string {
	return "History"
}

func (s *HistoryScreen) KeyHints() []layout.KeyHint {
	return []layout.KeyHint{
		{Key: "Enter", Description: "Details"},
		{Key: "↑↓", Description: "Navigate"},
		{Key: "Esc", Description: "Back"},
	}
}

func (s *HistoryScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case historyLoadedMsg:
		if msg.Err != nil {
			s.errMsg = msg.Err.Error()
		} else {
			s.sessions = msg.Sessions
			s.gems = msg.Gems
		}
		s.loaded = true
		return s, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return s, func() tea.Msg { return router.PopScreenMsg{} }
		case "up", "k":
			if s.selected > 0 {
				s.selected--
			}
			return s, nil
		case "down", "j":
			if s.selected < len(s.sessions)-1 {
				s.selected++
			}
			return s, nil
		case "enter":
			s.expanded[s.selected] = !s.expanded[s.selected]
			return s, nil
		}
	}
	return s, nil
}

func (s *HistoryScreen) View(width, height int) string {
	if s.errMsg != "" {
		return lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.Error).
			Render(fmt.Sprintf("\n\nError: %s", s.errMsg))
	}
	if !s.loaded {
		return lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.TextDim).
			Render("\n\n  Loading history...")
	}
	if len(s.sessions) == 0 {
		return lipgloss.NewStyle().
			Width(width).Align(lipgloss.Center).Foreground(theme.TextDim).Italic(true).
			Render("\n\n  No sessions yet. Start practicing!")
	}

	var b strings.Builder
	b.WriteString("\n")

	for i, sess := range s.sessions {
		dateStr := sess.Timestamp.Format("Jan 02, 2006")
		mins := sess.DurationSecs / 60
		secs := sess.DurationSecs % 60
		durationStr := fmt.Sprintf("%d:%02d", mins, secs)

		var accuracy float64
		if sess.QuestionsServed > 0 {
			accuracy = float64(sess.CorrectAnswers) / float64(sess.QuestionsServed) * 100
		}

		gemStr := ""
		if sess.GemCount > 0 {
			gemStr = fmt.Sprintf("  %d gem", sess.GemCount)
			if sess.GemCount > 1 {
				gemStr += "s"
			}
		}

		prefix := "  "
		if i == s.selected {
			prefix = "> "
		}

		line := fmt.Sprintf("%s%s  %s  %d questions  %.0f%% accuracy%s",
			prefix, dateStr, durationStr, sess.QuestionsServed, accuracy, gemStr)

		style := lipgloss.NewStyle().Foreground(theme.Text)
		if i == s.selected {
			style = style.Foreground(theme.Primary).Bold(true)
		}
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
			style.Render(line)))
		b.WriteString("\n")

		// Show expanded gem details.
		if s.expanded[i] {
			sessionGems := s.gems[sess.SessionID]
			if len(sessionGems) == 0 {
				b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
					lipgloss.NewStyle().Foreground(theme.TextDim).Italic(true).
						Render("    No gems this session")))
				b.WriteString("\n")
			} else {
				for _, g := range sessionGems {
					gemType := gems.GemType(g.GemType)
					rarity := gems.Rarity(g.Rarity)
					gemLine := fmt.Sprintf("    %s %s %s Gem — %s",
						gemType.Icon(), rarity.DisplayName(), gemType.DisplayName(), g.Reason)
					b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
						lipgloss.NewStyle().Foreground(rarityColor(rarity)).Render(gemLine)))
					b.WriteString("\n")
				}
			}
		}
	}

	return b.String()
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
