package oai_resp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// eventStream implements EventStream for OpenAI Responses API SSE responses.
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
	textBlockIndex   int
	textContent      strings.Builder
	toolCalls        map[int]*toolCallAccumulator
	currentItemIndex int
	finishReason     string
	inputTokens      int
	outputTokens     int
}

// toolCallAccumulator accumulates a single tool call.
type toolCallAccumulator struct {
	ID            string
	Name          string
	ArgumentsJSON strings.Builder
	announced     bool
	blockIndex    int
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

	// If finished and no pending events, return EOF
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

		// Parse SSE format: "event: <type>" followed by "data: <json>"
		if strings.HasPrefix(line, "event:") {
			// Read the event type but we primarily use the data
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			return s.buildFinalEvent()
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue // Skip unparseable chunks
		}

		// Handle different event types
		result, hasEvent := s.handleStreamEvent(&event)
		if hasEvent {
			return result, nil
		}
	}
}

// handleStreamEvent processes a stream event and returns a Vango event if applicable.
func (s *eventStream) handleStreamEvent(event *streamEvent) (types.StreamEvent, bool) {
	switch event.Type {
	case eventResponseCreated:
		// Response created - emit message_start
		if event.Response != nil {
			s.responseID = event.Response.ID
			s.model = event.Response.Model
		}
		s.started = true
		return types.MessageStartEvent{
			Type: "message_start",
			Message: types.MessageResponse{
				ID:      s.responseID,
				Type:    "message",
				Role:    "assistant",
				Model:   "oai-resp/" + s.model,
				Content: []types.ContentBlock{},
				Usage:   types.Usage{},
			},
		}, true

	case eventOutputItemAdded:
		// New output item added
		if event.Item != nil {
			s.accumulator.currentItemIndex = event.OutputIndex
			return s.handleOutputItemAdded(event.Item, event.OutputIndex)
		}

	case eventOutputTextDelta:
		// Text delta
		if event.Delta != "" {
			// If this is the first text content, emit content_block_start AND delta
			if s.accumulator.textContent.Len() == 0 {
				s.accumulator.textContent.WriteString(event.Delta)
				// Queue the delta event for the next call
				s.pending = append(s.pending, types.ContentBlockDeltaEvent{
					Type:  "content_block_delta",
					Index: s.accumulator.textBlockIndex,
					Delta: types.TextDelta{
						Type: "text_delta",
						Text: event.Delta,
					},
				})
				// Return the start event now
				return types.ContentBlockStartEvent{
					Type:         "content_block_start",
					Index:        s.accumulator.textBlockIndex,
					ContentBlock: types.TextBlock{Type: "text", Text: ""},
				}, true
			}

			s.accumulator.textContent.WriteString(event.Delta)
			return types.ContentBlockDeltaEvent{
				Type:  "content_block_delta",
				Index: s.accumulator.textBlockIndex,
				Delta: types.TextDelta{
					Type: "text_delta",
					Text: event.Delta,
				},
			}, true
		}

	case eventFunctionCallArgumentsDelta:
		// Function call arguments delta
		if event.Delta != "" {
			idx := event.OutputIndex
			acc, exists := s.accumulator.toolCalls[idx]
			if !exists {
				acc = &toolCallAccumulator{
					blockIndex: s.getNextBlockIndex(),
				}
				s.accumulator.toolCalls[idx] = acc
			}

			acc.ArgumentsJSON.WriteString(event.Delta)
			return types.ContentBlockDeltaEvent{
				Type:  "content_block_delta",
				Index: acc.blockIndex,
				Delta: types.InputJSONDelta{
					Type:        "input_json_delta",
					PartialJSON: event.Delta,
				},
			}, true
		}

	case eventOutputTextDone:
		// Text content finished - emit content_block_stop
		if s.accumulator.textContent.Len() > 0 {
			return types.ContentBlockStopEvent{
				Type:  "content_block_stop",
				Index: s.accumulator.textBlockIndex,
			}, true
		}

	case eventOutputItemDone:
		// Output item complete
		if event.Item != nil {
			return s.handleOutputItemDone(event.Item, event.OutputIndex)
		}

	case eventResponseCompleted:
		// Response completed - extract usage
		if event.Response != nil {
			s.accumulator.inputTokens = event.Response.Usage.InputTokens
			s.accumulator.outputTokens = event.Response.Usage.OutputTokens
		}
		s.finished = true
		stopReason := types.StopReasonEndTurn
		if s.accumulator.finishReason == "tool_calls" {
			stopReason = types.StopReasonToolUse
		}
		// Queue message_stop for next call
		s.pending = append(s.pending, types.MessageStopEvent{
			Type: "message_stop",
		})
		return types.MessageDeltaEvent{
			Type: "message_delta",
			Delta: struct {
				StopReason types.StopReason `json:"stop_reason,omitempty"`
			}{
				StopReason: stopReason,
			},
			Usage: types.Usage{
				InputTokens:  s.accumulator.inputTokens,
				OutputTokens: s.accumulator.outputTokens,
				TotalTokens:  s.accumulator.inputTokens + s.accumulator.outputTokens,
			},
		}, true

	case eventResponseFailed:
		// Response failed
		errMsg := "Request failed"
		if event.Response != nil && event.Response.Error != nil {
			errMsg = event.Response.Error.Message
		}
		s.finished = true
		return types.ErrorEvent{
			Type: "error",
			Error: types.Error{
				Type:    "provider_error",
				Message: errMsg,
			},
		}, true
	}

	return nil, false
}

// handleOutputItemAdded handles a new output item.
func (s *eventStream) handleOutputItemAdded(item *outputItem, index int) (types.StreamEvent, bool) {
	switch item.Type {
	case "message":
		// Text message - will get deltas via output_text.delta
		s.accumulator.textBlockIndex = index
		return nil, false

	case "function_call":
		// Function call started
		blockIndex := s.getNextBlockIndex()
		s.accumulator.toolCalls[index] = &toolCallAccumulator{
			ID:         item.CallID,
			Name:       item.Name,
			announced:  true,
			blockIndex: blockIndex,
		}
		return types.ContentBlockStartEvent{
			Type:  "content_block_start",
			Index: blockIndex,
			ContentBlock: types.ToolUseBlock{
				Type:  "tool_use",
				ID:    item.CallID,
				Name:  item.Name,
				Input: make(map[string]any),
			},
		}, true
	}

	return nil, false
}

// handleOutputItemDone handles a completed output item.
func (s *eventStream) handleOutputItemDone(item *outputItem, index int) (types.StreamEvent, bool) {
	switch item.Type {
	case "function_call":
		// Mark that we need tool use
		s.accumulator.finishReason = "tool_calls"
		// Emit content_block_stop for the tool
		if acc, exists := s.accumulator.toolCalls[index]; exists {
			return types.ContentBlockStopEvent{
				Type:  "content_block_stop",
				Index: acc.blockIndex,
			}, true
		}
	}

	return nil, false
}

// getNextBlockIndex returns the next available block index.
func (s *eventStream) getNextBlockIndex() int {
	maxIndex := -1
	if s.accumulator.textContent.Len() > 0 {
		maxIndex = s.accumulator.textBlockIndex
	}
	for _, tc := range s.accumulator.toolCalls {
		if tc.blockIndex > maxIndex {
			maxIndex = tc.blockIndex
		}
	}
	return maxIndex + 1
}

// buildFinalEvent builds the final events when stream ends.
func (s *eventStream) buildFinalEvent() (types.StreamEvent, error) {
	s.finished = true

	stopReason := types.StopReasonEndTurn
	if s.accumulator.finishReason == "tool_calls" {
		stopReason = types.StopReasonToolUse
	}

	// Queue message_stop for next call - will return EOF after this
	s.pending = append(s.pending, types.MessageStopEvent{
		Type: "message_stop",
	})

	// Return message_delta with stop reason and usage
	// Return nil error to allow pending events to be delivered
	return types.MessageDeltaEvent{
		Type: "message_delta",
		Delta: struct {
			StopReason types.StopReason `json:"stop_reason,omitempty"`
		}{
			StopReason: stopReason,
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
