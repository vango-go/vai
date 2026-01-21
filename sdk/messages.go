package vai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/vango-go/vai/pkg/core/types"
)

// MessagesService handles message creation and streaming.
type MessagesService struct {
	client *Client
}

// MessageRequest is an alias for the core types.MessageRequest.
type MessageRequest = types.MessageRequest

// Message is an alias for the core types.Message.
type Message = types.Message

// Response is the SDK's response type wrapping the core response.
type Response struct {
	*types.MessageResponse
}

// Create sends a non-streaming message request.
// If VoiceConfig is provided:
//   - Audio input blocks are transcribed to text before sending to the LLM
//   - Text output is synthesized to audio and added as an AudioBlock
func (s *MessagesService) Create(ctx context.Context, req *MessageRequest) (*Response, error) {
	if s.client.mode == modeDirect {
		return s.createDirect(ctx, req)
	}

	// Proxy mode - HTTP request
	return s.createViaProxy(ctx, req)
}

// createDirect handles direct mode message creation with voice processing.
func (s *MessagesService) createDirect(ctx context.Context, req *MessageRequest) (*Response, error) {
	processedReq := req
	var userTranscript string

	// Process audio input if voice config is set
	if req.Voice != nil && req.Voice.Input != nil && s.client.voicePipeline != nil {
		processedMessages, transcript, err := s.client.voicePipeline.ProcessInputAudio(ctx, req.Messages, req.Voice)
		if err != nil {
			return nil, fmt.Errorf("process audio input: %w", err)
		}
		userTranscript = transcript

		// Create modified request with processed messages
		reqCopy := *req
		reqCopy.Messages = processedMessages
		processedReq = &reqCopy
	}

	// Make the LLM request
	resp, err := s.client.core.CreateMessage(ctx, processedReq)
	if err != nil {
		return nil, err
	}

	// Add user transcript to metadata if available
	if userTranscript != "" {
		if resp.Metadata == nil {
			resp.Metadata = make(map[string]any)
		}
		resp.Metadata["user_transcript"] = userTranscript
	}

	// Synthesize audio output if voice output is configured
	if req.Voice != nil && req.Voice.Output != nil && s.client.voicePipeline != nil {
		textContent := resp.TextContent()
		if textContent != "" {
			audioData, err := s.client.voicePipeline.SynthesizeResponse(ctx, textContent, req.Voice)
			if err != nil {
				return nil, fmt.Errorf("synthesize audio: %w", err)
			}

			if audioData != nil && len(audioData) > 0 {
				// Determine format
				format := req.Voice.Output.Format
				if format == "" {
					format = "wav"
				}
				mediaType := "audio/" + format

				// Add audio content block to response
				resp.Content = append(resp.Content, types.AudioBlock{
					Type: "audio",
					Source: types.AudioSource{
						Type:      "base64",
						MediaType: mediaType,
						Data:      base64.StdEncoding.EncodeToString(audioData),
					},
					Transcript: &textContent,
				})
			}
		}
	}

	return &Response{MessageResponse: resp}, nil
}

// createViaProxy sends a request via the proxy server.
func (s *MessagesService) createViaProxy(ctx context.Context, req *MessageRequest) (*Response, error) {
	var resp types.MessageResponse
	if err := s.client.doProxyJSON(ctx, http.MethodPost, "/v1/messages", req, &resp); err != nil {
		return nil, err
	}
	return &Response{MessageResponse: &resp}, nil
}

// Stream sends a streaming message request.
func (s *MessagesService) Stream(ctx context.Context, req *MessageRequest) (*Stream, error) {
	// Set stream flag
	reqCopy := *req
	reqCopy.Stream = true

	if s.client.mode == modeDirect {
		eventStream, err := s.client.core.StreamMessage(ctx, &reqCopy)
		if err != nil {
			return nil, err
		}
		return newStreamFromEventStream(eventStream), nil
	}

	// Proxy mode - SSE connection
	return s.streamViaProxy(ctx, &reqCopy)
}

// streamViaProxy establishes a streaming connection via the proxy.
func (s *MessagesService) streamViaProxy(ctx context.Context, req *MessageRequest) (*Stream, error) {
	resp, err := s.client.openProxyStream(ctx, "/v1/messages", req)
	if err != nil {
		return nil, err
	}

	eventStream := newProxyEventStream(resp.Body)
	return newStreamFromEventStream(eventStream), nil
}

// Run executes a tool execution loop.
// It automatically handles tool calls until a stop condition is met.
func (s *MessagesService) Run(ctx context.Context, req *MessageRequest, opts ...RunOption) (*RunResult, error) {
	cfg := defaultRunConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return s.runLoop(ctx, req, &cfg)
}

// RunStream executes a streaming tool execution loop.
// It streams events as they occur and handles tool calls automatically.
func (s *MessagesService) RunStream(ctx context.Context, req *MessageRequest, opts ...RunOption) (*RunStream, error) {
	cfg := defaultRunConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return s.runStreamLoop(ctx, req, &cfg), nil
}

// Extract executes a request and unmarshals the structured output into dest.
// It automatically generates a JSON schema from the dest type.
func (s *MessagesService) Extract(ctx context.Context, req *MessageRequest, dest any) (*Response, error) {
	// Validate dest is a pointer to a struct
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("dest must be a pointer")
	}
	if destValue.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("dest must be a pointer to a struct")
	}

	// Generate JSON schema from the struct
	schema := GenerateJSONSchema(destValue.Elem().Type())

	// Create a copy of the request with the schema
	reqCopy := *req
	reqCopy.OutputFormat = &types.OutputFormat{
		Type:       "json_schema",
		JSONSchema: schema,
	}

	// Execute the request
	resp, err := s.Create(ctx, &reqCopy)
	if err != nil {
		return nil, err
	}

	// Extract JSON from response
	text := resp.TextContent()
	if text == "" {
		return nil, fmt.Errorf("no text content in response")
	}

	// Unmarshal into dest
	if err := json.Unmarshal([]byte(text), dest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp, nil
}

// GenerateJSONSchema generates a JSON Schema from a Go struct type.
func GenerateJSONSchema(t reflect.Type) *types.JSONSchema {
	additionalPropertiesFalse := false
	schema := &types.JSONSchema{
		Type:                 "object",
		Properties:           make(map[string]types.JSONSchema),
		AdditionalProperties: &additionalPropertiesFalse,
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Get JSON field name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		// Handle "name,omitempty" format
		fieldName := jsonTag
		for j, c := range jsonTag {
			if c == ',' {
				fieldName = jsonTag[:j]
				break
			}
		}

		// Generate property schema
		propSchema := generatePropertySchema(field.Type)

		// Add description from tag
		if desc := field.Tag.Get("desc"); desc != "" {
			propSchema.Description = desc
		}

		// Add enum from tag
		if enum := field.Tag.Get("enum"); enum != "" {
			propSchema.Enum = splitEnum(enum)
		}

		schema.Properties[fieldName] = propSchema

		// Check if required (not omitempty)
		isRequired := true
		for j, c := range jsonTag {
			if c == ',' {
				if contains(jsonTag[j+1:], "omitempty") {
					isRequired = false
				}
				break
			}
		}
		if isRequired {
			schema.Required = append(schema.Required, fieldName)
		}
	}

	return schema
}

func generatePropertySchema(t reflect.Type) types.JSONSchema {
	switch t.Kind() {
	case reflect.String:
		return types.JSONSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return types.JSONSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return types.JSONSchema{Type: "number"}
	case reflect.Bool:
		return types.JSONSchema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		items := generatePropertySchema(t.Elem())
		return types.JSONSchema{Type: "array", Items: &items}
	case reflect.Ptr:
		return generatePropertySchema(t.Elem())
	case reflect.Struct:
		return *GenerateJSONSchema(t)
	default:
		return types.JSONSchema{Type: "string"}
	}
}

func splitEnum(s string) []string {
	var result []string
	var current string
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
