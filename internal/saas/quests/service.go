// Package quests implements Parent Quests (specs/15-quests.md): parent-
// authored one-off question sets that appear on the child's treasure map
// and play through the standard expedition machinery.
//
// Quests are control plane — family-scoped like invites, NOT event-sourced.
// The service owns data invariants (targeting, status transitions, question
// validation, generation idempotency); permission decisions live in
// internal/saas/authz, and callers are expected to authorize first.
package quests

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/childprofile"
	"github.com/abhisek/mathiz/ent/quest"
	"github.com/abhisek/mathiz/ent/questprogress"
	"github.com/abhisek/mathiz/ent/questquestion"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/saas/credits"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrBadName     = errors.New("quest name must not be empty")
	ErrBadSkill    = errors.New("unknown skill tag")
	ErrBadChild    = errors.New("target child is not in this family")
	ErrBadStatus   = errors.New("status must be draft, active, or archived")
	ErrBadQuestion = errors.New("invalid question")
	ErrNoQuestions = errors.New("a quest needs at least one question before it can be published")
	ErrBadBrief    = errors.New("generation brief must not be empty")
	ErrBadCount    = errors.New("generation count must be between 1 and 20")
	ErrBadKey      = errors.New("generation clientKey must not be empty")
	ErrNoProvider  = errors.New("AI generation is not configured on this server")
	ErrGeneration  = errors.New("could not generate valid questions, try again")
)

// Quest statuses.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// MaxGenerateCount caps one generation request.
const MaxGenerateCount = 20

// GenerateCostDivisor: generation debits ceil(count/5) credits.
const GenerateCostDivisor = 5

// ProviderFactory builds an LLM provider for quest generation. Nil (or an
// error) means AI generation is unavailable; manual authoring still works.
type ProviderFactory func(ctx context.Context) (llm.Provider, error)

// Service implements quest operations on top of the ent client.
// credits may be nil (billing off → generation is free).
type Service struct {
	client   *ent.Client
	credits  *credits.Service
	provider ProviderFactory
}

func New(client *ent.Client, creditsSvc *credits.Service, provider ProviderFactory) *Service {
	return &Service{client: client, credits: creditsSvc, provider: provider}
}

// ---- Quest CRUD ----

// QuestInput carries the mutable quest fields.
type QuestInput struct {
	Name     string
	Emoji    string
	SkillID  string // "" = untagged
	ChildUID string // "" = all children in the space
}

// Create adds a draft quest to a family space.
func (s *Service) Create(ctx context.Context, spaceUID string, in QuestInput) (*ent.Quest, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, ErrBadName
	}
	if err := s.validateTarget(ctx, spaceUID, in.SkillID, in.ChildUID); err != nil {
		return nil, err
	}
	return s.client.Quest.Create().
		SetUID(uuid.NewString()).
		SetFamilySpaceID(spaceUID).
		SetName(strings.TrimSpace(in.Name)).
		SetEmoji(in.Emoji).
		SetSkillID(in.SkillID).
		SetChildUID(in.ChildUID).
		SetStatus(StatusDraft).
		Save(ctx)
}

// Quest returns a quest by UID. Also satisfies authz.QuestDirectory.
func (s *Service) Quest(ctx context.Context, questUID string) (*ent.Quest, error) {
	q, err := s.client.Quest.Query().
		Where(quest.UID(questUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return q, err
}

// BySpace lists all quests in a space, newest first.
func (s *Service) BySpace(ctx context.Context, spaceUID string) ([]*ent.Quest, error) {
	return s.client.Quest.Query().
		Where(quest.FamilySpaceID(spaceUID)).
		Order(ent.Desc(quest.FieldCreatedAt)).
		All(ctx)
}

// UpdateOpts carries optional quest updates. Nil fields are left unchanged.
type UpdateOpts struct {
	Name     *string
	Emoji    *string
	SkillID  *string
	ChildUID *string
	Status   *string
}

// Update applies rename / retarget / status changes. Activating (directly or
// via Publish) requires at least one question — nothing empty reaches a kid.
func (s *Service) Update(ctx context.Context, questUID string, opts UpdateOpts) (*ent.Quest, error) {
	q, err := s.Quest(ctx, questUID)
	if err != nil {
		return nil, err
	}
	upd := q.Update()
	if opts.Name != nil {
		if strings.TrimSpace(*opts.Name) == "" {
			return nil, ErrBadName
		}
		upd.SetName(strings.TrimSpace(*opts.Name))
	}
	if opts.Emoji != nil {
		upd.SetEmoji(*opts.Emoji)
	}
	skillID := q.SkillID
	if opts.SkillID != nil {
		skillID = *opts.SkillID
	}
	childUID := q.ChildUID
	if opts.ChildUID != nil {
		childUID = *opts.ChildUID
	}
	if err := s.validateTarget(ctx, q.FamilySpaceID, skillID, childUID); err != nil {
		return nil, err
	}
	upd.SetSkillID(skillID).SetChildUID(childUID)
	if opts.Status != nil {
		switch *opts.Status {
		case StatusDraft, StatusActive, StatusArchived:
		default:
			return nil, ErrBadStatus
		}
		if *opts.Status == StatusActive && q.Status != StatusActive {
			if err := s.requireQuestions(ctx, questUID); err != nil {
				return nil, err
			}
		}
		upd.SetStatus(*opts.Status)
	}
	return upd.Save(ctx)
}

// Publish flips a quest draft → active. Requires at least one question.
func (s *Service) Publish(ctx context.Context, questUID string) (*ent.Quest, error) {
	active := StatusActive
	return s.Update(ctx, questUID, UpdateOpts{Status: &active})
}

// Delete removes a quest with its questions and progress rows.
func (s *Service) Delete(ctx context.Context, questUID string) error {
	q, err := s.Quest(ctx, questUID)
	if err != nil {
		return err
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.QuestProgress.Delete().Where(questprogress.QuestUID(q.UID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete quest progress: %w", err)
	}
	if _, err := tx.QuestQuestion.Delete().Where(questquestion.QuestUID(q.UID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete quest questions: %w", err)
	}
	if _, err := tx.Quest.Delete().Where(quest.UID(q.UID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete quest: %w", err)
	}
	return tx.Commit()
}

// validateTarget checks the skill tag and target child against the space.
func (s *Service) validateTarget(ctx context.Context, spaceUID, skillID, childUID string) error {
	if skillID != "" {
		if _, err := skillgraph.GetSkill(skillID); err != nil {
			return ErrBadSkill
		}
	}
	if childUID != "" {
		ok, err := s.client.ChildProfile.Query().
			Where(childprofile.UID(childUID), childprofile.FamilySpaceID(spaceUID)).
			Exist(ctx)
		if err != nil {
			return fmt.Errorf("check target child: %w", err)
		}
		if !ok {
			return ErrBadChild
		}
	}
	return nil
}

func (s *Service) requireQuestions(ctx context.Context, questUID string) error {
	n, err := s.client.QuestQuestion.Query().
		Where(questquestion.QuestUID(questUID)).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("count questions: %w", err)
	}
	if n == 0 {
		return ErrNoQuestions
	}
	return nil
}

// CountQuestions returns how many questions a quest holds.
func (s *Service) CountQuestions(ctx context.Context, questUID string) (int, error) {
	return s.client.QuestQuestion.Query().
		Where(questquestion.QuestUID(questUID)).
		Count(ctx)
}

// ---- Question authoring ----

// QuestionInput is one authored question. It fully replaces the question on
// update (no partial field patching — the dashboard edits the whole card).
type QuestionInput struct {
	Text        string
	Answer      string
	AnswerType  string // integer | decimal | fraction | text
	Format      string // numeric | multiple_choice
	Choices     []string
	Hint        string
	Explanation string
}

// QuestionResult is a saved question plus an optional non-blocking warning:
// when the pure-Go math recompute disagrees with the given answer, the save
// succeeds but the parent is told (typo guard — a wrong answer key poisons
// kid trust).
type QuestionResult struct {
	Question *ent.QuestQuestion
	Warning  string
}

// AddQuestion appends a question to the quest.
func (s *Service) AddQuestion(ctx context.Context, questUID string, in QuestionInput) (*QuestionResult, error) {
	q, err := s.Quest(ctx, questUID)
	if err != nil {
		return nil, err
	}
	warning, err := validateQuestion(&in)
	if err != nil {
		return nil, err
	}
	pos, err := s.nextPosition(ctx, q.UID)
	if err != nil {
		return nil, err
	}
	qq, err := s.createQuestion(ctx, s.client, q.UID, pos, "", &in)
	if err != nil {
		return nil, err
	}
	return &QuestionResult{Question: qq, Warning: warning}, nil
}

// UpdateQuestion replaces a question's content (position is kept).
func (s *Service) UpdateQuestion(ctx context.Context, questUID, questionUID string, in QuestionInput) (*QuestionResult, error) {
	qq, err := s.question(ctx, questUID, questionUID)
	if err != nil {
		return nil, err
	}
	warning, err := validateQuestion(&in)
	if err != nil {
		return nil, err
	}
	updated, err := qq.Update().
		SetText(in.Text).
		SetAnswer(in.Answer).
		SetAnswerType(in.AnswerType).
		SetFormat(in.Format).
		SetChoices(in.Choices).
		SetHint(in.Hint).
		SetExplanation(in.Explanation).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return &QuestionResult{Question: updated, Warning: warning}, nil
}

// DeleteQuestion removes a question and its progress rows.
func (s *Service) DeleteQuestion(ctx context.Context, questUID, questionUID string) error {
	qq, err := s.question(ctx, questUID, questionUID)
	if err != nil {
		return err
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.QuestProgress.Delete().Where(questprogress.QuestionUID(qq.UID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete question progress: %w", err)
	}
	if _, err := tx.QuestQuestion.Delete().Where(questquestion.UID(qq.UID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete question: %w", err)
	}
	return tx.Commit()
}

// Questions lists a quest's questions in authored order.
func (s *Service) Questions(ctx context.Context, questUID string) ([]*ent.QuestQuestion, error) {
	return s.client.QuestQuestion.Query().
		Where(questquestion.QuestUID(questUID)).
		Order(ent.Asc(questquestion.FieldPosition), ent.Asc(questquestion.FieldCreatedAt)).
		All(ctx)
}

// question fetches a question, requiring it to belong to the quest — a
// question UID from another quest is treated as missing, not leaked.
func (s *Service) question(ctx context.Context, questUID, questionUID string) (*ent.QuestQuestion, error) {
	qq, err := s.client.QuestQuestion.Query().
		Where(questquestion.UID(questionUID), questquestion.QuestUID(questUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return qq, err
}

func (s *Service) nextPosition(ctx context.Context, questUID string) (int, error) {
	last, err := s.client.QuestQuestion.Query().
		Where(questquestion.QuestUID(questUID)).
		Order(ent.Desc(questquestion.FieldPosition)).
		First(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return last.Position + 1, nil
}

func (s *Service) createQuestion(ctx context.Context, client *ent.Client, questUID string, pos int, clientKey string, in *QuestionInput) (*ent.QuestQuestion, error) {
	return client.QuestQuestion.Create().
		SetUID(uuid.NewString()).
		SetQuestUID(questUID).
		SetPosition(pos).
		SetText(in.Text).
		SetAnswer(in.Answer).
		SetAnswerType(in.AnswerType).
		SetFormat(in.Format).
		SetChoices(in.Choices).
		SetHint(in.Hint).
		SetExplanation(in.Explanation).
		SetClientKey(clientKey).
		Save(ctx)
}

// validateQuestion hard-validates the input and returns a soft warning when
// the math recompute disagrees with the given answer.
func validateQuestion(in *QuestionInput) (warning string, err error) {
	in.Text = strings.TrimSpace(in.Text)
	in.Answer = strings.TrimSpace(in.Answer)
	if in.Text == "" {
		return "", fmt.Errorf("%w: question text is empty", ErrBadQuestion)
	}
	if in.Answer == "" {
		return "", fmt.Errorf("%w: answer is empty", ErrBadQuestion)
	}
	switch problemgen.AnswerFormat(in.Format) {
	case problemgen.FormatNumeric:
		if len(in.Choices) > 0 {
			return "", fmt.Errorf("%w: numeric questions must not have choices", ErrBadQuestion)
		}
	case problemgen.FormatMultipleChoice:
		if len(in.Choices) < 2 {
			return "", fmt.Errorf("%w: multiple choice needs at least 2 choices", ErrBadQuestion)
		}
		found := false
		for _, c := range in.Choices {
			if strings.EqualFold(strings.TrimSpace(c), in.Answer) {
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("%w: the answer must be one of the choices", ErrBadQuestion)
		}
	default:
		return "", fmt.Errorf("%w: format must be numeric or multiple_choice", ErrBadQuestion)
	}
	switch problemgen.AnswerType(in.AnswerType) {
	case problemgen.AnswerTypeInteger, problemgen.AnswerTypeDecimal, problemgen.AnswerTypeFraction:
	case problemgen.AnswerTypeText:
		if problemgen.AnswerFormat(in.Format) != problemgen.FormatMultipleChoice {
			return "", fmt.Errorf("%w: text answers must use multiple_choice format", ErrBadQuestion)
		}
	default:
		return "", fmt.Errorf("%w: answer type must be integer, decimal, fraction, or text", ErrBadQuestion)
	}

	// Soft check: recompute the answer from the question text where the
	// pure-Go math checker can (problemgen's MathCheckValidator). A
	// mismatch is a warning, never a block — word problems and conceptual
	// questions are not computable and pass silently.
	check := &problemgen.MathCheckValidator{}
	if verr := check.Validate(questionForCheck(in), problemgen.GenerateInput{}); verr != nil {
		return fmt.Sprintf("the math checker computed a different answer: %s — double-check for a typo", verr.Message), nil
	}
	return "", nil
}

func questionForCheck(in *QuestionInput) *problemgen.Question {
	return &problemgen.Question{
		Text:        in.Text,
		Format:      problemgen.AnswerFormat(in.Format),
		Answer:      in.Answer,
		AnswerType:  problemgen.AnswerType(in.AnswerType),
		Choices:     in.Choices,
		Hint:        in.Hint,
		Difficulty:  3,
		Explanation: in.Explanation,
	}
}

// ---- Play (game.QuestSource) ----

// PlayableQuest returns the quest and its not-yet-correctly-answered
// questions for a child. Inactive, cross-family, or mis-targeted quests all
// come back as ErrQuestUnavailable — don't confirm what the child can't see.
func (s *Service) PlayableQuest(ctx context.Context, childUID, questUID string) (*game.QuestPlay, error) {
	q, err := s.playableFor(ctx, childUID, questUID)
	if err != nil {
		return nil, err
	}
	questions, err := s.Questions(ctx, q.UID)
	if err != nil {
		return nil, err
	}
	correct, err := s.correctSet(ctx, q.UID, childUID)
	if err != nil {
		return nil, err
	}
	play := &game.QuestPlay{
		QuestUID: q.UID,
		Name:     q.Name,
		Emoji:    q.Emoji,
		SkillID:  q.SkillID,
	}
	for _, qq := range questions {
		if correct[qq.UID] {
			continue
		}
		play.Questions = append(play.Questions, game.QuestPlayQuestion{
			UID:         qq.UID,
			Text:        qq.Text,
			Answer:      qq.Answer,
			AnswerType:  qq.AnswerType,
			Format:      qq.Format,
			Choices:     qq.Choices,
			Hint:        qq.Hint,
			Explanation: qq.Explanation,
		})
	}
	if len(play.Questions) == 0 {
		return nil, game.ErrQuestDone
	}
	return play, nil
}

// RecordAnswer upserts the progress row for one graded answer and returns
// how many questions remain unanswered-correctly for the child. Once
// correct, a question stays correct (a later wrong retry can't undo it).
func (s *Service) RecordAnswer(ctx context.Context, questUID, childUID, questionUID string, correct bool) (int, error) {
	existing, err := s.client.QuestProgress.Query().
		Where(
			questprogress.QuestUID(questUID),
			questprogress.ChildUID(childUID),
			questprogress.QuestionUID(questionUID),
		).
		Only(ctx)
	switch {
	case err == nil:
		if correct && !existing.Correct {
			if err := existing.Update().SetCorrect(true).Exec(ctx); err != nil {
				return 0, fmt.Errorf("update quest progress: %w", err)
			}
		}
	case ent.IsNotFound(err):
		err := s.client.QuestProgress.Create().
			SetUID(uuid.NewString()).
			SetQuestUID(questUID).
			SetChildUID(childUID).
			SetQuestionUID(questionUID).
			SetCorrect(correct).
			Exec(ctx)
		if err != nil && !ent.IsConstraintError(err) {
			return 0, fmt.Errorf("create quest progress: %w", err)
		}
		if ent.IsConstraintError(err) && correct {
			// Lost a race with a concurrent insert: converge on correct=true.
			if _, err := s.client.QuestProgress.Update().
				Where(
					questprogress.QuestUID(questUID),
					questprogress.ChildUID(childUID),
					questprogress.QuestionUID(questionUID),
				).
				SetCorrect(true).
				Save(ctx); err != nil {
				return 0, fmt.Errorf("converge quest progress: %w", err)
			}
		}
	default:
		return 0, fmt.Errorf("query quest progress: %w", err)
	}
	return s.remaining(ctx, questUID, childUID)
}

// ActiveQuests lists the active quests targeted at a child, with progress.
// Read-only: it backs the map render.
func (s *Service) ActiveQuests(ctx context.Context, childUID string) ([]game.QuestMapItem, error) {
	child, err := s.child(ctx, childUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil // no profile → no quests (fresh/test children)
		}
		return nil, err
	}
	quests, err := s.client.Quest.Query().
		Where(
			quest.FamilySpaceID(child.FamilySpaceID),
			quest.Status(StatusActive),
			quest.ChildUIDIn("", childUID),
		).
		Order(ent.Asc(quest.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]game.QuestMapItem, 0, len(quests))
	for _, q := range quests {
		total, err := s.CountQuestions(ctx, q.UID)
		if err != nil {
			return nil, err
		}
		correct, err := s.correctCount(ctx, q.UID, childUID)
		if err != nil {
			return nil, err
		}
		items = append(items, game.QuestMapItem{
			ID:      q.UID,
			Name:    q.Name,
			Emoji:   q.Emoji,
			Total:   total,
			Correct: correct,
			Done:    total > 0 && correct >= total,
		})
	}
	return items, nil
}

// ProgressFor reports (correct, total) for one child on one quest — used by
// the parent dashboard detail view.
func (s *Service) ProgressFor(ctx context.Context, questUID, childUID string) (correct, total int, err error) {
	total, err = s.CountQuestions(ctx, questUID)
	if err != nil {
		return 0, 0, err
	}
	correct, err = s.correctCount(ctx, questUID, childUID)
	if err != nil {
		return 0, 0, err
	}
	return correct, total, nil
}

// playableFor resolves child + quest and enforces play targeting: active
// status, same family, and child target "" or this child.
func (s *Service) playableFor(ctx context.Context, childUID, questUID string) (*ent.Quest, error) {
	child, err := s.child(ctx, childUID)
	if err != nil {
		return nil, game.ErrQuestUnavailable
	}
	q, err := s.Quest(ctx, questUID)
	if err != nil {
		return nil, game.ErrQuestUnavailable
	}
	if q.FamilySpaceID != child.FamilySpaceID ||
		q.Status != StatusActive ||
		(q.ChildUID != "" && q.ChildUID != childUID) {
		return nil, game.ErrQuestUnavailable
	}
	return q, nil
}

func (s *Service) child(ctx context.Context, childUID string) (*ent.ChildProfile, error) {
	c, err := s.client.ChildProfile.Query().
		Where(childprofile.UID(childUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Service) correctSet(ctx context.Context, questUID, childUID string) (map[string]bool, error) {
	rows, err := s.client.QuestProgress.Query().
		Where(
			questprogress.QuestUID(questUID),
			questprogress.ChildUID(childUID),
			questprogress.Correct(true),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(rows))
	for _, r := range rows {
		set[r.QuestionUID] = true
	}
	return set, nil
}

func (s *Service) correctCount(ctx context.Context, questUID, childUID string) (int, error) {
	return s.client.QuestProgress.Query().
		Where(
			questprogress.QuestUID(questUID),
			questprogress.ChildUID(childUID),
			questprogress.Correct(true),
		).
		Count(ctx)
}

func (s *Service) remaining(ctx context.Context, questUID, childUID string) (int, error) {
	total, err := s.CountQuestions(ctx, questUID)
	if err != nil {
		return 0, err
	}
	correct, err := s.correctCount(ctx, questUID, childUID)
	if err != nil {
		return 0, err
	}
	if correct > total {
		return 0, nil
	}
	return total - correct, nil
}
