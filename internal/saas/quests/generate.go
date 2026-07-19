package quests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/questquestion"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

// AI question generation (specs/15-quests.md §3): the parent writes a brief,
// the server generates through problemgen-style validation, and the result
// is saved as draft questions FOR REVIEW — nothing reaches a kid unapproved.
//
// Idempotency is structural twice over: generated questions carry the
// client key (a retried click returns the already-saved batch, no LLM call),
// and the credit debit's source is "questgen:<questUID>:<clientKey>" so a
// replay can never double-debit.

// genSystemPrompt mirrors problemgen's rules for a parent-briefed batch.
const genSystemPrompt = `You are a math tutor writing a one-off practice quest for a child, based on a parent's brief.

Rules:
- Generate exactly the requested number of practice problems matching the parent's brief.
- Use plain ASCII text for all math. No LaTeX, no Unicode symbols. Use / for fractions, * for multiplication, and standard operators.
- Each question must be clear, self-contained, and age-appropriate for the given grade range.
- Each answer must be correct and in simplest form (reduce fractions, no trailing zeros on decimals, no leading zeros on integers).
- Each explanation shows the solution step by step, suitable for a child.
- Choose "numeric" format for computation problems (the student types the answer).
- Choose "multiple_choice" format for conceptual, comparison, or identification problems (the student picks from 4 options).
- For multiple choice, provide exactly 4 options where exactly one is correct. Distractors should reflect common mistakes, not random values.
- Options are shuffled before display, so hints and explanations must refer to options by their content, never by their position (no "the first option" or "option A").
- Use answer_type "text" only for conceptual reasoning questions, always with "multiple_choice" format.
- Include a short helpful hint for every question.
- Vary the questions: no two questions in the batch may be near-duplicates.`

// questBatchSchema wraps problemgen's per-question schema in an array.
var questBatchSchema = &llm.Schema{
	Name:        "quest-questions",
	Description: "A batch of math practice questions for a parent-authored quest",
	Definition: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type":  "array",
				"items": problemgen.QuestionSchema.Definition,
			},
		},
		"required":             []any{"questions"},
		"additionalProperties": false,
	},
}

// genQuestion matches problemgen's LLM output shape.
type genQuestion struct {
	QuestionText string   `json:"question_text"`
	Format       string   `json:"format"`
	Answer       string   `json:"answer"`
	AnswerType   string   `json:"answer_type"`
	Choices      []string `json:"choices"`
	Hint         string   `json:"hint"`
	Difficulty   int      `json:"difficulty"`
	Explanation  string   `json:"explanation"`
}

// genValidators is the problemgen validation chain generated questions must
// pass. Failing questions are dropped, not retried — the parent reviews the
// batch anyway.
var genValidators = []problemgen.Validator{
	&problemgen.StructuralValidator{},
	&problemgen.AnswerFormatValidator{},
	&problemgen.MathCheckValidator{},
}

// GenerateResult is a saved (or replayed) generation batch.
type GenerateResult struct {
	Questions []*ent.QuestQuestion
	// Replayed means this clientKey was already generated: the existing
	// batch is returned and nothing was debited or regenerated.
	Replayed bool
}

// Generate produces count draft questions from the parent's brief, validates
// them, debits ceil(count/5) credits (AFTER successful generation — a failed
// LLM call costs nothing; nil credits service = billing off = free), and
// saves them onto the quest.
func (s *Service) Generate(ctx context.Context, questUID, brief string, count int, clientKey string) (*GenerateResult, error) {
	q, err := s.Quest(ctx, questUID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(brief) == "" {
		return nil, ErrBadBrief
	}
	if count < 1 || count > MaxGenerateCount {
		return nil, ErrBadCount
	}
	if strings.TrimSpace(clientKey) == "" {
		return nil, ErrBadKey
	}
	// Retry of the same click: return the already-saved batch (idempotent
	// even if the quest was published in between).
	existing, err := s.client.QuestQuestion.Query().
		Where(questquestion.QuestUID(q.UID), questquestion.ClientKey(clientKey)).
		Order(ent.Asc(questquestion.FieldPosition)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return &GenerateResult{Questions: existing, Replayed: true}, nil
	}

	// Fresh generation lands on drafts only: publishing is the approval
	// gate, so adding unreviewed AI questions to an active quest is refused.
	if q.Status != StatusDraft {
		return nil, fmt.Errorf("%w: generate into draft quests only (set the quest back to draft first)", ErrBadStatus)
	}

	if s.provider == nil {
		return nil, ErrNoProvider
	}
	provider, err := s.provider(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoProvider, err)
	}

	resp, err := provider.Generate(llm.WithPurpose(ctx, "quest-gen"), llm.Request{
		System: genSystemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: buildGenerateMessage(q, brief, count)},
		},
		Schema:      questBatchSchema,
		MaxTokens:   512 * count,
		Temperature: 0.7,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeneration, err)
	}

	var raw struct {
		Questions []genQuestion `json:"questions"`
	}
	if err := json.Unmarshal(resp.Content, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGeneration, err)
	}

	valid := make([]*problemgen.Question, 0, len(raw.Questions))
	for _, g := range raw.Questions {
		pq := &problemgen.Question{
			Text:        g.QuestionText,
			Format:      problemgen.AnswerFormat(g.Format),
			Answer:      g.Answer,
			AnswerType:  problemgen.AnswerType(g.AnswerType),
			Choices:     g.Choices,
			Hint:        g.Hint,
			Difficulty:  g.Difficulty,
			Explanation: g.Explanation,
			SkillID:     q.SkillID,
		}
		if !passesValidation(pq) {
			continue
		}
		// Mandatory for AI-generated questions (LLMs put the right answer
		// first); parent-authored questions keep their authored order.
		pq.ShuffleChoices()
		valid = append(valid, pq)
		if len(valid) == count {
			break
		}
	}
	if len(valid) == 0 {
		return nil, ErrGeneration
	}

	// Debit only now, after successful generation + validation. The unique
	// ledger source makes a concurrent duplicate request a no-op debit.
	if s.credits != nil {
		amount := (count + GenerateCostDivisor - 1) / GenerateCostDivisor
		source := "questgen:" + q.UID + ":" + clientKey
		if err := s.credits.Debit(ctx, q.FamilySpaceID, amount, source); err != nil {
			return nil, err // credits.ErrInsufficient → API 402 out_of_credits
		}
	}

	pos, err := s.nextPosition(ctx, q.UID)
	if err != nil {
		return nil, err
	}
	saved := make([]*ent.QuestQuestion, 0, len(valid))
	for i, pq := range valid {
		in := &QuestionInput{
			Text:        pq.Text,
			Answer:      pq.Answer,
			AnswerType:  string(pq.AnswerType),
			Format:      string(pq.Format),
			Choices:     pq.Choices,
			Hint:        pq.Hint,
			Explanation: pq.Explanation,
		}
		qq, err := s.createQuestion(ctx, s.client, q.UID, pos+i, clientKey, in)
		if err != nil {
			return nil, fmt.Errorf("save generated question: %w", err)
		}
		saved = append(saved, qq)
	}
	return &GenerateResult{Questions: saved}, nil
}

func passesValidation(pq *problemgen.Question) bool {
	for _, v := range genValidators {
		if verr := v.Validate(pq, problemgen.GenerateInput{}); verr != nil {
			return false
		}
	}
	return true
}

// buildGenerateMessage assembles the user prompt: the parent's brief plus
// skill context when the quest is tagged.
func buildGenerateMessage(q *ent.Quest, brief string, count int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Number of questions: %d\n", count)
	fmt.Fprintf(&b, "Parent's brief: %s\n", strings.TrimSpace(brief))
	if q.SkillID != "" {
		if skill, err := skillgraph.GetSkill(q.SkillID); err == nil {
			fmt.Fprintf(&b, "\nTarget skill: %s\n", skill.Name)
			fmt.Fprintf(&b, "Skill description: %s\n", skill.Description)
			fmt.Fprintf(&b, "Grade: %d\n", skill.GradeLevel)
			if len(skill.Keywords) > 0 {
				fmt.Fprintf(&b, "Keywords: %s\n", strings.Join(skill.Keywords, ", "))
			}
		}
	}
	return b.String()
}
