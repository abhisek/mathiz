package home

import (
	"context"
	"strings"
	"time"

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
)

// HomeScreen is the main home screen of the application.
type HomeScreen struct {
	menu          components.Menu
	menuLabels    []string
	gemCount      int
	masteredCount int
	reviewsDue    int
	mascotVariant MascotVariant
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

	// Compute mastered count, reviews due, and mascot variant from snapshot.
	var masteredCount, reviewsDue int
	var recentMastery bool
	now := time.Now()

	if snap != nil && snap.Data.Mastery != nil {
		for _, sm := range snap.Data.Mastery.Skills {
			if sm.State == "mastered" {
				masteredCount++
				if sm.MasteredAt != nil {
					if t, err := time.Parse(time.RFC3339, *sm.MasteredAt); err == nil {
						if now.Sub(t) < 24*time.Hour {
							recentMastery = true
						}
					}
				}
			}
		}
	}
	if snap != nil && snap.Data.SpacedRep != nil {
		for _, rs := range snap.Data.SpacedRep.Reviews {
			nextReview, err := time.Parse(time.RFC3339, rs.NextReviewDate)
			if err != nil {
				continue
			}
			if !now.Before(nextReview) {
				reviewsDue++
			}
		}
	}

	mascotVariant := MascotIdle
	if reviewsDue >= 3 {
		mascotVariant = MascotAlert
	} else if recentMastery {
		mascotVariant = MascotCelebrating
	}

	skillStates := computeSkillStates(snap)
	reviewBadges := computeReviewBadges(snap)

	menuLabels := []string{"START GAME", "SKILL MAP", "GEM VAULT", "HISTORY", "EXIT GAME"}

	items := []components.MenuItem{
		{Label: menuLabels[0], Action: func() tea.Cmd {
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
		{Label: menuLabels[1], Action: func() tea.Cmd {
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: skillmap.New(skillStates, reviewBadges)}
			}
		}},
		{Label: menuLabels[2], Action: func() tea.Cmd {
			if eventRepo == nil {
				return func() tea.Msg {
					return router.PushScreenMsg{Screen: placeholder.New("Gem Vault")}
				}
			}
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: gemvault.New(eventRepo)}
			}
		}},
		{Label: menuLabels[3], Action: func() tea.Cmd {
			if eventRepo == nil {
				return func() tea.Msg {
					return router.PushScreenMsg{Screen: placeholder.New("History")}
				}
			}
			return func() tea.Msg {
				return router.PushScreenMsg{Screen: history.New(eventRepo)}
			}
		}},
		{Label: menuLabels[4], Action: func() tea.Cmd {
			return tea.Quit
		}},
	}

	return &HomeScreen{
		menu:          components.NewMenu(items),
		menuLabels:    menuLabels,
		gemCount:      gemCount,
		masteredCount: masteredCount,
		reviewsDue:    reviewsDue,
		mascotVariant: mascotVariant,
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

	// All sections share a uniform content width so they line up.
	cw := contentWidth(width)

	var sections []string

	// 1. Title
	sections = append(sections, renderTitle(cw, compact))

	// 2. Mascot (full mode only)
	if !compact {
		sections = append(sections, renderMascotBox(h.mascotVariant, cw))
	}

	// 3. Stats bar (double-bordered, same width)
	sections = append(sections, renderStatsBar(
		h.masteredCount, h.gemCount, h.reviewsDue, cw, compact))

	// 4. Menu (same width box)
	sections = append(sections, renderArcadeMenu(
		h.menuLabels, h.menu.Selected, cw))

	content := strings.Join(sections, "\n\n")

	// Wrap in cabinet frame, centered in the full area
	return renderCabinetFrame(content, width, height)
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
