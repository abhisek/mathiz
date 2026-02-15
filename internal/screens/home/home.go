package home

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	"github.com/abhisek/mathiz/internal/selfupdate"
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
	updateResult  *selfupdate.UpdateResult
	llmMissing    bool
}

var _ screen.Screen = (*HomeScreen)(nil)

// New creates a new HomeScreen.
func New(generator problemgen.Generator, eventRepo store.EventRepo, snapRepo store.SnapshotRepo, diagService *diagnosis.Service, lessonService *lessons.Service, compressor *lessons.Compressor, gemService *gems.Service, updateResult *selfupdate.UpdateResult) *HomeScreen {
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

	llmMissing := generator == nil
	menuLabels := []string{"START GAME", "SKILL MAP", "GEM VAULT", "HISTORY", "EXIT GAME"}

	items := []components.MenuItem{
		{Label: menuLabels[0], Disabled: llmMissing, Action: func() tea.Cmd {
			if generator == nil || eventRepo == nil || snapRepo == nil {
				return nil
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
		updateResult:  updateResult,
		llmMissing:    llmMissing,
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
	// Available vertical space inside the cabinet frame (2 for border).
	available := height - 2
	cw := contentWidth(width)
	compact := width < 100

	// Build disabled set from menu items.
	disabledItems := make(map[int]bool)
	for i, item := range h.menu.Items {
		if item.Disabled {
			disabledItems[i] = true
		}
	}

	// Pre-render candidates and measure heights.
	menuBordered := renderArcadeMenu(h.menuLabels, h.menu.Selected, cw, disabledItems)
	menuCompact := renderArcadeMenuCompact(h.menuLabels, h.menu.Selected, cw, disabledItems)
	statsBar := renderStatsBar(h.masteredCount, h.gemCount, h.reviewsDue, cw, compact)
	titleCompact := renderTitle(cw, true)
	titleFull := renderTitle(cw, false)
	mascot := renderMascotBox(h.mascotVariant, cw)

	hMenu := lipgloss.Height(menuBordered)
	hMenuCompact := lipgloss.Height(menuCompact)
	hStats := lipgloss.Height(statsBar)
	hTitleCompact := lipgloss.Height(titleCompact)
	hTitleFull := lipgloss.Height(titleFull)
	hMascot := lipgloss.Height(mascot)

	// 1. Start with bordered menu (highest priority).
	menu := menuBordered
	used := hMenu

	// 2. Try adding stats bar (sep = 2 for "\n\n", 1 for "\n").
	canStats := used+2+hStats <= available
	if canStats {
		used += 2 + hStats
	}

	// 3. Try adding compact title.
	canTitle := canStats && used+2+hTitleCompact <= available
	if canTitle {
		used += 2 + hTitleCompact
	}

	// If base elements overflow, switch to compact (borderless) menu.
	if used > available {
		menu = menuCompact
		used = hMenuCompact

		canStats = used+1+hStats <= available
		if canStats {
			used += 1 + hStats
		}
		canTitle = canStats && used+1+hTitleCompact <= available
		if canTitle {
			used += 1 + hTitleCompact
		}
	}

	// Determine separator based on how tight space is.
	tight := !canTitle || (available-used) < 4
	sep := "\n\n"
	if tight {
		sep = "\n"
		// Recalculate used with single-line separators.
		used = lipgloss.Height(menu)
		if canStats {
			used += 1 + hStats
		}
		if canTitle {
			used += 1 + hTitleCompact
		}
	}

	// 4. Upgrade: try full ASCII title if space allows.
	useFullTitle := false
	if canTitle && !compact {
		extra := hTitleFull - hTitleCompact
		if used+extra <= available {
			useFullTitle = true
			used += extra
		}
	}

	// 5. Upgrade: try mascot if space allows.
	canMascot := false
	if !compact && used+2+hMascot <= available {
		canMascot = true
	}

	// 6. LLM banner (only when LLM is missing and space allows).
	var llmBanner string
	canLLMBanner := false
	if h.llmMissing {
		llmBanner = renderLLMBanner(cw)
		hBanner := lipgloss.Height(llmBanner)
		if used+1+hBanner <= available {
			canLLMBanner = true
			used += 1 + hBanner
		}
	}

	// 7. Lowest priority: update note (only if update available and space allows).
	var updateNote string
	canUpdate := false
	if h.updateResult != nil && h.updateResult.UpdateAvailable {
		updateNote = renderUpdateNote(h.updateResult.LatestVersion, cw)
		hUpdate := lipgloss.Height(updateNote)
		if used+1+hUpdate <= available {
			canUpdate = true
		}
	}

	// Assemble sections in display order: title, mascot, stats, banner, menu, update.
	var sections []string

	if canTitle {
		if useFullTitle {
			sections = append(sections, titleFull)
		} else {
			sections = append(sections, titleCompact)
		}
	}

	if canMascot {
		sections = append(sections, mascot)
	}

	if canStats {
		sections = append(sections, statsBar)
	}

	if canLLMBanner {
		sections = append(sections, llmBanner)
	}

	sections = append(sections, menu)

	if canUpdate {
		sections = append(sections, updateNote)
	}

	content := strings.Join(sections, sep)
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
