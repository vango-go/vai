package gemini_oauth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

var debugStream = os.Getenv("DEBUG_GEMINI_STREAM") != ""

// EventStream is the interface for streaming events.
type EventStream interface {
	Next() (types.StreamEvent, error)
	Close() error
}

// eventStream implements EventStream for Gemini OAuth SSE responses.
type eventStream struct {
	reader      *bufio.Reader
	closer      io.Closer
	model       string
	err         error
	accumulator streamAccumulator
	started     bool
	finished    bool
	pending     []types.StreamEvent
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
// The body should already be wrapped in TransformingReader if needed.
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
		if !strings.HasPrefix(line, "data:") {
			if debugStream {
				log.Printf("[STREAM] Skipping non-data line: %q", line)
			}
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

		if debugStream {
			log.Printf("[STREAM] Raw data: %s", data)
		}

		// Check for stream end markers
		if data == "[DONE]" || data == "" {
			return s.buildFinalEvent()
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if debugStream {
				log.Printf("[STREAM] Failed to parse chunk: %v", err)
			}
			continue // Skip unparseable chunks
		}

		if debugStream {
			log.Printf("[STREAM] Parsed chunk: candidates=%d", len(chunk.Candidates))
			if len(chunk.Candidates) > 0 {
				log.Printf("[STREAM] Candidate[0].Content.Parts=%d", len(chunk.Candidates[0].Content.Parts))
				for i, part := range chunk.Candidates[0].Content.Parts {
					log.Printf("[STREAM]   Part[%d]: text=%q functionCall=%v", i, part.Text, part.FunctionCall != nil)
				}
			}
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
		// Queue parts processing as pending events so they're emitted after message_start
		if !s.started {
			s.started = true

			// Process parts and queue them as pending events
			s.processParts(candidate.Content.Parts)

			return types.MessageStartEvent{
				Type: "message_start",
				Message: types.MessageResponse{
					ID:      fmt.Sprintf("msg_%s", s.model),
					Type:    "message",
					Role:    "assistant",
					Model:   "gemini-oauth/" + s.model,
					Content: []types.ContentBlock{},
					Usage:   types.Usage{},
				},
			}, nil
		}

		// Process parts
		for partIdx, part := range candidate.Content.Parts {
			// Handle text delta
			if part.Text != "" {
				if !s.accumulator.textStarted {
					s.accumulator.textStarted = true
					s.accumulator.textContent.WriteString(part.Text)
					s.pending = append(s.pending, types.ContentBlockDeltaEvent{
						Type:  "content_block_delta",
						Index: 0,
						Delta: types.TextDelta{
							Type: "text_delta",
							Text: part.Text,
						},
					})
					return types.ContentBlockStartEvent{
						Type:         "content_block_start",
						Index:        0,
						ContentBlock: types.TextBlock{Type: "text", Text: ""},
					}, nil
				}

				s.accumulator.textContent.WriteString(part.Text)
				return types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: types.TextDelta{
						Type: "text_delta",
						Text: part.Text,
					},
				}, nil
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

				if !acc.announced {
					acc.announced = true
					toolIndex := partIdx
					if s.accumulator.textStarted {
						toolIndex = partIdx + 1
					}

					input := acc.Args
					if input == nil {
						input = make(map[string]any)
					}
					if acc.ThoughtSignature != "" {
						input["__thought_signature"] = acc.ThoughtSignature
					}

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

					return types.ContentBlockStartEvent{
						Type:  "content_block_start",
						Index: toolIndex,
						ContentBlock: types.ToolUseBlock{
							Type:  "tool_use",
							ID:    fmt.Sprintf("call_%s", acc.Name),
							Name:  acc.Name,
							Input: make(map[string]any),
						},
					}, nil
				}
			}
		}
	}
}

// processParts processes content parts and queues them as pending events.
func (s *eventStream) processParts(parts []geminiPart) {
	for partIdx, part := range parts {
		// Handle text
		if part.Text != "" {
			if !s.accumulator.textStarted {
				s.accumulator.textStarted = true
				s.accumulator.textContent.WriteString(part.Text)
				// Queue content block start
				s.pending = append(s.pending, types.ContentBlockStartEvent{
					Type:         "content_block_start",
					Index:        0,
					ContentBlock: types.TextBlock{Type: "text", Text: ""},
				})
				// Queue text delta
				s.pending = append(s.pending, types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: types.TextDelta{
						Type: "text_delta",
						Text: part.Text,
					},
				})
			} else {
				s.accumulator.textContent.WriteString(part.Text)
				s.pending = append(s.pending, types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: 0,
					Delta: types.TextDelta{
						Type: "text_delta",
						Text: part.Text,
					},
				})
			}
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

			if !acc.announced {
				acc.announced = true
				toolIndex := partIdx
				if s.accumulator.textStarted {
					toolIndex = partIdx + 1
				}

				input := acc.Args
				if input == nil {
					input = make(map[string]any)
				}
				if acc.ThoughtSignature != "" {
					input["__thought_signature"] = acc.ThoughtSignature
				}

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
}

// buildFinalEvent builds the final events when stream ends.
func (s *eventStream) buildFinalEvent() (types.StreamEvent, error) {
	s.finished = true

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
