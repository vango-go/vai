package vai

import (
	"encoding/base64"

	"github.com/vango-go/vai/pkg/core/types"
)

// ContentBlock is an alias for the core types.ContentBlock interface.
type ContentBlock = types.ContentBlock

// Text creates a text content block.
func Text(s string) types.ContentBlock {
	return types.TextBlock{Type: "text", Text: s}
}

// Image creates an image content block from bytes.
func Image(data []byte, mediaType string) types.ContentBlock {
	return types.ImageBlock{
		Type: "image",
		Source: types.ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	}
}

// ImageURL creates an image content block from a URL.
func ImageURL(url string) types.ContentBlock {
	return types.ImageBlock{
		Type: "image",
		Source: types.ImageSource{
			Type: "url",
			URL:  url,
		},
	}
}

// Audio creates an audio content block.
func Audio(data []byte, mediaType string) types.ContentBlock {
	return types.AudioBlock{
		Type: "audio",
		Source: types.AudioSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	}
}

// Video creates a video content block.
func Video(data []byte, mediaType string) types.ContentBlock {
	return types.VideoBlock{
		Type: "video",
		Source: types.VideoSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	}
}

// Document creates a document content block.
func Document(data []byte, mediaType, filename string) types.ContentBlock {
	return types.DocumentBlock{
		Type: "document",
		Source: types.DocumentSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
		Filename: filename,
	}
}

// ToolResult creates a tool result content block.
func ToolResult(toolUseID string, content []types.ContentBlock) types.ContentBlock {
	return types.ToolResultBlock{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		Content:   content,
	}
}

// ToolResultError creates an error tool result.
func ToolResultError(toolUseID string, errMsg string) types.ContentBlock {
	return types.ToolResultBlock{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		Content:   []types.ContentBlock{Text(errMsg)},
		IsError:   true,
	}
}

// ContentBlocks is a helper to create a slice of content blocks.
func ContentBlocks(blocks ...types.ContentBlock) []types.ContentBlock {
	return blocks
}
