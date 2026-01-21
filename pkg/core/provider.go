package core

import (
	"context"
	"io"

	"github.com/vango-go/vai/pkg/core/types"
)

// Provider is the interface that all LLM providers must implement.
type Provider interface {
	// Name returns the provider identifier (e.g., "anthropic", "openai").
	Name() string

	// CreateMessage sends a non-streaming request.
	CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error)

	// StreamMessage sends a streaming request.
	StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error)

	// Capabilities returns what this provider supports.
	Capabilities() ProviderCapabilities
}

// EventStream is an iterator over streaming events.
type EventStream interface {
	// Next returns the next event. Returns nil, io.EOF when done.
	Next() (types.StreamEvent, error)

	// Close releases resources.
	Close() error
}

// Ensure io.EOF is accessible
var _ = io.EOF

// ProviderCapabilities describes what a provider supports.
type ProviderCapabilities struct {
	Vision           bool     `json:"vision"`
	AudioInput       bool     `json:"audio_input"`
	AudioOutput      bool     `json:"audio_output"`
	Video            bool     `json:"video"`
	Tools            bool     `json:"tools"`
	ToolStreaming    bool     `json:"tool_streaming"`
	Thinking         bool     `json:"thinking"`
	StructuredOutput bool     `json:"structured_output"`
	NativeTools      []string `json:"native_tools"` // "web_search", "code_execution", etc.
}

// ProviderRegistry manages available providers.
type ProviderRegistry interface {
	// Register adds a provider to the registry.
	Register(provider Provider)

	// Get returns a provider by name.
	Get(name string) (Provider, bool)

	// List returns all registered provider names.
	List() []string
}

// defaultRegistry is the default provider registry.
type defaultRegistry struct {
	providers map[string]Provider
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() ProviderRegistry {
	return &defaultRegistry{
		providers: make(map[string]Provider),
	}
}

func (r *defaultRegistry) Register(provider Provider) {
	r.providers[provider.Name()] = provider
}

func (r *defaultRegistry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *defaultRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
