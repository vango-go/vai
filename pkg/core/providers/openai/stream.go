package openai

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// eventStream implements EventStream for OpenAI SSE responses.
type eventStream struct {
	reader      *bufio.Reader
	closer      io.Closer
	err         error
	responseID  string
	model       string
	accumulator streamAccumulator
	started     bool
	finished    bool
	pending     []types.StreamEvent // Queue for buffered events
}

// streamAccumulator accumulates streamed data.
type streamAccumulator struct {
	textIndex     int
	textContent   strings.Builder
	toolCalls     map[int]*toolCallAccumulator
	finishReason  string
	inputTokens   int
	outputTokens  int
}

// toolCallAccumulator accumulates a single tool call.
type toolCallAccumulator struct {
	ID            string
	Name          string
	ArgumentsJSON strings.Builder
	announced     bool
}

// chatChunk is the OpenAI streaming chunk format.
type chatChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string          `json:"role,omitempty"`
			Content   string          `json:"content,omitempty"`
			ToolCalls []toolCallDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// toolCallDelta represents a tool call delta in streaming (has Index field).
type toolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// newEventStream creates a new event stream from an HTTP response body.
func newEventStream(body io.ReadCloser) *eventStream {
	return &eventStream{
		reader: bufio.NewReader(body),
		closer: body,
		accumulator: streamAccumulator{
			toolCalls: make(map[int]*toolCallAccumulator),
		},
	}
}

// Next returns the next event from the stream.
// Returns nil, io.EOF when the stream is complete.
func (s *eventStream) Next() (types.StreamEvent, error) {
	if s.err != nil {
		return nil, s.err
	}

	// Return pending events first
	if len(s.pending) > 0 {
		event := s.pending[0]
		s.pending = s.pending[1:]
		return event, nil
	}

	if s.finished {
		return nil, io.EOF
	}

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return s.buildFinalEvent()
			}
			s.err = err
			return nil, err
		}

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse SSE format: "data: <json>"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			return s.buildFinalEvent()
		}

		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // Skip unparseable chunks
		}

		// Store response ID and model
		if chunk.ID != "" {
			s.responseID = chunk.ID
		}
		if chunk.Model != "" {
			s.model = chunk.Model
		}

		// Handle usage (comes at end with stream_options)
		if chunk.Usage != nil {
			s.accumulator.inputTokens = chunk.Usage.PromptTokens
			s.accumulator.outputTokens = chunk.Usage.CompletionTokens
		}

		// No choices means this is just a usage update
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Handle finish reason
		if choice.FinishReason != "" {
			s.accumulator.finishReason = choice.FinishReason
		}

		// Emit message_start if not yet started
		if !s.started {
			s.started = true
			return types.MessageStartEvent{
				Type: "message_start",
				Message: types.MessageResponse{
					ID:      s.responseID,
					Type:    "message",
					Role:    "assistant",
					Model:   "openai/" + s.model,
					Content: []types.ContentBlock{},
					Usage:   types.Usage{},
				},
			}, nil
		}

		// Handle text delta
		if choice.Delta.Content != "" {
			// If this is the first text content, emit content_block_start AND delta
			if s.accumulator.textContent.Len() == 0 {
				s.accumulator.textContent.WriteString(choice.Delta.Content)
				// Queue the delta event for the next call
				s.pending = append(s.pending, types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: types.TextDelta{
						Type: "text_delta",
						Text: choice.Delta.Content,
					},
				})
				// Return the start event now
				return types.ContentBlockStartEvent{
					Type:         "content_block_start",
					Index:        0,
					ContentBlock: types.TextBlock{Type: "text", Text: ""},
				}, nil
			}

			s.accumulator.textContent.WriteString(choice.Delta.Content)
			return types.ContentBlockDeltaEvent{
				Type:  "content_block_delta",
				Index: 0,
				Delta: types.TextDelta{
					Type: "text_delta",
					Text: choice.Delta.Content,
				},
			}, nil
		}

		// Handle tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			idx := tc.Index
			acc, exists := s.accumulator.toolCalls[idx]
			if !exists {
				acc = &toolCallAccumulator{
					ID:   tc.ID,
					Name: tc.Function.Name,
				}
				s.accumulator.toolCalls[idx] = acc
			}

			// Update ID if provided
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			// Update name if provided
			if tc.Function.Name != "" {
				acc.Name = tc.Function.Name
			}

			// Emit tool call start if not announced
			if !acc.announced && acc.ID != "" && acc.Name != "" {
				acc.announced = true
				// Tool use index starts after text (if text exists, it's at 0)
				toolIndex := idx + 1
				if s.accumulator.textContent.Len() == 0 {
					toolIndex = idx
				}
				return types.ContentBlockStartEvent{
					Type:  "content_block_start",
					Index: toolIndex,
					ContentBlock: types.ToolUseBlock{
						Type:  "tool_use",
						ID:    acc.ID,
						Name:  acc.Name,
						Input: make(map[string]any),
					},
				}, nil
			}

			// Handle arguments delta
			if tc.Function.Arguments != "" {
				acc.ArgumentsJSON.WriteString(tc.Function.Arguments)
				toolIndex := idx + 1
				if s.accumulator.textContent.Len() == 0 {
					toolIndex = idx
				}
				return types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: toolIndex,
					Delta: types.InputJSONDelta{
						Type:        "input_json_delta",
						PartialJSON: tc.Function.Arguments,
					},
				}, nil
			}
		}
	}
}

// buildFinalEvent builds the final events when stream ends.
func (s *eventStream) buildFinalEvent() (types.StreamEvent, error) {
	s.finished = true

	// Return message_delta with stop reason and usage
	return types.MessageDeltaEvent{
		Type: "message_delta",
		Delta: struct {
			StopReason types.StopReason `json:"stop_reason,omitempty"`
		}{
			StopReason: mapFinishReason(s.accumulator.finishReason),
		},
		Usage: types.Usage{
			InputTokens:  s.accumulator.inputTokens,
			OutputTokens: s.accumulator.outputTokens,
			TotalTokens:  s.accumulator.inputTokens + s.accumulator.outputTokens,
		},
	}, io.EOF
}

// Close releases resources associated with the stream.
func (s *eventStream) Close() error {
	return s.closer.Close()
}
