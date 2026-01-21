package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// chatRequest is the OpenAI Chat Completions API request format.
type chatRequest struct {
	Model            string          `json:"model"`
	Messages         []chatMessage   `json:"messages"`
	MaxTokens        *int            `json:"max_completion_tokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	Tools            []chatTool      `json:"tools,omitempty"`
	ToolChoice       any             `json:"tool_choice,omitempty"`
	ResponseFormat   *responseFormat `json:"response_format,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	StreamOptions    *streamOptions  `json:"stream_options,omitempty"`
}

// chatMessage is a single message in OpenAI format.
type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"` // string or []contentPart
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// contentPart is a multimodal content part.
type contentPart struct {
	Type       string      `json:"type"`
	Text       string      `json:"text,omitempty"`
	ImageURL   *imageURL   `json:"image_url,omitempty"`
	InputAudio *inputAudio `json:"input_audio,omitempty"`
}

// imageURL contains image URL or data URL.
type imageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// inputAudio contains audio input data.
type inputAudio struct {
	Data   string `json:"data"`   // base64
	Format string `json:"format"` // "wav", "mp3"
}

// chatTool is a tool definition in OpenAI format.
type chatTool struct {
	Type     string       `json:"type"` // "function"
	Function toolFunction `json:"function"`
}

// toolFunction is the function definition.
type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// toolCall represents a tool call in OpenAI format.
type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// responseFormat specifies structured output format.
type responseFormat struct {
	Type       string      `json:"type"` // "json_schema", "json_object", "text"
	JSONSchema *jsonSchema `json:"json_schema,omitempty"`
}

// jsonSchema is the schema for structured output.
type jsonSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

// streamOptions configures streaming behavior.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// buildRequest converts a Vango request to an OpenAI request.
func (p *Provider) buildRequest(req *types.MessageRequest) *chatRequest {
	openaiReq := &chatRequest{
		Model:       req.Model, // Engine already stripped provider prefix
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
	}

	// Set max_tokens
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = DefaultMaxTokens
	}
	openaiReq.MaxTokens = &maxTokens

	// Translate messages
	openaiReq.Messages = p.translateMessages(req.Messages, req.System)

	// Translate tools
	if len(req.Tools) > 0 {
		openaiReq.Tools = p.translateTools(req.Tools)
	}

	// Translate tool choice
	if req.ToolChoice != nil {
		openaiReq.ToolChoice = p.translateToolChoice(req.ToolChoice)
	}

	// Translate output format for structured outputs
	if req.OutputFormat != nil && req.OutputFormat.Type == "json_schema" {
		schemaBytes, _ := json.Marshal(req.OutputFormat.JSONSchema)
		openaiReq.ResponseFormat = &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchema{
				Name:   "response",
				Schema: schemaBytes,
				Strict: true,
			},
		}
	}

	return openaiReq
}

// translateMessages converts Vango messages to OpenAI format.
func (p *Provider) translateMessages(messages []types.Message, system any) []chatMessage {
	result := make([]chatMessage, 0, len(messages)+1)

	// Add system message if present
	if system != nil {
		switch s := system.(type) {
		case string:
			result = append(result, chatMessage{Role: "system", Content: s})
		case []types.ContentBlock:
			// Convert content blocks to text for system
			var text string
			for _, block := range s {
				if tb, ok := block.(types.TextBlock); ok {
					text += tb.Text + "\n"
				}
			}
			result = append(result, chatMessage{Role: "system", Content: strings.TrimSpace(text)})
		}
	}

	for _, msg := range messages {
		blocks := msg.ContentBlocks()

		// Check if this message contains tool results
		// In OpenAI, each tool result must be a separate message with role "tool"
		hasToolResults := false
		for _, block := range blocks {
			if tr, ok := block.(types.ToolResultBlock); ok {
				hasToolResults = true
				result = append(result, chatMessage{
					Role:       "tool",
					ToolCallID: tr.ToolUseID,
					Content:    p.toolResultToText(tr.Content),
				})
			}
		}
		if hasToolResults {
			continue
		}

		openaiMsg := chatMessage{Role: msg.Role}

		// Check if assistant message has tool calls
		if msg.Role == "assistant" {
			for _, block := range blocks {
				if tu, ok := block.(types.ToolUseBlock); ok {
					inputJSON, _ := json.Marshal(tu.Input)
					openaiMsg.ToolCalls = append(openaiMsg.ToolCalls, toolCall{
						ID:   tu.ID,
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      tu.Name,
							Arguments: string(inputJSON),
						},
					})
				}
			}
		}

		// Convert content blocks
		openaiMsg.Content = p.translateContentBlocks(blocks)

		result = append(result, openaiMsg)
	}

	return result
}

// translateContentBlocks converts Vango content blocks to OpenAI format.
func (p *Provider) translateContentBlocks(blocks []types.ContentBlock) any {
	// Filter out tool use blocks (handled separately)
	var nonToolBlocks []types.ContentBlock
	for _, block := range blocks {
		if _, ok := block.(types.ToolUseBlock); !ok {
			nonToolBlocks = append(nonToolBlocks, block)
		}
	}

	// If empty after filtering, return empty string
	if len(nonToolBlocks) == 0 {
		return ""
	}

	// If only text, return as string
	if len(nonToolBlocks) == 1 {
		if tb, ok := nonToolBlocks[0].(types.TextBlock); ok {
			return tb.Text
		}
	}

	// Multiple blocks or non-text: use content parts array
	parts := make([]contentPart, 0, len(nonToolBlocks))

	for _, block := range nonToolBlocks {
		switch b := block.(type) {
		case types.TextBlock:
			parts = append(parts, contentPart{Type: "text", Text: b.Text})

		case types.ImageBlock:
			var url string
			if b.Source.Type == "url" {
				url = b.Source.URL
			} else {
				// Convert base64 to data URL
				url = fmt.Sprintf("data:%s;base64,%s", b.Source.MediaType, b.Source.Data)
			}
			parts = append(parts, contentPart{
				Type:     "image_url",
				ImageURL: &imageURL{URL: url},
			})

		case types.AudioBlock:
			parts = append(parts, contentPart{
				Type: "input_audio",
				InputAudio: &inputAudio{
					Data:   b.Source.Data,
					Format: getAudioFormat(b.Source.MediaType),
				},
			})

		case types.ToolUseBlock:
			// Skip tool use blocks - they're handled separately
			continue

		case types.ToolResultBlock:
			// Skip tool result blocks - they're handled separately
			continue
		}
	}

	// If we ended up with just one text part, return as string
	if len(parts) == 1 && parts[0].Type == "text" {
		return parts[0].Text
	}

	return parts
}

// translateTools converts Vango tools to OpenAI format.
func (p *Provider) translateTools(tools []types.Tool) []chatTool {
	result := make([]chatTool, 0, len(tools))

	for _, tool := range tools {
		switch tool.Type {
		case types.ToolTypeFunction:
			schemaBytes, _ := json.Marshal(tool.InputSchema)
			result = append(result, chatTool{
				Type: "function",
				Function: toolFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  schemaBytes,
				},
			})

		case types.ToolTypeWebSearch:
			// OpenAI doesn't have native web search in Chat Completions
			// This would need to be handled via a function tool or Responses API
			// For now, we skip it (could emit a warning)
			continue

		case types.ToolTypeCodeExecution:
			// OpenAI code_interpreter is only available in Assistants/Responses API
			// For Chat Completions, we skip it
			continue

		case types.ToolTypeFileSearch:
			// OpenAI file_search is only available in Assistants/Responses API
			// For Chat Completions, we skip it
			continue
		}
	}

	return result
}

// translateToolChoice converts Vango tool choice to OpenAI format.
func (p *Provider) translateToolChoice(tc *types.ToolChoice) any {
	switch tc.Type {
	case "auto":
		return "auto"
	case "none":
		return "none"
	case "any":
		return "required"
	case "tool":
		return map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": tc.Name,
			},
		}
	}
	return "auto"
}

// toolResultToText converts tool result content to text.
func (p *Provider) toolResultToText(content []types.ContentBlock) string {
	var result string
	for _, block := range content {
		if tb, ok := block.(types.TextBlock); ok {
			result += tb.Text
		}
	}
	return result
}

// getAudioFormat extracts the audio format from media type.
func getAudioFormat(mediaType string) string {
	switch mediaType {
	case "audio/wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/webm":
		return "webm"
	default:
		return "wav"
	}
}

// stripProviderPrefix removes the provider prefix from a model string.
// "openai/gpt-4o" -> "gpt-4o"
func stripProviderPrefix(model string) string {
	if idx := strings.Index(model, "/"); idx != -1 {
		return model[idx+1:]
	}
	return model
}
