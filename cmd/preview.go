package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/spf13/cobra"
)

var previewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview LLM-generated questions for a skill (no database)",
	Long: `Generate and interactively answer questions for a specific skill.

This is a stateless developer tool — no database, no mastery tracking, no events.
Useful for evaluating question quality and testing new skills.`,
	RunE: runPreview,
}

func init() {
	previewCmd.Flags().String("skill", "", "Skill ID or CommonCoreID (required)")
	previewCmd.Flags().String("tier", "learn", "Difficulty tier: learn or prove")
	previewCmd.Flags().Int("count", 5, "Number of questions to generate")
	_ = previewCmd.MarkFlagRequired("skill")
}

func runPreview(cmd *cobra.Command, args []string) error {
	skillVal, _ := cmd.Flags().GetString("skill")
	tierVal, _ := cmd.Flags().GetString("tier")
	count, _ := cmd.Flags().GetInt("count")

	// Resolve skill.
	skill, err := resolveSkill(skillVal)
	if err != nil {
		return err
	}

	// Parse tier.
	var tier skillgraph.Tier
	switch strings.ToLower(tierVal) {
	case "learn":
		tier = skillgraph.TierLearn
	case "prove":
		tier = skillgraph.TierProve
	default:
		return fmt.Errorf("invalid tier %q: must be learn or prove", tierVal)
	}

	// Create LLM provider (no EventRepo — logging skipped).
	ctx := context.Background()
	provider, err := llm.NewProviderFromEnv(ctx, nil)
	if err != nil {
		return fmt.Errorf("LLM provider: %w", err)
	}

	gen := problemgen.New(provider, problemgen.DefaultConfig())
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Printf("Skill: %s — %s (Grade %d, %s)\n",
		skill.ID, skill.Name, skill.GradeLevel, tierVal)
	fmt.Printf("Generating %d questions...\n\n", count)

	var correct int
	var priorQuestions []string

	for i := 1; i <= count; i++ {
		input := problemgen.GenerateInput{
			Skill:          skill,
			Tier:           tier,
			PriorQuestions: priorQuestions,
		}

		q, err := gen.Generate(ctx, input)
		if err != nil {
			fmt.Printf("Question %d: generation failed: %v\n\n", i, err)
			continue
		}

		priorQuestions = append(priorQuestions, q.Text)

		// Display question.
		fmt.Printf("── Question %d/%d ──\n", i, count)
		fmt.Println(q.Text)
		if q.Format == problemgen.FormatMultipleChoice && len(q.Choices) > 0 {
			for j, c := range q.Choices {
				fmt.Printf("  %d) %s\n", j+1, c)
			}
		}

		// Read answer.
		fmt.Print("\nYour answer: ")
		if !scanner.Scan() {
			fmt.Println("\n(input closed)")
			break
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer == "" {
			fmt.Println("(skipped)\n")
			continue
		}

		// Check answer.
		if problemgen.CheckAnswer(answer, q) {
			correct++
			fmt.Println("\033[32m✓ Correct!\033[0m")
		} else {
			fmt.Printf("\033[31m✗ Wrong.\033[0m Answer: %s\n", q.Answer)
		}

		if q.Explanation != "" {
			fmt.Printf("Explanation: %s\n", q.Explanation)
		}
		fmt.Println()
	}

	// Summary.
	fmt.Printf("── Summary: %d/%d correct ──\n", correct, count)
	return nil
}

// resolveSkill finds a skill by ID first, then by CommonCoreID fallback.
func resolveSkill(val string) (skillgraph.Skill, error) {
	// Try exact ID first.
	if s, err := skillgraph.GetSkill(val); err == nil {
		return s, nil
	}

	// Fall back to CommonCoreID match.
	var matches []skillgraph.Skill
	for _, s := range skillgraph.AllSkills() {
		if s.CommonCoreID == val {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return skillgraph.Skill{}, fmt.Errorf("no skill found for %q", val)
	case 1:
		return matches[0], nil
	default:
		var ids []string
		for _, s := range matches {
			ids = append(ids, s.ID)
		}
		return skillgraph.Skill{}, fmt.Errorf("multiple skills match %q: %s — use --skill with a specific ID",
			val, strings.Join(ids, ", "))
	}
}
