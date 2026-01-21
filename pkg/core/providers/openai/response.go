package openai

import (
	"encoding/json"
	"fmt"

	"github.com/vango-go/vai/pkg/core/types"
)

// chatResponse is the OpenAI Chat Completions response format.
type chatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// parseResponse parses an OpenAI response into a Vango response.
func (p *Provider) parseResponse(body []byte) (*types.MessageResponse, error) {
	var openaiResp chatResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openaiResp.Choices[0]

	// Build content blocks
	content := make([]types.ContentBlock, 0)

	// Add text content
	if choice.Message.Content != nil {
		switch c := choice.Message.Content.(type) {
		case string:
			if c != "" {
				content = append(content, types.TextBlock{Type: "text", Text: c})
			}
		}
	}

	// Add tool calls as tool_use blocks
	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
			// If we can't parse the arguments, use empty map
			input = make(map[string]any)
		}

		content = append(content, types.ToolUseBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	// Map finish reason to stop reason
	stopReason := mapFinishReason(choice.FinishReason)

	return &types.MessageResponse{
		ID:         openaiResp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      "openai/" + openaiResp.Model,
		Content:    content,
		StopReason: stopReason,
		Usage: types.Usage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
			TotalTokens:  openaiResp.Usage.TotalTokens,
		},
	}, nil
}

// mapFinishReason converts OpenAI finish_reason to Vango stop_reason.
func mapFinishReason(reason string) types.StopReason {
	switch reason {
	case "stop":
		return types.StopReasonEndTurn
	case "length":
		return types.StopReasonMaxTokens
	case "tool_calls":
		return types.StopReasonToolUse
	case "content_filter":
		return types.StopReasonEndTurn
	default:
		return types.StopReasonEndTurn
	}
}
