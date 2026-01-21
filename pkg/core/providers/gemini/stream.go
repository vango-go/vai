package gemini

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// eventStream implements EventStream for Gemini SSE responses.
type eventStream struct {
	reader      *bufio.Reader
	closer      io.Closer
	model       string
	err         error
	accumulator streamAccumulator
	started     bool
	finished    bool
	pending     []types.StreamEvent // Queue for buffered events
}

// streamAccumulator accumulates streamed data.
type streamAccumulator struct {
	textIndex       int
	textStarted     bool
	textContent     strings.Builder
	toolCalls       map[int]*toolCallAccumulator
	finishReason    string
	inputTokens     int
	outputTokens    int
}

// toolCallAccumulator accumulates a single tool call.
type toolCallAccumulator struct {
	Name             string
	Args             map[string]any
	ThoughtSignature string
	announced        bool
}

// streamChunk represents a streaming chunk from Gemini.
type streamChunk struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
}

// newEventStream creates a new event stream from an HTTP response body.
func newEventStream(body io.ReadCloser, model string) *eventStream {
	return &eventStream{
		reader: bufio.NewReader(body),
		closer: body,
		model:  model,
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

		// Check for stream end markers
		if data == "[DONE]" || data == "" {
			return s.buildFinalEvent()
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // Skip unparseable chunks
		}

		// Handle usage (may come at end)
		if chunk.UsageMetadata != nil {
			s.accumulator.inputTokens = chunk.UsageMetadata.PromptTokenCount
			s.accumulator.outputTokens = chunk.UsageMetadata.CandidatesTokenCount
		}

		// No candidates means this is just a usage update
		if len(chunk.Candidates) == 0 {
			continue
		}

		candidate := chunk.Candidates[0]

		// Handle finish reason
		if candidate.FinishReason != "" {
			s.accumulator.finishReason = candidate.FinishReason
		}

		// Emit message_start if not yet started
		if !s.started {
			s.started = true
			// Queue message_start, then continue to process this chunk's parts
			s.pending = append(s.pending, types.MessageStartEvent{
				Type: "message_start",
				Message: types.MessageResponse{
					ID:      fmt.Sprintf("msg_%s", s.model),
					Type:    "message",
					Role:    "assistant",
					Model:   "gemini/" + s.model,
					Content: []types.ContentBlock{},
					Usage:   types.Usage{},
				},
			})
		}

		// Process parts
		for partIdx, part := range candidate.Content.Parts {
			// Handle text delta
			if part.Text != "" {
				// If this is the first text content, queue content_block_start first
				if !s.accumulator.textStarted {
					s.accumulator.textStarted = true
					s.pending = append(s.pending, types.ContentBlockStartEvent{
						Type:         "content_block_start",
						Index:        0,
						ContentBlock: types.TextBlock{Type: "text", Text: ""},
					})
				}

				s.accumulator.textContent.WriteString(part.Text)
				// Queue the delta event
				s.pending = append(s.pending, types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: types.TextDelta{
						Type: "text_delta",
						Text: part.Text,
					},
				})
			}

			// Handle function call
			if part.FunctionCall != nil {
				acc, exists := s.accumulator.toolCalls[partIdx]
				if !exists {
					acc = &toolCallAccumulator{
						Name:             part.FunctionCall.Name,
						Args:             part.FunctionCall.Args,
						ThoughtSignature: part.ThoughtSignature,
					}
					s.accumulator.toolCalls[partIdx] = acc
				}

				// Emit tool call start if not announced
				if !acc.announced {
					acc.announced = true
					// Tool use index starts after text (if text exists, it's at 0)
					toolIndex := partIdx
					if s.accumulator.textStarted {
						toolIndex = partIdx + 1
					}

					// Prepare input with thought signature if present
					input := acc.Args
					if input == nil {
						input = make(map[string]any)
					}
					if acc.ThoughtSignature != "" {
						input["__thought_signature"] = acc.ThoughtSignature
					}

					// Queue the tool use start event
					s.pending = append(s.pending, types.ContentBlockStartEvent{
						Type:  "content_block_start",
						Index: toolIndex,
						ContentBlock: types.ToolUseBlock{
							Type:  "tool_use",
							ID:    fmt.Sprintf("call_%s", acc.Name),
							Name:  acc.Name,
							Input: make(map[string]any),
						},
					})

					// Queue the input delta
					if len(acc.Args) > 0 {
						argsJSON, _ := json.Marshal(acc.Args)
						s.pending = append(s.pending, types.ContentBlockDeltaEvent{
							Type:  "content_block_delta",
							Index: toolIndex,
							Delta: types.InputJSONDelta{
								Type:        "input_json_delta",
								PartialJSON: string(argsJSON),
							},
						})
					}
				}
			}
		}

		// Return pending events if we have any
		if len(s.pending) > 0 {
			event := s.pending[0]
			s.pending = s.pending[1:]
			return event, nil
		}
	}
}

// buildFinalEvent builds the final events when stream ends.
func (s *eventStream) buildFinalEvent() (types.StreamEvent, error) {
	// If already finished, return EOF
	if s.finished {
		return nil, io.EOF
	}

	s.finished = true

	// Return message_delta with stop reason and usage (next call will return EOF)
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
	}, nil
}

// Close releases resources associated with the stream.
func (s *eventStream) Close() error {
	return s.closer.Close()
}
