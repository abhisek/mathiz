package home

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/router"
	"github.com/abhisek/mathiz/internal/screen"
	"github.com/abhisek/mathiz/internal/screens/gemvault"
	"github.com/abhisek/mathiz/internal/screens/history"
	"github.com/abhisek/mathiz/internal/screens/placeholder"
	sessionscreen "github.com/abhisek/mathiz/internal/screens/session"
	"github.com/abhisek/mathiz/internal/screens/skillmap"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/abhisek/mathiz/internal/ui/components"
	"github.com/abhisek/mathiz/internal/ui/theme"
)

// HomeScreen is the main home screen of the application.
type HomeScreen struct {
	menu     components.Menu
	gemCount int
}

var _ screen.Screen = (*HomeScreen)(nil)

// New creates a new HomeScreen.
func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service, lessonService *lessons.Service, compressor *lessons.Compressor, gemService *gems.Service) *HomeScreen {
	// Load snapshot for gem count and skill states.
	var snap *store.Snapshot
	if snapRepo != nil {
		snap, _ = snapRepo.Latest(context.Background())
	}

	var gemCount int
	if snap != nil && snap.Data.Gems != nil {
		gemCount = snap.Data.Gems.TotalCount
	}

	skillStates := computeSkillStates(snap)
	reviewBadges := computeReviewBadges(snap)

	items := []components.MenuItem{
		{Label: "Start Game", Action: func() tea.Cmd {
			if generator == nil || eventRepo == nil || snapRepo == nil {
				return func() tea.Msg {
					return router.PushScreenMsg{Screen: placeholder.New("Start Game")}
				}
			}
			return func() tea.Msg {
				return router.PushScreenMsg{
					Screen: sessionscreen.New(generator, eventRepo, snapRepo, diagService, lessonService, compressor, gemService),
				}
			}
		}},
		{Label: "Skill Map", Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: skillmap.New(skillStates, reviewBadges)}
			}
		}},
		{Label: "Gem Vault", Action: func() tea.Cmd {
			if eventRepo == nil {
				return func() tea.Msg {
					return router.PushScreenMsg{Screen: placeholder.New("Gem Vault")}
				}
			}
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: gemvault.New(eventRepo)}
			}
		}},
		{Label: "History", Action: func() tea.Cmd {
			if eventRepo == nil {
				return func() tea.Msg {
					return router.PushScreenMsg{Screen: placeholder.New("History")}
				}
			}
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: history.New(eventRepo)}
			}
		}},
		{Label: "Exit Game", Action: func() tea.Cmd {
			return tea.Quit
		}},
	}

	return &HomeScreen{
		menu:     components.NewMenu(items),
		gemCount: gemCount,
	}
}

func (h *HomeScreen) Init() tea.Cmd {
	return nil
}

func (h *HomeScreen) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	var cmd tea.Cmd
	h.menu, cmd = h.menu.Update(msg)
	return h, cmd
}

func (h *HomeScreen) View(width, height int) string {
	// height is the content area; estimate full terminal height
	// by adding back header (3) + footer (3) + frame gaps
	termHeight := height + 8
	compact := termHeight < 30 || width < 100

	var sections []string

	if !compact {
		mascot := lipgloss.PlaceHorizontal(width, lipgloss.Center, RenderMascot())
		sections = append(sections, mascot)
	}

	var greetingText string
	if compact {
		greetingText = "Hey there, math explorer! ✦"
	} else {
		greetingText = "Hey there, math explorer!\nReady to level up today? ✦"
	}

	greeting := lipgloss.NewStyle().
		Width(width).
		Foreground(theme.Text).
		Align(lipgloss.Center).
		Render(greetingText)
	sections = append(sections, greeting)

	// Gem count display.
	if h.gemCount > 0 {
		gemLine := fmt.Sprintf("✦ %d gems", h.gemCount)
		gemDisplay := lipgloss.NewStyle().
			Width(width).
			Foreground(theme.Accent).
			Align(lipgloss.Center).
			Render(gemLine)
		sections = append(sections, gemDisplay)
	}

	// Render the menu as a left-aligned block, then center the whole block
	menuBlock := h.menu.View()
	centeredMenu := lipgloss.PlaceHorizontal(width, lipgloss.Center, menuBlock)
	sections = append(sections, centeredMenu)

	content := "\n" + strings.Join(sections, "\n\n")

	// Fill the full width×height to prevent artifacts from previous screens.
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (h *HomeScreen) Title() string {
	return "Home"
}

// computeReviewBadges extracts review schedule badges from snapshot spaced rep data.
func computeReviewBadges(snap *store.Snapshot) map[string]skillmap.ReviewBadge {
	badges := make(map[string]skillmap.ReviewBadge)
	if snap == nil || snap.Data.SpacedRep == nil {
		return badges
	}
	now := time.Now()
	for id, rs := range snap.Data.SpacedRep.Reviews {
		nextReview, err := time.Parse(time.RFC3339, rs.NextReviewDate)
		if err != nil {
			continue
		}
		badges[id] = skillmap.ReviewBadge{
			Due:       !now.Before(nextReview),
			Graduated: rs.Graduated,
		}
	}
	return badges
}

// computeSkillStates maps snapshot mastery data to skillgraph.SkillState values.
func computeSkillStates(snap *store.Snapshot) map[string]skillgraph.SkillState {
	states := make(map[string]skillgraph.SkillState)
	if snap == nil || snap.Data.Mastery == nil {
		return states
	}
	for id, sm := range snap.Data.Mastery.Skills {
		switch sm.State {
		case "mastered":
			states[id] = skillgraph.StateMastered
		case "rusty":
			states[id] = skillgraph.StateRusty
		case "learning":
			if sm.CurrentTier == "prove" {
				states[id] = skillgraph.StateProving
			} else {
				states[id] = skillgraph.StateLearning
			}
		// "new" → skip, same as no entry
		}
	}
	return states
}
