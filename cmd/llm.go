package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/abhisek/mathiz/internal/store"
	"github.com/spf13/cobra"
)

var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "Inspect LLM request/response events",
}

var llmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent LLM events",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		purpose, _ := cmd.Flags().GetString("purpose")

		dbPath, err := store.DefaultDBPath()
		if err != nil {
			return fmt.Errorf("resolve database path: %w", err)
		}

		s, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		ctx := context.Background()
		events, err := s.EventRepo().QueryLLMEvents(ctx, store.QueryOpts{Limit: limit})
		if err != nil {
			return fmt.Errorf("query events: %w", err)
		}

		if len(events) == 0 {
			fmt.Println("No LLM events found.")
			return nil
		}

		// Header.
		fmt.Printf("%-5s  %-19s  %-14s  %-28s  %-6s  %-6s  %-7s  %s\n",
			"ID", "Timestamp", "Purpose", "Model", "In", "Out", "Ms", "OK")
		fmt.Println(strings.Repeat("\u2500", 100))

		for _, e := range events {
			if purpose != "" && e.Purpose != purpose {
				continue
			}
			ok := "\u2705"
			if !e.Success {
				ok = "\u274c"
			}
			model := e.Model
			if len(model) > 28 {
				model = model[:28]
			}
			fmt.Printf("%-5d  %-19s  %-14s  %-28s  %-6d  %-6d  %-7d  %s\n",
				e.ID,
				e.Timestamp.Local().Format("2006-01-02 15:04:05"),
				e.Purpose,
				model,
				e.InputTokens,
				e.OutputTokens,
				e.LatencyMs,
				ok,
			)
		}
		return nil
	},
}

var llmViewCmd = &cobra.Command{
	Use:   "view <id>",
	Short: "View full request/response for an LLM event",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id int
		if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
			return fmt.Errorf("invalid ID %q: %w", args[0], err)
		}

		dbPath, err := store.DefaultDBPath()
		if err != nil {
			return fmt.Errorf("resolve database path: %w", err)
		}

		s, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		ctx := context.Background()
		e, err := s.EventRepo().GetLLMEvent(ctx, id)
		if err != nil {
			return fmt.Errorf("get event: %w", err)
		}
		if e == nil {
			return fmt.Errorf("event %d not found", id)
		}

		sep := strings.Repeat("\u2500", 60)

		fmt.Printf("ID:        %d\n", e.ID)
		fmt.Printf("Time:      %s\n", e.Timestamp.Local().Format("2006-01-02 15:04:05"))
		fmt.Printf("Provider:  %s\n", e.Provider)
		fmt.Printf("Model:     %s\n", e.Model)
		fmt.Printf("Purpose:   %s\n", e.Purpose)
		fmt.Printf("Tokens:    %d in / %d out\n", e.InputTokens, e.OutputTokens)
		fmt.Printf("Latency:   %dms\n", e.LatencyMs)
		fmt.Printf("Success:   %v\n", e.Success)
		if e.ErrorMessage != "" {
			fmt.Printf("Error:     %s\n", e.ErrorMessage)
		}

		fmt.Println()
		fmt.Println(sep)
		fmt.Println("REQUEST")
		fmt.Println(sep)
		if e.RequestBody != "" {
			fmt.Println(e.RequestBody)
		} else {
			fmt.Println("(not captured)")
		}

		fmt.Println(sep)
		fmt.Println("RESPONSE")
		fmt.Println(sep)
		if e.ResponseBody != "" {
			fmt.Println(e.ResponseBody)
		} else {
			fmt.Println("(not captured)")
		}

		return nil
	},
}

func init() {
	llmListCmd.Flags().IntP("limit", "n", 20, "Number of events to show")
	llmListCmd.Flags().StringP("purpose", "p", "", "Filter by purpose (e.g. question-gen, lesson, diagnosis)")

	llmCmd.AddCommand(llmListCmd)
	llmCmd.AddCommand(llmViewCmd)
}
