package problemgen

// StructuralValidator checks that required fields are present, within
// length limits, and have valid enum values.
type StructuralValidator struct{}

func (v *StructuralValidator) Name() string { return "structural" }

func (v *StructuralValidator) Validate(q *Question, _ GenerateInput) *ValidationError {
	if q.Text == "" {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "question_text is empty",
			Retryable: true,
		}
	}
	if len(q.Text) > 500 {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "question_text exceeds 500 characters",
			Retryable: true,
		}
	}
	if q.Explanation == "" {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "explanation is empty",
			Retryable: true,
		}
	}
	if len(q.Explanation) > 1000 {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "explanation exceeds 1000 characters",
			Retryable: true,
		}
	}
	if q.Difficulty < 1 || q.Difficulty > 5 {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "difficulty must be between 1 and 5",
			Retryable: true,
		}
	}
	if q.Format != FormatNumeric && q.Format != FormatMultipleChoice {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "format must be \"numeric\" or \"multiple_choice\"",
			Retryable: true,
		}
	}
	if q.AnswerType != AnswerTypeInteger && q.AnswerType != AnswerTypeDecimal && q.AnswerType != AnswerTypeFraction && q.AnswerType != AnswerTypeText {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "answer_type must be \"integer\", \"decimal\", \"fraction\", or \"text\"",
			Retryable: true,
		}
	}
	if q.AnswerType == AnswerTypeText && q.Format != FormatMultipleChoice {
		return &ValidationError{
			Validator: v.Name(),
			Message:   "answer_type \"text\" must use \"multiple_choice\" format",
			Retryable: true,
		}
	}
	return nil
}
