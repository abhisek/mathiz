package store

import (
	"context"
	"fmt"

	"github.com/abhisek/mathiz/ent"
)

// eventRepo implements EventRepo backed by ent and the global sequence counter.
type eventRepo struct {
	client *ent.Client
	seq    *sequenceCounter
}

func (r *eventRepo) AppendLLMRequest(ctx context.Context, data LLMRequestEventData) error {
	seqNum, err := r.seq.Next(ctx)
	if err != nil {
		return fmt.Errorf("next sequence: %w", err)
	}

	_, err = r.client.LLMRequestEvent.Create().
		SetSequence(seqNum).
		SetProvider(data.Provider).
		SetModel(data.Model).
		SetPurpose(data.Purpose).
		SetInputTokens(data.InputTokens).
		SetOutputTokens(data.OutputTokens).
		SetLatencyMs(data.LatencyMs).
		SetSuccess(data.Success).
		SetErrorMessage(data.ErrorMessage).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save LLM request event: %w", err)
	}

	return nil
}
