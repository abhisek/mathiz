package llm

// ModelCost holds per-million-token pricing for a model.
// Prices are in USD per 1 million tokens, sourced from models.dev.
type ModelCost struct {
	InputPerMTok  float64 // USD per 1M input tokens
	OutputPerMTok float64 // USD per 1M output tokens
}

// Cost calculates the total USD cost for the given token counts.
func (c ModelCost) Cost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)*c.InputPerMTok/1_000_000 +
		float64(outputTokens)*c.OutputPerMTok/1_000_000
}

// LookupCost returns the pricing for a model ID, or nil if unknown.
func LookupCost(modelID string) *ModelCost {
	if c, ok := modelCosts[modelID]; ok {
		return &c
	}
	return nil
}

// modelCosts is the embedded pricing table extracted from models.dev.
// Last updated: 2026-02-15.
var modelCosts = map[string]ModelCost{
	// Anthropic
	"claude-3-5-haiku-20241022":    {0.8, 4},
	"claude-3-5-haiku-latest":      {0.8, 4},
	"claude-3-5-sonnet-20240620":   {3, 15},
	"claude-3-5-sonnet-20241022":   {3, 15},
	"claude-3-7-sonnet-20250219":   {3, 15},
	"claude-3-7-sonnet-latest":     {3, 15},
	"claude-3-haiku-20240307":      {0.25, 1.25},
	"claude-3-opus-20240229":       {15, 75},
	"claude-3-sonnet-20240229":     {3, 15},
	"claude-haiku-4-5":             {1, 5},
	"claude-haiku-4-5-20251001":    {1, 5},
	"claude-opus-4-0":              {15, 75},
	"claude-opus-4-1":              {15, 75},
	"claude-opus-4-1-20250805":     {15, 75},
	"claude-opus-4-20250514":       {15, 75},
	"claude-opus-4-5":              {5, 25},
	"claude-opus-4-5-20251101":     {5, 25},
	"claude-opus-4-6":              {5, 25},
	"claude-sonnet-4-0":            {3, 15},
	"claude-sonnet-4-20250514":     {3, 15},
	"claude-sonnet-4-5":            {3, 15},
	"claude-sonnet-4-5-20250929":   {3, 15},

	// OpenAI
	"codex-mini-latest":     {1.5, 6},
	"gpt-3.5-turbo":         {0.5, 1.5},
	"gpt-4":                 {30, 60},
	"gpt-4-turbo":           {10, 30},
	"gpt-4.1":               {2, 8},
	"gpt-4.1-mini":          {0.4, 1.6},
	"gpt-4.1-nano":          {0.1, 0.4},
	"gpt-4o":                {2.5, 10},
	"gpt-4o-2024-05-13":     {5, 15},
	"gpt-4o-2024-08-06":     {2.5, 10},
	"gpt-4o-2024-11-20":     {2.5, 10},
	"gpt-4o-mini":           {0.15, 0.6},
	"gpt-5":                 {1.25, 10},
	"gpt-5-chat-latest":     {1.25, 10},
	"gpt-5-codex":           {1.25, 10},
	"gpt-5-mini":            {0.25, 2},
	"gpt-5-nano":            {0.05, 0.4},
	"gpt-5-pro":             {15, 120},
	"gpt-5.1":               {1.25, 10},
	"gpt-5.1-chat-latest":   {1.25, 10},
	"gpt-5.1-codex":         {1.25, 10},
	"gpt-5.1-codex-max":     {1.25, 10},
	"gpt-5.1-codex-mini":    {0.25, 2},
	"gpt-5.2":               {1.75, 14},
	"gpt-5.2-chat-latest":   {1.75, 14},
	"gpt-5.2-codex":         {1.75, 14},
	"gpt-5.2-pro":           {21, 168},
	"gpt-5.3-codex":         {1.75, 14},
	"gpt-5.3-codex-spark":   {1.75, 14},
	"o1":                    {15, 60},
	"o1-mini":               {1.1, 4.4},
	"o1-preview":            {15, 60},
	"o1-pro":                {150, 600},
	"o3":                    {2, 8},
	"o3-deep-research":      {10, 40},
	"o3-mini":               {1.1, 4.4},
	"o3-pro":                {20, 80},
	"o4-mini":               {1.1, 4.4},
	"o4-mini-deep-research": {2, 8},

	// Google (Gemini)
	"gemini-1.5-flash":                           {0.075, 0.3},
	"gemini-1.5-flash-8b":                        {0.0375, 0.15},
	"gemini-1.5-pro":                             {1.25, 5},
	"gemini-2.0-flash":                           {0.1, 0.4},
	"gemini-2.0-flash-lite":                      {0.075, 0.3},
	"gemini-2.5-flash":                           {0.3, 2.5},
	"gemini-2.5-flash-lite":                      {0.1, 0.4},
	"gemini-2.5-flash-lite-preview-06-17":        {0.1, 0.4},
	"gemini-2.5-flash-lite-preview-09-2025":      {0.1, 0.4},
	"gemini-2.5-flash-preview-04-17":             {0.15, 0.6},
	"gemini-2.5-flash-preview-05-20":             {0.15, 0.6},
	"gemini-2.5-flash-preview-09-2025":           {0.3, 2.5},
	"gemini-2.5-pro":                             {1.25, 10},
	"gemini-2.5-pro-preview-05-06":               {1.25, 10},
	"gemini-2.5-pro-preview-06-05":               {1.25, 10},
	"gemini-3-flash-preview":                     {0.5, 3},
	"gemini-3-pro-preview":                       {2, 12},
	"gemini-flash-latest":                        {0.3, 2.5},
	"gemini-flash-lite-latest":                   {0.1, 0.4},
}
