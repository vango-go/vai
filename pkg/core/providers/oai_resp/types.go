// Package oai_resp implements the OpenAI Responses API provider.
// It translates between the Vango API format (Anthropic-style) and OpenAI's Responses API format.
package oai_resp

import "encoding/json"

// --- Request Types ---

// responsesRequest is the OpenAI Responses API request format.
type responsesRequest struct {
	Model              string           `json:"model"`
	Input              any              `json:"input"`                         // string | []inputItem
	Instructions       string           `json:"instructions,omitempty"`        // system prompt
	Tools              []responsesTool  `json:"tools,omitempty"`
	ToolChoice         any              `json:"tool_choice,omitempty"`         // "auto" | "none" | "required" | specific
	ParallelToolCalls  *bool            `json:"parallel_tool_calls,omitempty"`
	MaxOutputTokens    *int             `json:"max_output_tokens,omitempty"`
	Temperature        *float64         `json:"temperature,omitempty"`
	TopP               *float64         `json:"top_p,omitempty"`
	Store              *bool            `json:"store,omitempty"`
	Stream             bool             `json:"stream,omitempty"`
	PreviousResponseID string           `json:"previous_response_id,omitempty"`
	Reasoning          *reasoningConfig `json:"reasoning,omitempty"`
	Text               *textConfig      `json:"text,omitempty"`
	Metadata           map[string]any   `json:"metadata,omitempty"`
}

// inputItem represents an item in the input array.
type inputItem struct {
	Type    string `json:"type"`              // "message", "function_call_output", "function_call"
	Role    string `json:"role,omitempty"`    // "user", "assistant", "system"
	Content any    `json:"content,omitempty"` // string | []contentPart
	// For function_call_output
	CallID string `json:"call_id,omitempty"`
	Output string `json:"output,omitempty"`
	// For function_call (input items representing previous assistant tool calls)
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// contentPart for multimodal input.
type contentPart struct {
	Type     string `json:"type"` // "input_text", "input_image", "input_audio", "input_file"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	// For base64 encoded content
	Image *imageData `json:"image,omitempty"`
	Audio *audioData `json:"audio,omitempty"`
}

// imageData contains base64 image data.
type imageData struct {
	URL    string `json:"url,omitempty"`    // data URL or http URL
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// audioData contains base64 audio data.
type audioData struct {
	Data   string `json:"data"`   // base64
	Format string `json:"format"` // "wav", "mp3"
}

// responsesTool represents a tool in Responses API format.
type responsesTool struct {
	Type string `json:"type"` // "function", "web_search", "code_interpreter", "file_search", "computer_use_preview", "image_generation"

	// For function tools
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`

	// For file_search
	VectorStoreIDs []string `json:"vector_store_ids,omitempty"`

	// For code_interpreter
	Container *containerConfig `json:"container,omitempty"`

	// For web_search
	SearchContextSize string `json:"search_context_size,omitempty"` // "low", "medium", "high"

	// For computer_use_preview
	DisplayWidth  int    `json:"display_width,omitempty"`
	DisplayHeight int    `json:"display_height,omitempty"`
	Environment   string `json:"environment,omitempty"` // "browser", "desktop"
}

// containerConfig for code_interpreter.
type containerConfig struct {
	Type        string   `json:"type,omitempty"`         // "auto"
	MemoryLimit string   `json:"memory_limit,omitempty"` // "1g", "2g", "4g", "8g"
	FileIDs     []string `json:"file_ids,omitempty"`
}

// reasoningConfig for o-series models.
type reasoningConfig struct {
	Effort  string `json:"effort,omitempty"`  // "low", "medium", "high"
	Summary string `json:"summary,omitempty"` // "auto", "concise", "detailed"
}

// textConfig for structured output.
type textConfig struct {
	Format *formatConfig `json:"format,omitempty"`
}

// formatConfig specifies output format.
type formatConfig struct {
	Type       string          `json:"type"`                  // "json_schema", "json_object", "text"
	Name       string          `json:"name,omitempty"`        // for json_schema
	Schema     json.RawMessage `json:"schema,omitempty"`      // for json_schema
	Strict     *bool           `json:"strict,omitempty"`      // for json_schema
}

// --- Response Types ---

// reasoningSummaryItem represents a summary item in reasoning output.
type reasoningSummaryItem struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
}

// incompleteDetails contains details about why a response is incomplete.
type incompleteDetails struct {
	Reason string `json:"reason"` // "max_output_tokens", "stop_sequence", etc.
}

// responsesResponse is the OpenAI Responses API response format.
type responsesResponse struct {
	ID                 string             `json:"id"`      // "resp_xxx"
	Object             string             `json:"object"`  // "response"
	Status             string             `json:"status"`  // "completed", "in_progress", "failed", "incomplete"
	IncompleteDetails  *incompleteDetails `json:"incomplete_details,omitempty"`
	Model              string             `json:"model"`
	Output             []outputItem       `json:"output"`
	OutputText         string             `json:"output_text"` // helper field with full text
	Usage              responsesUsage     `json:"usage"`
	Error              *responsesError    `json:"error,omitempty"`
	PreviousResponseID string             `json:"previous_response_id,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty"`
}

// outputItem represents an item in the output array.
type outputItem struct {
	Type   string `json:"type"` // "message", "function_call", "web_search_call", "code_interpreter_call", "file_search_call", "image_generation_call", "reasoning"
	ID     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"` // "completed", "in_progress"

	// For "message" type
	Role    string          `json:"role,omitempty"`
	Content []outputContent `json:"content,omitempty"`

	// For "function_call" type
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// For "web_search_call" type
	Query   string             `json:"query,omitempty"`
	Results []webSearchResult  `json:"results,omitempty"`

	// For "code_interpreter_call" type
	Code       string                  `json:"code,omitempty"`
	CodeOutput []codeInterpreterOutput `json:"output,omitempty"`

	// For "file_search_call" type
	FileSearchResults []fileSearchResult `json:"results,omitempty"`

	// For "image_generation_call" type
	ImageResult *imageGenResult `json:"result,omitempty"`

	// For "reasoning" type
	Summary []reasoningSummaryItem `json:"summary,omitempty"`
}

// outputContent represents content within a message output.
type outputContent struct {
	Type        string       `json:"type"` // "output_text", "refusal"
	Text        string       `json:"text,omitempty"`
	Annotations []annotation `json:"annotations,omitempty"`
}

// annotation represents an annotation in output text.
type annotation struct {
	Type       string `json:"type"` // "url_citation", "file_citation"
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
	FileID     string `json:"file_id,omitempty"`
}

// webSearchResult represents a web search result.
type webSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// codeInterpreterOutput represents code interpreter output.
type codeInterpreterOutput struct {
	Type    string `json:"type"` // "logs", "image"
	Logs    string `json:"logs,omitempty"`
	ImageID string `json:"image_id,omitempty"`
}

// fileSearchResult represents a file search result.
type fileSearchResult struct {
	FileID   string  `json:"file_id"`
	FileName string  `json:"filename"`
	Score    float64 `json:"score"`
	Text     string  `json:"text,omitempty"`
}

// imageGenResult represents an image generation result.
type imageGenResult struct {
	Data   string `json:"data,omitempty"`   // base64 encoded image
	Format string `json:"format,omitempty"` // "png", "jpeg"
}

// responsesUsage contains token usage information.
type responsesUsage struct {
	InputTokens        int                   `json:"input_tokens"`
	OutputTokens       int                   `json:"output_tokens"`
	TotalTokens        int                   `json:"total_tokens"`
	InputTokensDetails *inputTokensDetails   `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *outputTokensDetails `json:"output_tokens_details,omitempty"`
}

// inputTokensDetails contains detailed input token breakdown.
type inputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// outputTokensDetails contains detailed output token breakdown.
type outputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// responsesError represents an error in the response.
type responsesError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- Streaming Types ---

// streamEvent represents a streaming event from the Responses API.
type streamEvent struct {
	Type           string             `json:"type"`
	Response       *responsesResponse `json:"response,omitempty"`
	Item           *outputItem        `json:"item,omitempty"`
	Delta          string             `json:"delta,omitempty"`
	OutputIndex    int                `json:"output_index,omitempty"`
	ContentIndex   int                `json:"content_index,omitempty"`
	SequenceNumber int                `json:"sequence_number,omitempty"`
}

// Stream event types
const (
	eventResponseCreated              = "response.created"
	eventResponseInProgress           = "response.in_progress"
	eventResponseCompleted            = "response.completed"
	eventResponseFailed               = "response.failed"
	eventResponseIncomplete           = "response.incomplete"
	eventOutputItemAdded              = "response.output_item.added"
	eventOutputItemDone               = "response.output_item.done"
	eventContentPartAdded             = "response.content_part.added"
	eventContentPartDone              = "response.content_part.done"
	eventOutputTextDelta              = "response.output_text.delta"
	eventOutputTextDone               = "response.output_text.done"
	eventFunctionCallArgumentsDelta   = "response.function_call_arguments.delta"
	eventFunctionCallArgumentsDone    = "response.function_call_arguments.done"
	eventFileSearchCallSearching      = "response.file_search_call.searching"
	eventFileSearchCallCompleted      = "response.file_search_call.completed"
	eventWebSearchCallSearching       = "response.web_search_call.searching"
	eventWebSearchCallCompleted       = "response.web_search_call.completed"
	eventCodeInterpreterCallInProgress = "response.code_interpreter_call.in_progress"
	eventCodeInterpreterCallCompleted = "response.code_interpreter_call.completed"
)
