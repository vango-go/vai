package vai

import (
	"sync/atomic"

	"github.com/vango-go/vai/pkg/core"
	"github.com/vango-go/vai/pkg/core/types"
)

// Stream wraps a streaming response from the Messages API.
type Stream struct {
	eventStream core.EventStream
	events      <-chan types.StreamEvent
	response    *types.MessageResponse
	err         error
	closed      atomic.Bool
	done        chan struct{}
}

// newStreamFromEventStream creates a Stream from a core.EventStream.
func newStreamFromEventStream(eventStream core.EventStream) *Stream {
	s := &Stream{
		eventStream: eventStream,
		done:        make(chan struct{}),
	}

	events := make(chan types.StreamEvent)
	s.events = events

	// Start goroutine to read events
	go s.readEvents(events)

	return s
}

// readEvents reads events from the core event stream and sends them to the channel.
func (s *Stream) readEvents(events chan<- types.StreamEvent) {
	defer close(events)
	defer close(s.done)

	var response types.MessageResponse
	var currentContent []types.ContentBlock

	for {
		event, err := s.eventStream.Next()
		if err != nil {
			// io.EOF means normal end
			s.err = err
			break
		}
		if event == nil {
			break
		}

		// Build up the response
		switch e := event.(type) {
		case types.MessageStartEvent:
			response = e.Message
		case types.ContentBlockStartEvent:
			// Ensure we have enough capacity
			for len(currentContent) <= e.Index {
				currentContent = append(currentContent, nil)
			}
			currentContent[e.Index] = e.ContentBlock
		case types.ContentBlockDeltaEvent:
			// Apply delta to current content block
			if e.Index < len(currentContent) {
				currentContent[e.Index] = applyDelta(currentContent[e.Index], e.Delta)
			}
		case types.MessageDeltaEvent:
			response.StopReason = e.Delta.StopReason
			response.Usage = e.Usage
		}

		// Don't block if stream is closed
		if s.closed.Load() {
			break
		}

		select {
		case events <- event:
		case <-s.done:
			return
		}
	}

	response.Content = currentContent
	s.response = &response
}

// applyDelta applies a delta to a content block.
func applyDelta(block types.ContentBlock, delta types.Delta) types.ContentBlock {
	switch d := delta.(type) {
	case types.TextDelta:
		if tb, ok := block.(types.TextBlock); ok {
			tb.Text += d.Text
			return tb
		}
	case types.InputJSONDelta:
		// For tool use blocks, accumulate the partial JSON
		if tu, ok := block.(types.ToolUseBlock); ok {
			// Store partial JSON in a way that can be parsed later
			// For now, just accumulate in a string field
			_ = tu
			_ = d.PartialJSON
		}
	case types.ThinkingDelta:
		if tb, ok := block.(types.ThinkingBlock); ok {
			tb.Thinking += d.Thinking
			return tb
		}
	}
	return block
}

// Events returns the channel of stream events.
func (s *Stream) Events() <-chan types.StreamEvent {
	return s.events
}

// Response returns the final response after the stream ends.
// This will block until the stream is complete.
func (s *Stream) Response() *types.MessageResponse {
	<-s.done
	return s.response
}

// Err returns any error that occurred during streaming.
func (s *Stream) Err() error {
	return s.err
}

// Close closes the stream and releases resources.
func (s *Stream) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		if s.eventStream != nil {
			return s.eventStream.Close()
		}
	}
	return nil
}

// TextContent returns the accumulated text content from the stream.
// This will block until the stream is complete.
func (s *Stream) TextContent() string {
	resp := s.Response()
	if resp == nil {
		return ""
	}
	return resp.TextContent()
}
