package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show learning statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := resolveDBPath(cmd)
		if err != nil {
			return fmt.Errorf("resolve database path: %w", err)
		}

		s, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		ctx := context.Background()
		snapRepo := s.SnapshotRepo()

		snap, err := snapRepo.Latest(ctx)
		if err != nil {
			return fmt.Errorf("load snapshot: %w", err)
		}

		var snapData *store.SnapshotData
		if snap != nil {
			snapData = &snap.Data
		}

		svc := mastery.NewService(snapData, s.EventRepo())

		// Count skills by state.
		var newCount, learningCount, masteredCount, rustyCount int
		type skillFluency struct {
			name    string
			state   mastery.MasteryState
			fluency float64
			rustyAt *time.Time
		}
		var skills []skillFluency

		for _, skill := range skillgraph.AllSkills() {
			sm := svc.GetMastery(skill.ID)
			switch sm.State {
			case mastery.StateNew:
				newCount++
			case mastery.StateLearning:
				learningCount++
			case mastery.StateMastered:
				masteredCount++
			case mastery.StateRusty:
				rustyCount++
			}
			if sm.State != mastery.StateNew {
				skills = append(skills, skillFluency{
					name:    skill.Name,
					state:   sm.State,
					fluency: sm.FluencyScore(),
					rustyAt: sm.RustyAt,
				})
			}
		}

		// Sort by fluency descending.
		sort.Slice(skills, func(i, j int) bool {
			return skills[i].fluency > skills[j].fluency
		})

		fmt.Println("Mathiz Stats")
		fmt.Println(strings.Repeat("\u2500", 36))
		fmt.Println()
		fmt.Printf("Skills: %d mastered, %d learning, %d rusty, %d new\n",
			masteredCount, learningCount, rustyCount, newCount)
		fmt.Println()

		if len(skills) > 0 {
			fmt.Println("Top Skills by Fluency:")
			top := skills
			if len(top) > 5 {
				top = top[:5]
			}
			for _, sf := range top {
				icon := stateIcon(sf.state)
				fmt.Printf("  %s %-30s %.2f\n", icon, sf.name, sf.fluency)
			}
			fmt.Println()
		}

		// Rusty skills.
		var rustySkills []skillFluency
		for _, sf := range skills {
			if sf.state == mastery.StateRusty {
				rustySkills = append(rustySkills, sf)
			}
		}
		if len(rustySkills) > 0 {
			fmt.Println("Rusty Skills:")
			for _, sf := range rustySkills {
				ageStr := ""
				if sf.rustyAt != nil {
					days := int(time.Since(*sf.rustyAt).Hours() / 24)
					ageStr = fmt.Sprintf(" (rusty %d days ago)", days)
				}
				fmt.Printf("  %s %-30s %.2f%s\n",
					stateIcon(sf.state), sf.name, sf.fluency, ageStr)
			}
			fmt.Println()
		}

		return nil
	},
}

func stateIcon(state mastery.MasteryState) string {
	switch state {
	case mastery.StateMastered:
		return "●"
	case mastery.StateLearning:
		return "◐"
	case mastery.StateRusty:
		return "○"
	default:
		return " "
	}
}
