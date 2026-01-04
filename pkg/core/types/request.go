package types

// MessageRequest is the primary request structure for the Messages API.
// Based on Anthropic Messages API with extensions for multi-provider support.
type MessageRequest struct {
	Model    string    `json:"model"`    // "provider/model-name"
	Messages []Message `json:"messages"`

	// Generation parameters
	MaxTokens     int      `json:"max_tokens,omitempty"`
	System        any      `json:"system,omitempty"` // string or []ContentBlock
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Tools
	Tools      []Tool      `json:"tools,omitempty"`
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`

	// Streaming
	Stream bool `json:"stream,omitempty"`

	// Output configuration
	OutputFormat *OutputFormat `json:"output_format,omitempty"`
	Output       *OutputConfig `json:"output,omitempty"`

	// Voice pipeline
	Voice *VoiceConfig `json:"voice,omitempty"`

	// Provider-specific extensions
	Extensions map[string]any `json:"extensions,omitempty"`

	// User metadata (passthrough)
	Metadata map[string]any `json:"metadata,omitempty"`
}

// OutputFormat specifies structured output requirements.
type OutputFormat struct {
	Type       string      `json:"type"`                  // "json_schema"
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// OutputConfig specifies multimodal output settings.
type OutputConfig struct {
	Modalities []string     `json:"modalities,omitempty"` // ["text", "image", "audio"]
	Image      *ImageConfig `json:"image,omitempty"`
}

// ImageConfig specifies image generation parameters.
type ImageConfig struct {
	Size string `json:"size,omitempty"` // "1024x1024"
}

// JSONSchema represents a JSON Schema for structured output.
type JSONSchema struct {
	Type                 string                `json:"type"`
	Properties           map[string]JSONSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Description          string                `json:"description,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	Items                *JSONSchema           `json:"items,omitempty"`
	AdditionalProperties *bool                 `json:"additionalProperties,omitempty"`
}
