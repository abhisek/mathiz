package llm

import (
	"context"
	"encoding/json"
	"sync"
)

// MockResponse is a canned response for the MockProvider.
type MockResponse struct {
	Content json.RawMessage
	Usage   Usage
	Err     error
}

// MockProvider is a deterministic Provider for testing.
// It returns canned responses in FIFO order and records all requests.
type MockProvider struct {
	mu        sync.Mutex
	responses []MockResponse
	Calls     []Request
}

// NewMockProvider creates a MockProvider with the given canned responses.
func NewMockProvider(responses ...MockResponse) *MockProvider {
	return &MockProvider{responses: responses}
}

// Generate returns the next canned response or ErrProviderUnavailable if
// the queue is empty.
func (m *MockProvider) Generate(_ context.Context, req Request) (*Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, req)

	if len(m.responses) == 0 {
		return nil, &ErrProviderUnavailable{Err: nil}
	}

	resp := m.responses[0]
	m.responses = m.responses[1:]

	if resp.Err != nil {
		return nil, resp.Err
	}

	return &Response{
		Content:    resp.Content,
		Usage:      resp.Usage,
		Model:      "mock",
		StopReason: "end",
	}, nil
}

// ModelID returns "mock".
func (m *MockProvider) ModelID() string {
	return "mock"
}

// AddResponse appends a canned response to the queue.
func (m *MockProvider) AddResponse(resp MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
}

// CallCount returns the number of Generate calls made.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}
