package vai

import (
	"context"

	"github.com/vango-go/vai/pkg/core"
)

// ModelsService handles model listing and information.
type ModelsService struct {
	client *Client
}

// Model represents an available model.
type Model struct {
	ID           string                    `json:"id"`           // "anthropic/claude-sonnet-4"
	Provider     string                    `json:"provider"`     // "anthropic"
	Name         string                    `json:"name"`         // "claude-sonnet-4"
	DisplayName  string                    `json:"display_name"` // "Claude Sonnet 4"
	Description  string                    `json:"description,omitempty"`
	Context      int                       `json:"context"`    // Max context window
	MaxOutput    int                       `json:"max_output"` // Max output tokens
	Capabilities core.ProviderCapabilities `json:"capabilities"`
	Pricing      *ModelPricing             `json:"pricing,omitempty"`
}

// ModelPricing contains pricing information for a model.
type ModelPricing struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
	Currency         string  `json:"currency"` // "USD"
}

// ListModelsResponse contains the list of available models.
type ListModelsResponse struct {
	Models []Model `json:"models"`
}

// List returns all available models.
func (s *ModelsService) List(ctx context.Context) (*ListModelsResponse, error) {
	// TODO: Implement model listing
	return &ListModelsResponse{
		Models: []Model{},
	}, nil
}

// Get returns information about a specific model.
func (s *ModelsService) Get(ctx context.Context, modelID string) (*Model, error) {
	// TODO: Implement model retrieval
	return nil, nil
}

// ListByProvider returns models from a specific provider.
func (s *ModelsService) ListByProvider(ctx context.Context, provider string) (*ListModelsResponse, error) {
	// TODO: Implement filtered model listing
	return &ListModelsResponse{
		Models: []Model{},
	}, nil
}
