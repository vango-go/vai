package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/vango-go/vai/pkg/core/types"
)

// geminiResponse is the Gemini API response format.
type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

// geminiCandidate represents a single candidate response.
type geminiCandidate struct {
	Content           geminiContent      `json:"content"`
	FinishReason      string             `json:"finishReason"`
	Index             int                `json:"index"`
	GroundingMetadata *groundingMetadata `json:"groundingMetadata,omitempty"`
	SafetyRatings     []safetyRating     `json:"safetyRatings,omitempty"`
}

// geminiUsage contains token usage information.
type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThinkingTokenCount   int `json:"thinkingTokenCount,omitempty"`
}

// groundingMetadata contains grounding/search results.
type groundingMetadata struct {
	WebSearchQueries  []string          `json:"webSearchQueries,omitempty"`
	SearchEntryPoint  *searchEntryPoint `json:"searchEntryPoint,omitempty"`
	GroundingChunks   []groundingChunk  `json:"groundingChunks,omitempty"`
	GroundingSupports []groundingSupport `json:"groundingSupports,omitempty"`
}

// searchEntryPoint contains search UI data.
type searchEntryPoint struct {
	RenderedContent string `json:"renderedContent,omitempty"`
}

// groundingChunk represents a single grounding source.
type groundingChunk struct {
	Web *webChunk `json:"web,omitempty"`
}

// webChunk contains web source information.
type webChunk struct {
	URI    string `json:"uri"`
	Title  string `json:"title"`
	Domain string `json:"domain,omitempty"`
}

// groundingSupport links text to sources.
type groundingSupport struct {
	Segment              *textSegment `json:"segment,omitempty"`
	GroundingChunkIndices []int       `json:"groundingChunkIndices,omitempty"`
	ConfidenceScores     []float64   `json:"confidenceScores,omitempty"`
}

// textSegment represents a text segment.
type textSegment struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	Text       string `json:"text"`
}

// safetyRating represents a safety assessment.
type safetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Blocked     bool   `json:"blocked"`
}

// parseResponse parses a Gemini response into a Vango response.
func (p *Provider) parseResponse(body []byte, model string) (*types.MessageResponse, error) {
	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := geminiResp.Candidates[0]

	// Build content blocks
	content := p.parseContentParts(candidate.Content.Parts)

	// Map finish reason to stop reason
	// Note: Gemini 3 returns STOP even for function calls, so we detect by content
	stopReason := mapFinishReason(candidate.FinishReason)
	if stopReason == types.StopReasonEndTurn && hasFunctionCalls(content) {
		stopReason = types.StopReasonToolUse
	}

	// Build usage
	usage := types.Usage{}
	if geminiResp.UsageMetadata != nil {
		usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount
		usage.OutputTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = geminiResp.UsageMetadata.TotalTokenCount
	}

	// Build response
	resp := &types.MessageResponse{
		ID:         fmt.Sprintf("msg_%s", model), // Gemini doesn't return IDs
		Type:       "message",
		Role:       "assistant",
		Model:      "gemini/" + model,
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}

	// Add grounding metadata to response metadata if present
	if candidate.GroundingMetadata != nil {
		resp.Metadata = map[string]any{
			"grounding": candidate.GroundingMetadata,
		}
	}

	return resp, nil
}

// parseContentParts converts Gemini parts to Vango content blocks.
func (p *Provider) parseContentParts(parts []geminiPart) []types.ContentBlock {
	content := make([]types.ContentBlock, 0, len(parts))

	for _, part := range parts {
		// Text content
		if part.Text != "" {
			content = append(content, types.TextBlock{
				Type: "text",
				Text: part.Text,
			})
		}

		// Function call
		if part.FunctionCall != nil {
			input := part.FunctionCall.Args
			if input == nil {
				input = make(map[string]any)
			}

			// Preserve thought signature for Gemini 3 function calling
			if part.ThoughtSignature != "" {
				input["__thought_signature"] = part.ThoughtSignature
			}

			content = append(content, types.ToolUseBlock{
				Type:  "tool_use",
				ID:    fmt.Sprintf("call_%s", part.FunctionCall.Name), // Gemini doesn't provide IDs
				Name:  part.FunctionCall.Name,
				Input: input,
			})
		}

		// Executable code result (from code_execution tool)
		// Gemini returns this as a special part type
		if part.InlineData != nil && part.InlineData.MIMEType == "text/x-python" {
			// This is code execution output - treat as text
			content = append(content, types.TextBlock{
				Type: "text",
				Text: part.InlineData.Data,
			})
		}
	}

	return content
}

// hasFunctionCalls checks if content contains any tool use blocks.
func hasFunctionCalls(content []types.ContentBlock) bool {
	for _, block := range content {
		if _, ok := block.(types.ToolUseBlock); ok {
			return true
		}
	}
	return false
}

// mapFinishReason converts Gemini finish reason to Vango stop reason.
func mapFinishReason(reason string) types.StopReason {
	switch reason {
	case "STOP":
		return types.StopReasonEndTurn
	case "MAX_TOKENS":
		return types.StopReasonMaxTokens
	case "SAFETY":
		return types.StopReasonEndTurn // Safety stops treated as end
	case "RECITATION":
		return types.StopReasonEndTurn
	case "TOOL_USE", "FUNCTION_CALL":
		return types.StopReasonToolUse
	default:
		return types.StopReasonEndTurn
	}
}
