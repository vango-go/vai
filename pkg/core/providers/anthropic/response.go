package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/vango-go/vai/pkg/core/types"
)

// anthropicResponse matches Anthropic's response format.
// Since Vango AI uses the same format, this is nearly identical to types.MessageResponse.
type anthropicResponse struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Role         string            `json:"role"`
	Model        string            `json:"model"`
	Content      []json.RawMessage `json:"content"`
	StopReason   string            `json:"stop_reason"`
	StopSequence *string           `json:"stop_sequence,omitempty"`
	Usage        types.Usage       `json:"usage"`
}

// parseResponse parses an Anthropic response into a Vango AI response.
func parseResponse(body []byte) (*types.MessageResponse, error) {
	var anthResp anthropicResponse
	if err := json.Unmarshal(body, &anthResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Parse content blocks
	content, err := parseContentBlocks(anthResp.Content)
	if err != nil {
		return nil, fmt.Errorf("parse content: %w", err)
	}

	return &types.MessageResponse{
		ID:           anthResp.ID,
		Type:         anthResp.Type,
		Role:         anthResp.Role,
		Model:        "anthropic/" + anthResp.Model, // Add provider prefix back
		Content:      content,
		StopReason:   types.StopReason(anthResp.StopReason),
		StopSequence: anthResp.StopSequence,
		Usage:        anthResp.Usage,
	}, nil
}

// parseContentBlocks parses raw JSON content blocks into typed ContentBlocks.
func parseContentBlocks(raw []json.RawMessage) ([]types.ContentBlock, error) {
	result := make([]types.ContentBlock, 0, len(raw))

	for _, r := range raw {
		block, err := types.UnmarshalContentBlock(r)
		if err != nil {
			// Skip unknown block types rather than failing
			continue
		}
		result = append(result, block)
	}

	return result, nil
}
