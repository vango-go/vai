package types

import (
	"encoding/json"
	"fmt"
)

// ContentBlock is the interface for all content types.
// INPUT:  text, image, audio, video, document, tool_result
// OUTPUT: text, tool_use, thinking, image, audio
type ContentBlock interface {
	BlockType() string
}

// --- Input Blocks ---

// TextBlock represents text content.
type TextBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

func (t TextBlock) BlockType() string { return "text" }

// ImageBlock represents image content.
type ImageBlock struct {
	Type   string      `json:"type"` // "image"
	Source ImageSource `json:"source"`
}

func (t ImageBlock) BlockType() string { return "image" }

// ImageSource contains the image data or reference.
type ImageSource struct {
	Type      string `json:"type"`                 // "base64" or "url"
	MediaType string `json:"media_type,omitempty"` // "image/png", etc.
	Data      string `json:"data,omitempty"`       // base64 data
	URL       string `json:"url,omitempty"`        // URL reference
}

// AudioBlock represents audio content.
type AudioBlock struct {
	Type       string      `json:"type"` // "audio"
	Source     AudioSource `json:"source"`
	Transcript *string     `json:"transcript,omitempty"` // For output audio
}

func (t AudioBlock) BlockType() string { return "audio" }

// AudioSource contains the audio data.
type AudioSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "audio/wav", "audio/mp3", etc.
	Data      string `json:"data"`       // base64 data
}

// VideoBlock represents video content.
type VideoBlock struct {
	Type   string      `json:"type"` // "video"
	Source VideoSource `json:"source"`
}

func (t VideoBlock) BlockType() string { return "video" }

// VideoSource contains the video data.
type VideoSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "video/mp4"
	Data      string `json:"data"`
}

// DocumentBlock represents document content (PDF, etc.).
type DocumentBlock struct {
	Type     string         `json:"type"` // "document"
	Source   DocumentSource `json:"source"`
	Filename string         `json:"filename,omitempty"`
}

func (t DocumentBlock) BlockType() string { return "document" }

// DocumentSource contains the document data.
type DocumentSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "application/pdf"
	Data      string `json:"data"`
}

// ToolResultBlock represents the result of a tool call.
type ToolResultBlock struct {
	Type      string         `json:"type"` // "tool_result"
	ToolUseID string         `json:"tool_use_id"`
	Content   []ContentBlock `json:"content"`
	IsError   bool           `json:"is_error,omitempty"`
}

func (t ToolResultBlock) BlockType() string { return "tool_result" }

// MarshalJSON implements custom JSON marshaling for ToolResultBlock.
func (t ToolResultBlock) MarshalJSON() ([]byte, error) {
	// Create a map for marshaling
	m := map[string]any{
		"type":        t.Type,
		"tool_use_id": t.ToolUseID,
	}

	if t.IsError {
		m["is_error"] = true
	}

	// Marshal content blocks
	if len(t.Content) > 0 {
		contentJSON := make([]json.RawMessage, len(t.Content))
		for i, block := range t.Content {
			b, err := json.Marshal(block)
			if err != nil {
				return nil, err
			}
			contentJSON[i] = b
		}
		m["content"] = contentJSON
	}

	return json.Marshal(m)
}

// --- Output Blocks ---

// ToolUseBlock represents a tool call from the model.
type ToolUseBlock struct {
	Type  string         `json:"type"` // "tool_use"
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (t ToolUseBlock) BlockType() string { return "tool_use" }

// ThinkingBlock represents model reasoning output.
type ThinkingBlock struct {
	Type     string  `json:"type"` // "thinking"
	Thinking string  `json:"thinking"`
	Summary  *string `json:"summary,omitempty"`
}

func (t ThinkingBlock) BlockType() string { return "thinking" }

// --- Server-Side Tool Blocks (Anthropic native tools) ---

// ServerToolUseBlock represents a server-side tool invocation by Anthropic.
// This is used for native tools like web_search that are executed server-side.
type ServerToolUseBlock struct {
	Type  string         `json:"type"` // "server_tool_use"
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (t ServerToolUseBlock) BlockType() string { return "server_tool_use" }

// WebSearchToolResultBlock contains the results from web search.
type WebSearchToolResultBlock struct {
	Type      string                 `json:"type"` // "web_search_tool_result"
	ToolUseID string                 `json:"tool_use_id"`
	Content   []WebSearchResultEntry `json:"content"`
}

func (t WebSearchToolResultBlock) BlockType() string { return "web_search_tool_result" }

// WebSearchResultEntry is a single search result.
type WebSearchResultEntry struct {
	Type          string `json:"type"` // "web_search_result"
	URL           string `json:"url"`
	Title         string `json:"title"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
	PageAge       string `json:"page_age,omitempty"`
}

// UnmarshalContentBlock deserializes a content block from JSON.
func UnmarshalContentBlock(data []byte) (ContentBlock, error) {
	// First, determine the type
	var typeHolder struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeHolder); err != nil {
		return nil, err
	}

	switch typeHolder.Type {
	case "text":
		var block TextBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "image":
		var block ImageBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "audio":
		var block AudioBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "video":
		var block VideoBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "document":
		var block DocumentBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "tool_result":
		var block ToolResultBlock
		// Handle nested content blocks
		var raw struct {
			Type      string            `json:"type"`
			ToolUseID string            `json:"tool_use_id"`
			Content   []json.RawMessage `json:"content"`
			IsError   bool              `json:"is_error"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		block.Type = raw.Type
		block.ToolUseID = raw.ToolUseID
		block.IsError = raw.IsError
		for _, c := range raw.Content {
			cb, err := UnmarshalContentBlock(c)
			if err != nil {
				return nil, err
			}
			block.Content = append(block.Content, cb)
		}
		return block, nil

	case "tool_use":
		var block ToolUseBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "thinking":
		var block ThinkingBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "server_tool_use":
		var block ServerToolUseBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	case "web_search_tool_result":
		var block WebSearchToolResultBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}
		return block, nil

	default:
		// Return a generic text block for unknown types to avoid breaking on new Anthropic features
		return TextBlock{Type: typeHolder.Type, Text: fmt.Sprintf("[unknown block type: %s]", typeHolder.Type)}, nil
	}
}

// UnmarshalContentBlocks deserializes a slice of content blocks from JSON.
func UnmarshalContentBlocks(data []byte) ([]ContentBlock, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	blocks := make([]ContentBlock, len(raw))
	for i, r := range raw {
		block, err := UnmarshalContentBlock(r)
		if err != nil {
			return nil, err
		}
		blocks[i] = block
	}
	return blocks, nil
}
