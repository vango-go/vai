package types

import "strings"

// MessageResponse is the response from the Messages API.
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         string         `json:"role"` // "assistant"
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   StopReason     `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
	Metadata     map[string]any `json:"metadata,omitempty"` // SDK-added metadata (e.g., user_transcript)
}

// StopReason indicates why generation stopped.
type StopReason string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonStopSequence StopReason = "stop_sequence"
	StopReasonToolUse      StopReason = "tool_use"
)

// TextContent returns all text content concatenated.
func (r *MessageResponse) TextContent() string {
	var parts []string
	for _, block := range r.Content {
		if tb, ok := block.(TextBlock); ok {
			parts = append(parts, tb.Text)
		}
		if tb, ok := block.(*TextBlock); ok {
			parts = append(parts, tb.Text)
		}
	}
	return strings.Join(parts, "")
}

// ToolUses returns all tool use blocks.
func (r *MessageResponse) ToolUses() []ToolUseBlock {
	var uses []ToolUseBlock
	for _, block := range r.Content {
		if tu, ok := block.(ToolUseBlock); ok {
			uses = append(uses, tu)
		}
		if tu, ok := block.(*ToolUseBlock); ok {
			uses = append(uses, *tu)
		}
	}
	return uses
}

// HasToolUse returns true if the response contains tool calls.
func (r *MessageResponse) HasToolUse() bool {
	for _, block := range r.Content {
		switch block.(type) {
		case ToolUseBlock, *ToolUseBlock:
			return true
		}
	}
	return false
}

// ThinkingContent returns all thinking content concatenated.
func (r *MessageResponse) ThinkingContent() string {
	var parts []string
	for _, block := range r.Content {
		if tb, ok := block.(ThinkingBlock); ok {
			parts = append(parts, tb.Thinking)
		}
		if tb, ok := block.(*ThinkingBlock); ok {
			parts = append(parts, tb.Thinking)
		}
	}
	return strings.Join(parts, "")
}

// ImageContent returns the first image block, or nil if none.
func (r *MessageResponse) ImageContent() *ImageBlock {
	for _, block := range r.Content {
		if ib, ok := block.(ImageBlock); ok {
			return &ib
		}
		if ib, ok := block.(*ImageBlock); ok {
			return ib
		}
	}
	return nil
}

// AudioContent returns the first audio block, or nil if none.
func (r *MessageResponse) AudioContent() *AudioBlock {
	for _, block := range r.Content {
		if ab, ok := block.(AudioBlock); ok {
			return &ab
		}
		if ab, ok := block.(*AudioBlock); ok {
			return ab
		}
	}
	return nil
}

// UserTranscript returns the transcribed user audio input, if any.
// This is populated when the request contained audio input and voice.input was configured.
// Returns empty string if this request didn't contain audio input.
func (r *MessageResponse) UserTranscript() string {
	if r.Metadata != nil {
		if t, ok := r.Metadata["user_transcript"].(string); ok {
			return t
		}
	}
	return ""
}
