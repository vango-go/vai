package vai

import (
	"fmt"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// StreamCallbacks defines handlers for stream events.
// All callbacks are optional - nil callbacks are skipped.
type StreamCallbacks struct {
	// Content deltas
	OnTextDelta     func(text string)     // Text content as it streams
	OnThinkingDelta func(thinking string) // Claude's reasoning process (extended thinking)
	OnAudioChunk    func(data []byte)     // Audio data chunks

	// Tools (for RunStream)
	OnToolCallStart func(id, name string, input map[string]any)        // Tool execution starting
	OnToolResult    func(id, name string, content []types.ContentBlock, err error) // Tool execution complete

	// Lifecycle (for RunStream)
	OnStepStart    func(index int)                    // New step beginning
	OnStepComplete func(index int, response *Response) // Step finished
	OnInterrupted  func(partialText string, behavior InterruptBehavior) // Stream was interrupted

	// Errors
	OnError func(err error)
}

// TextDeltaFrom extracts text from any event that contains a text delta.
// Returns the text and true if found, empty string and false otherwise.
func TextDeltaFrom(event RunStreamEvent) (string, bool) {
	switch e := event.(type) {
	case StreamEventWrapper:
		if delta, ok := e.Event.(types.ContentBlockDeltaEvent); ok {
			if text, ok := delta.Delta.(types.TextDelta); ok {
				return text.Text, true
			}
		}
	}
	return "", false
}

// ThinkingDeltaFrom extracts thinking content from thinking delta events.
// Returns the thinking text and true if found.
func ThinkingDeltaFrom(event RunStreamEvent) (string, bool) {
	switch e := event.(type) {
	case StreamEventWrapper:
		if delta, ok := e.Event.(types.ContentBlockDeltaEvent); ok {
			if thinking, ok := delta.Delta.(types.ThinkingDelta); ok {
				return thinking.Thinking, true
			}
		}
	}
	return "", false
}

// AudioChunkFrom extracts audio data from audio chunk events.
// Returns the audio bytes and true if found.
func AudioChunkFrom(event RunStreamEvent) ([]byte, bool) {
	if audio, ok := event.(AudioChunkEvent); ok {
		return audio.Data, true
	}
	return nil, false
}

// Process consumes the stream with callbacks and returns the accumulated text.
// This is a convenience method for the common pattern of handling stream events.
//
// Example:
//
//	text, err := stream.Process(vai.StreamCallbacks{
//	    OnTextDelta:  func(t string) { fmt.Print(t) },
//	    OnAudioChunk: func(d []byte) { player.Write(d) },
//	})
func (rs *RunStream) Process(callbacks StreamCallbacks) (string, error) {
	var text strings.Builder

	for event := range rs.Events() {
		switch e := event.(type) {
		case StreamEventWrapper:
			rs.processStreamEvent(e.Event, &text, callbacks)

		case AudioChunkEvent:
			if callbacks.OnAudioChunk != nil {
				callbacks.OnAudioChunk(e.Data)
			}

		case StepStartEvent:
			if callbacks.OnStepStart != nil {
				callbacks.OnStepStart(e.Index)
			}

		case StepCompleteEvent:
			if callbacks.OnStepComplete != nil {
				callbacks.OnStepComplete(e.Index, e.Response)
			}

		case ToolCallStartEvent:
			if callbacks.OnToolCallStart != nil {
				callbacks.OnToolCallStart(e.ID, e.Name, e.Input)
			}

		case ToolResultEvent:
			if callbacks.OnToolResult != nil {
				callbacks.OnToolResult(e.ID, e.Name, e.Content, e.Error)
			}

		case InterruptedEvent:
			if callbacks.OnInterrupted != nil {
				callbacks.OnInterrupted(e.PartialText, e.Behavior)
			}

		case RunCompleteEvent:
			// Run finished, loop will exit

		}
	}

	if err := rs.Err(); err != nil {
		if callbacks.OnError != nil {
			callbacks.OnError(err)
		}
		return text.String(), err
	}

	return text.String(), nil
}

// processStreamEvent handles the inner stream events (from the LLM).
func (rs *RunStream) processStreamEvent(event types.StreamEvent, text *strings.Builder, callbacks StreamCallbacks) {
	switch e := event.(type) {
	case types.ContentBlockDeltaEvent:
		switch delta := e.Delta.(type) {
		case types.TextDelta:
			text.WriteString(delta.Text)
			if callbacks.OnTextDelta != nil {
				callbacks.OnTextDelta(delta.Text)
			}
		case types.ThinkingDelta:
			if callbacks.OnThinkingDelta != nil {
				callbacks.OnThinkingDelta(delta.Thinking)
			}
		}

	case types.ErrorEvent:
		if callbacks.OnError != nil {
			callbacks.OnError(fmt.Errorf("%s: %s", e.Error.Type, e.Error.Message))
		}
	}
}
