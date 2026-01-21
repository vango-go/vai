// Package anthropic implements the Anthropic Messages API provider.
// Since the Vango AI API is based on Anthropic's Messages API, this provider
// is essentially a passthrough with minimal translation.
package anthropic

import (
	"context"
	"net/http"

	"github.com/vango-go/vai/pkg/core/types"
)

const (
	// DefaultBaseURL is the default Anthropic API endpoint.
	DefaultBaseURL = "https://api.anthropic.com"

	// APIVersion is the required Anthropic API version header.
	APIVersion = "2023-06-01"

	// BetaHeader enables beta features like prompt caching, web search, and structured outputs.
	BetaHeader = "prompt-caching-2024-07-31,web-search-2025-03-05,structured-outputs-2025-11-13"

	// DefaultMaxTokens is the default max tokens if not specified.
	DefaultMaxTokens = 4096
)

// ProviderCapabilities describes what a provider supports.
// This is a local copy to avoid import cycles.
type ProviderCapabilities struct {
	Vision           bool
	AudioInput       bool
	AudioOutput      bool
	Video            bool
	Tools            bool
	ToolStreaming    bool
	Thinking         bool
	StructuredOutput bool
	NativeTools      []string
}

// EventStream is an iterator over streaming events.
type EventStream interface {
	Next() (types.StreamEvent, error)
	Close() error
}

// Provider implements the Anthropic Messages API.
type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new Anthropic provider.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "anthropic"
}

// Capabilities returns what this provider supports.
func (p *Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Vision:           true,
		AudioInput:       false,
		AudioOutput:      false,
		Video:            false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true,
		StructuredOutput: true,
		NativeTools:      []string{"web_search", "code_execution", "computer_use", "text_editor"},
	}
}

// CreateMessage sends a non-streaming request to Anthropic.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	// Build the Anthropic request
	anthReq := buildRequest(req)

	// Make HTTP call
	respBody, err := p.doRequest(ctx, anthReq, false)
	if err != nil {
		return nil, err
	}

	// Parse response (minimal translation needed - same format)
	return parseResponse(respBody)
}

// StreamMessage sends a streaming request to Anthropic.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	// Build request with stream=true
	anthReq := buildRequest(req)
	anthReq.Stream = true

	// Make HTTP call (returns SSE stream)
	body, err := p.doStreamRequest(ctx, anthReq)
	if err != nil {
		return nil, err
	}

	return newEventStream(body), nil
}
