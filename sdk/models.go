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
	if s.client.mode == modeDirect {
		return &ListModelsResponse{Models: []Model{}}, nil
	}

	var resp proxyModelsResponse
	if err := s.client.doProxyGET(ctx, "/v1/models", &resp); err != nil {
		return nil, err
	}

	models := make([]Model, 0, len(resp.Providers))
	for _, provider := range resp.Providers {
		models = append(models, Model{
			ID:           provider.ID,
			Provider:     provider.ID,
			Name:         provider.ID,
			DisplayName:  provider.Name,
			Capabilities: provider.Capabilities,
		})
	}

	return &ListModelsResponse{Models: models}, nil
}

// Get returns information about a specific model.
func (s *ModelsService) Get(ctx context.Context, modelID string) (*Model, error) {
	resp, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, model := range resp.Models {
		if model.ID == modelID {
			return &model, nil
		}
	}
	return nil, core.NewNotFoundError("model not found")
}

// ListByProvider returns models from a specific provider.
func (s *ModelsService) ListByProvider(ctx context.Context, provider string) (*ListModelsResponse, error) {
	resp, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]Model, 0, len(resp.Models))
	for _, model := range resp.Models {
		if model.Provider == provider {
			filtered = append(filtered, model)
		}
	}
	return &ListModelsResponse{Models: filtered}, nil
}

type proxyModelsResponse struct {
	Providers []struct {
		ID           string                    `json:"id"`
		Name         string                    `json:"name"`
		Capabilities core.ProviderCapabilities `json:"capabilities"`
	} `json:"providers"`
}
