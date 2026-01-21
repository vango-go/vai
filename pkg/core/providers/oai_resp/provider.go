package oai_resp

import (
	"context"
	"net/http"

	"github.com/vango-go/vai/pkg/core/types"
)

const (
	// DefaultBaseURL is the default OpenAI API endpoint.
	DefaultBaseURL = "https://api.openai.com/v1"

	// DefaultMaxTokens is the default max tokens if not specified.
	DefaultMaxTokens = 4096

	// MinMaxTokens is the minimum allowed by OpenAI Responses API.
	MinMaxTokens = 16
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

// Provider implements the OpenAI Responses API.
type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new OpenAI Responses API provider.
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
	return "oai-resp"
}

// Capabilities returns what this provider supports.
func (p *Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Vision:           true,
		AudioInput:       true,
		AudioOutput:      true,
		Video:            false,
		Tools:            true,
		ToolStreaming:    true,
		Thinking:         true, // o-series reasoning models
		StructuredOutput: true,
		NativeTools:      []string{"web_search", "code_interpreter", "file_search", "image_generation", "computer_use"},
	}
}

// CreateMessage sends a non-streaming request to OpenAI Responses API.
func (p *Provider) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	// Validate request for unsupported features
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Build the Responses API request
	respReq := p.buildRequest(req)

	// Make HTTP call
	respBody, err := p.doRequest(ctx, respReq)
	if err != nil {
		return nil, err
	}

	// Parse and translate response
	return p.parseResponse(respBody, req.Extensions)
}

// StreamMessage sends a streaming request to OpenAI Responses API.
func (p *Provider) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	// Validate request for unsupported features
	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Build request with stream=true
	respReq := p.buildRequest(req)
	respReq.Stream = true

	// Make HTTP call (returns SSE stream)
	body, err := p.doStreamRequest(ctx, respReq)
	if err != nil {
		return nil, err
	}

	return newEventStream(body), nil
}

// validateRequest checks for unsupported features and returns an error if found.
func (p *Provider) validateRequest(req *types.MessageRequest) error {
	if len(req.StopSequences) > 0 {
		return &Error{
			Type:    "unsupported_feature",
			Message: "stop_sequences is not supported by the OpenAI Responses API",
		}
	}
	return nil
}
