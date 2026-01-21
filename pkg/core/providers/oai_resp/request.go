package oai_resp

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// buildRequest converts a Vango request to an OpenAI Responses API request.
func (p *Provider) buildRequest(req *types.MessageRequest) *responsesRequest {
	respReq := &responsesRequest{
		Model: req.Model, // Engine already stripped provider prefix
	}

	// Reasoning models (o1, o3, gpt-5, etc.) don't support temperature/top_p
	if !isReasoningModel(req.Model) {
		respReq.Temperature = req.Temperature
		respReq.TopP = req.TopP
	} else if req.Temperature != nil || req.TopP != nil {
		log.Printf("oai_resp: warning: model %q does not support temperature/top_p parameters, ignoring", req.Model)
	}

	// Set max_output_tokens (enforce minimum of 16 required by OpenAI)
	if req.MaxTokens > 0 {
		maxTokens := req.MaxTokens
		if maxTokens < MinMaxTokens {
			maxTokens = MinMaxTokens
		}
		respReq.MaxOutputTokens = &maxTokens
	} else {
		defaultMax := DefaultMaxTokens
		respReq.MaxOutputTokens = &defaultMax
	}

	// System prompt -> instructions
	respReq.Instructions = p.systemToInstructions(req.System)

	// Messages -> input array
	respReq.Input = p.translateMessages(req.Messages)

	// Translate tools
	if len(req.Tools) > 0 {
		respReq.Tools = p.translateTools(req.Tools)
	}

	// Translate tool choice
	if req.ToolChoice != nil {
		respReq.ToolChoice = p.translateToolChoice(req.ToolChoice)
	}

	// Translate structured output
	if req.OutputFormat != nil && req.OutputFormat.Type == "json_schema" {
		respReq.Text = p.translateOutputFormat(req.OutputFormat)
	}

	// Apply extensions for reasoning, state management, etc.
	p.applyExtensions(respReq, req.Extensions)

	// Pass through metadata
	if len(req.Metadata) > 0 {
		respReq.Metadata = req.Metadata
	}

	return respReq
}

// systemToInstructions converts Vango system prompt to OpenAI instructions.
func (p *Provider) systemToInstructions(system any) string {
	if system == nil {
		return ""
	}

	switch s := system.(type) {
	case string:
		return s
	case []types.ContentBlock:
		var text strings.Builder
		for _, block := range s {
			if tb, ok := block.(types.TextBlock); ok {
				text.WriteString(tb.Text)
				text.WriteString("\n")
			}
		}
		return strings.TrimSpace(text.String())
	default:
		return ""
	}
}

// translateMessages converts Vango messages to Responses API input format.
func (p *Provider) translateMessages(messages []types.Message) []inputItem {
	items := make([]inputItem, 0, len(messages))

	for _, msg := range messages {
		blocks := msg.ContentBlocks()

		// Handle tool results - these become function_call_output items
		for _, block := range blocks {
			if tr, ok := block.(types.ToolResultBlock); ok {
				items = append(items, inputItem{
					Type:   "function_call_output",
					CallID: tr.ToolUseID,
					Output: p.toolResultToString(tr.Content),
				})
			}
		}

		// Check if this message only contained tool results
		hasNonToolResults := false
		for _, block := range blocks {
			if _, ok := block.(types.ToolResultBlock); !ok {
				hasNonToolResults = true
				break
			}
		}
		if !hasNonToolResults {
			continue
		}

		// For assistant messages with tool use, we need to emit function_call items
		if msg.Role == "assistant" {
			for _, block := range blocks {
				if tu, ok := block.(types.ToolUseBlock); ok {
					inputBytes, _ := json.Marshal(tu.Input)
					items = append(items, inputItem{
						Type:      "function_call",
						CallID:    tu.ID,
						Name:      tu.Name,
						Arguments: string(inputBytes),
					})
				}
			}
		}

		// Create message input item for text content
		textContent := p.blocksToText(blocks)
		if textContent != "" || !p.hasToolUseBlocks(blocks) {
			item := inputItem{
				Type: "message",
				Role: msg.Role,
			}

			// Check if we need multimodal content
			if p.hasMultimodalContent(blocks) {
				item.Content = p.translateContentParts(blocks)
			} else {
				// Simple text content
				item.Content = textContent
			}

			// Only add message item if it has content or if it's not an assistant with only tool calls
			if textContent != "" || msg.Role != "assistant" || !p.hasToolUseBlocks(blocks) {
				items = append(items, item)
			}
		}
	}

	return items
}

// hasToolUseBlocks checks if blocks contain tool use blocks.
func (p *Provider) hasToolUseBlocks(blocks []types.ContentBlock) bool {
	for _, block := range blocks {
		if _, ok := block.(types.ToolUseBlock); ok {
			return true
		}
	}
	return false
}

// hasMultimodalContent checks if blocks contain non-text content.
func (p *Provider) hasMultimodalContent(blocks []types.ContentBlock) bool {
	for _, block := range blocks {
		switch block.(type) {
		case types.ImageBlock, types.AudioBlock, types.VideoBlock, types.DocumentBlock:
			return true
		}
	}
	return false
}

// translateContentParts converts blocks to multimodal content parts.
func (p *Provider) translateContentParts(blocks []types.ContentBlock) []contentPart {
	parts := make([]contentPart, 0, len(blocks))

	for _, block := range blocks {
		switch b := block.(type) {
		case types.TextBlock:
			parts = append(parts, contentPart{
				Type: "input_text",
				Text: b.Text,
			})

		case types.ImageBlock:
			part := contentPart{Type: "input_image"}
			if b.Source.Type == "url" {
				part.ImageURL = b.Source.URL
			} else {
				// Convert base64 to data URL
				part.Image = &imageData{
					URL: fmt.Sprintf("data:%s;base64,%s", b.Source.MediaType, b.Source.Data),
				}
			}
			parts = append(parts, part)

		case types.AudioBlock:
			parts = append(parts, contentPart{
				Type: "input_audio",
				Audio: &audioData{
					Data:   b.Source.Data,
					Format: getAudioFormat(b.Source.MediaType),
				},
			})

		case types.DocumentBlock:
			// Documents are sent as files
			part := contentPart{Type: "input_file"}
			// If we have base64, we'd need to upload as file first
			// For now, treat as inline if possible
			parts = append(parts, part)

		case types.ToolUseBlock:
			// Skip tool use blocks in input (these are in assistant messages for history)
			continue

		case types.ToolResultBlock:
			// Already handled separately
			continue
		}
	}

	return parts
}

// blocksToText extracts text from blocks.
func (p *Provider) blocksToText(blocks []types.ContentBlock) string {
	var text strings.Builder
	for _, block := range blocks {
		switch b := block.(type) {
		case types.TextBlock:
			text.WriteString(b.Text)
		case types.ToolUseBlock:
			// Skip tool use in text extraction
			continue
		case types.ToolResultBlock:
			// Skip tool result in text extraction
			continue
		}
	}
	return text.String()
}

// toolResultToString converts tool result content to string output.
func (p *Provider) toolResultToString(content []types.ContentBlock) string {
	var result strings.Builder
	for _, block := range content {
		switch b := block.(type) {
		case types.TextBlock:
			result.WriteString(b.Text)
		case types.ImageBlock:
			// For images in tool results, include a reference
			result.WriteString("[Image data]")
		}
	}
	return result.String()
}

// translateOutputFormat converts Vango output format to Responses API text config.
func (p *Provider) translateOutputFormat(of *types.OutputFormat) *textConfig {
	if of == nil || of.Type != "json_schema" {
		return nil
	}

	schemaBytes, err := json.Marshal(of.JSONSchema)
	if err != nil {
		return nil
	}

	strict := true
	return &textConfig{
		Format: &formatConfig{
			Type:   "json_schema",
			Name:   "response",
			Schema: schemaBytes,
			Strict: &strict,
		},
	}
}

// applyExtensions applies provider-specific extensions to the request.
func (p *Provider) applyExtensions(respReq *responsesRequest, extensions map[string]any) {
	if extensions == nil {
		return
	}

	// Check for oai_resp specific extensions
	oaiExt, ok := extensions["oai_resp"].(map[string]any)
	if !ok {
		// Also check for "openai" for compatibility
		oaiExt, ok = extensions["openai"].(map[string]any)
		if !ok {
			return
		}
	}

	// Previous response ID for multi-turn state
	if prevID, ok := oaiExt["previous_response_id"].(string); ok && prevID != "" {
		respReq.PreviousResponseID = prevID
	}

	// Reasoning configuration for o-series models
	if reasoning, ok := oaiExt["reasoning"].(map[string]any); ok {
		respReq.Reasoning = &reasoningConfig{
			Effort:  getString(reasoning, "effort", ""),
			Summary: getString(reasoning, "summary", ""),
		}
	}

	// Store configuration
	if store, ok := oaiExt["store"].(bool); ok {
		respReq.Store = &store
	}
}

// getString safely extracts a string from a map.
func getString(m map[string]any, key, defaultVal string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return defaultVal
}

// isReasoningModel checks if the model is a reasoning model that doesn't support temperature.
func isReasoningModel(model string) bool {
	model = strings.ToLower(model)
	// o1, o3, o4 series and gpt-5 are reasoning models
	if strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4") {
		return true
	}
	if strings.Contains(model, "gpt-5") {
		return true
	}
	return false
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
	case "audio/ogg":
		return "ogg"
	default:
		return "wav"
	}
}
