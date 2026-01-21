package oai_resp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// parseResponse converts an OpenAI Responses API response to Vango format.
func (p *Provider) parseResponse(body []byte, reqExtensions map[string]any) (*types.MessageResponse, error) {
	var respResp responsesResponse
	if err := json.Unmarshal(body, &respResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Check for API error in response
	if respResp.Error != nil {
		return nil, &Error{
			Type:    ErrorType(respResp.Error.Code),
			Message: respResp.Error.Message,
		}
	}

	content := make([]types.ContentBlock, 0)
	stopReason := mapResponseStatus(respResp.Status, respResp.IncompleteDetails)

	for _, item := range respResp.Output {
		switch item.Type {
		case "message":
			// Extract text content from message items
			for _, c := range item.Content {
				switch c.Type {
				case "output_text":
					if c.Text != "" {
						content = append(content, types.TextBlock{
							Type: "text",
							Text: c.Text,
						})
					}
				case "refusal":
					// Handle refusal as text
					if c.Text != "" {
						content = append(content, types.TextBlock{
							Type: "text",
							Text: "[Refusal] " + c.Text,
						})
					}
				}
			}

		case "function_call":
			// Map to tool_use block
			var input map[string]any
			if item.Arguments != "" {
				json.Unmarshal([]byte(item.Arguments), &input)
			}
			content = append(content, types.ToolUseBlock{
				Type:  "tool_use",
				ID:    item.CallID,
				Name:  item.Name,
				Input: input,
			})
			stopReason = types.StopReasonToolUse

		case "web_search_call":
			// Map web search to server tool use block
			content = append(content, types.ServerToolUseBlock{
				Type:  "server_tool_use",
				ID:    item.ID,
				Name:  "web_search",
				Input: map[string]any{"query": item.Query},
			})
			// Add search results if available
			if len(item.Results) > 0 {
				results := make([]types.WebSearchResultEntry, 0, len(item.Results))
				for _, r := range item.Results {
					results = append(results, types.WebSearchResultEntry{
						Type:  "web_search_result",
						URL:   r.URL,
						Title: r.Title,
					})
				}
				content = append(content, types.WebSearchToolResultBlock{
					Type:      "web_search_tool_result",
					ToolUseID: item.ID,
					Content:   results,
				})
			}

		case "code_interpreter_call":
			// Add the code as text
			if item.Code != "" {
				content = append(content, types.TextBlock{
					Type: "text",
					Text: fmt.Sprintf("```python\n%s\n```", item.Code),
				})
			}
			// Add execution output
			for _, out := range item.CodeOutput {
				if out.Type == "logs" && out.Logs != "" {
					content = append(content, types.TextBlock{
						Type: "text",
						Text: fmt.Sprintf("Output:\n%s", out.Logs),
					})
				}
			}

		case "file_search_call":
			// Map file search results
			if len(item.FileSearchResults) > 0 {
				for _, r := range item.FileSearchResults {
					content = append(content, types.TextBlock{
						Type: "text",
						Text: fmt.Sprintf("[File: %s]\n%s", r.FileName, r.Text),
					})
				}
			}

		case "image_generation_call":
			// Map generated image
			if item.ImageResult != nil && item.ImageResult.Data != "" {
				mediaType := "image/png"
				if item.ImageResult.Format == "jpeg" {
					mediaType = "image/jpeg"
				}
				content = append(content, types.ImageBlock{
					Type: "image",
					Source: types.ImageSource{
						Type:      "base64",
						MediaType: mediaType,
						Data:      item.ImageResult.Data,
					},
				})
			}

		case "reasoning":
			// Map reasoning to thinking block - summary is an array of summary items
			var thinkingText strings.Builder
			for _, s := range item.Summary {
				if s.Type == "summary_text" && s.Text != "" {
					if thinkingText.Len() > 0 {
						thinkingText.WriteString("\n")
					}
					thinkingText.WriteString(s.Text)
				}
			}
			if thinkingText.Len() > 0 {
				content = append(content, types.ThinkingBlock{
					Type:     "thinking",
					Thinking: thinkingText.String(),
				})
			}
		}
	}

	// If no text content was extracted from output items, check the output_text helper field
	if !hasTextContent(content) && respResp.OutputText != "" {
		content = append([]types.ContentBlock{types.TextBlock{
			Type: "text",
			Text: respResp.OutputText,
		}}, content...)
	}

	// Build response extensions to include response_id for state management
	responseExtensions := map[string]any{
		"oai_resp": map[string]any{
			"response_id": respResp.ID,
		},
	}

	// Map usage
	usage := types.Usage{
		InputTokens:  respResp.Usage.InputTokens,
		OutputTokens: respResp.Usage.OutputTokens,
		TotalTokens:  respResp.Usage.TotalTokens,
	}

	// Include cached tokens if available
	if respResp.Usage.InputTokensDetails != nil && respResp.Usage.InputTokensDetails.CachedTokens > 0 {
		cached := respResp.Usage.InputTokensDetails.CachedTokens
		usage.CacheReadTokens = &cached
	}

	return &types.MessageResponse{
		ID:         respResp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      "oai-resp/" + respResp.Model,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
		Metadata:   responseExtensions,
	}, nil
}

// mapResponseStatus maps OpenAI response status to stop reason.
func mapResponseStatus(status string, details *incompleteDetails) types.StopReason {
	switch status {
	case "completed":
		return types.StopReasonEndTurn
	case "incomplete":
		if details != nil {
			switch details.Reason {
			case "max_output_tokens":
				return types.StopReasonMaxTokens
			case "stop_sequence":
				return types.StopReasonStopSequence
			}
		}
		return types.StopReasonMaxTokens // Default for incomplete
	case "failed":
		return types.StopReasonEndTurn
	default:
		return types.StopReasonEndTurn
	}
}

// hasTextContent checks if content blocks contain any text.
func hasTextContent(content []types.ContentBlock) bool {
	for _, block := range content {
		if tb, ok := block.(types.TextBlock); ok && tb.Text != "" {
			return true
		}
	}
	return false
}

// mapStatus maps OpenAI Responses API status to stop reason.
func mapStatus(status string) types.StopReason {
	switch status {
	case "completed":
		return types.StopReasonEndTurn
	case "incomplete":
		return types.StopReasonMaxTokens
	case "failed":
		return types.StopReasonEndTurn
	default:
		return types.StopReasonEndTurn
	}
}
