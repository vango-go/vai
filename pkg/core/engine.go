package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// Engine is the core translation and routing engine.
// It handles provider selection, request translation, and response normalization.
type Engine struct {
	registry     ProviderRegistry
	providerKeys map[string]string
}

// NewEngine creates a new Engine with the given provider keys.
// If providerKeys is nil, environment variables will be used.
func NewEngine(providerKeys map[string]string) *Engine {
	if providerKeys == nil {
		providerKeys = make(map[string]string)
	}
	return &Engine{
		registry:     NewProviderRegistry(),
		providerKeys: providerKeys,
	}
}

// RegisterProvider adds a provider to the engine.
func (e *Engine) RegisterProvider(provider Provider) {
	e.registry.Register(provider)
}

// GetProvider returns a provider by name.
func (e *Engine) GetProvider(name string) (Provider, bool) {
	return e.registry.Get(name)
}

// ProviderNames returns the list of registered provider names.
func (e *Engine) ProviderNames() []string {
	return e.registry.List()
}

// CreateMessage routes the request to the appropriate provider.
func (e *Engine) CreateMessage(ctx context.Context, req *types.MessageRequest) (*types.MessageResponse, error) {
	providerName, modelName, err := ParseModelString(req.Model)
	if err != nil {
		return nil, err
	}

	provider, ok := e.registry.Get(providerName)
	if !ok {
		return nil, NewProviderError(providerName, fmt.Errorf("provider not registered"))
	}

	// Create a copy of the request with just the model name
	reqCopy := *req
	reqCopy.Model = modelName

	return provider.CreateMessage(ctx, &reqCopy)
}

// StreamMessage routes the streaming request to the appropriate provider.
func (e *Engine) StreamMessage(ctx context.Context, req *types.MessageRequest) (EventStream, error) {
	providerName, modelName, err := ParseModelString(req.Model)
	if err != nil {
		return nil, err
	}

	provider, ok := e.registry.Get(providerName)
	if !ok {
		return nil, NewProviderError(providerName, fmt.Errorf("provider not registered"))
	}

	// Create a copy of the request with just the model name
	reqCopy := *req
	reqCopy.Model = modelName

	return provider.StreamMessage(ctx, &reqCopy)
}

// GetAPIKey returns the API key for a provider.
// It first checks the explicit keys, then environment variables.
func (e *Engine) GetAPIKey(provider string) string {
	// Check explicit keys first
	if key, ok := e.providerKeys[provider]; ok {
		return key
	}

	// Check environment variables
	envKey := strings.ToUpper(provider) + "_API_KEY"
	return os.Getenv(envKey)
}

// ParseModelString parses a model string in the format "provider/model-name".
func ParseModelString(model string) (provider string, modelName string, err error) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return "", "", NewInvalidRequestError(
			fmt.Sprintf("invalid model format: %q, expected 'provider/model-name'", model),
		)
	}
	return parts[0], parts[1], nil
}
