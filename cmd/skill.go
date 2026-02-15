package cmd

import (
	"fmt"
	"strings"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Browse the skill graph",
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills (optionally filtered by strand or grade)",
	RunE: func(cmd *cobra.Command, args []string) error {
		strand, _ := cmd.Flags().GetString("strand")
		grade, _ := cmd.Flags().GetInt("grade")

		var skills []skillgraph.Skill

		switch {
		case strand != "" && grade != 0:
			return fmt.Errorf("use --strand or --grade, not both")
		case strand != "":
			skills = skillgraph.ByStrand(skillgraph.Strand(strand))
			if len(skills) == 0 {
				return fmt.Errorf("no skills found for strand %q", strand)
			}
		case grade != 0:
			skills = skillgraph.ByGrade(grade)
			if len(skills) == 0 {
				return fmt.Errorf("no skills found for grade %d", grade)
			}
		default:
			skills = skillgraph.AllSkills()
		}

		// Header.
		fmt.Printf("%-30s  %-40s  %5s  %-24s  %s\n",
			"ID", "Name", "Grade", "Strand", "CCSS")
		fmt.Println(strings.Repeat("\u2500", 115))

		for _, s := range skills {
			name := s.Name
			if len(name) > 40 {
				name = name[:37] + "..."
			}
			fmt.Printf("%-30s  %-40s  %5d  %-24s  %s\n",
				s.ID, name, s.GradeLevel,
				skillgraph.StrandDisplayName(s.Strand), s.CommonCoreID)
		}

		fmt.Printf("\n%d skills\n", len(skills))
		return nil
	},
}

func init() {
	skillListCmd.Flags().String("strand", "", "Filter by strand (e.g. addition-and-subtraction)")
	skillListCmd.Flags().Int("grade", 0, "Filter by grade level (3, 4, or 5)")

	skillCmd.AddCommand(skillListCmd)
}
