package live

import (
	"time"

	"github.com/vango-go/vai/pkg/core/types"
)

// Event is the interface for all live session events.
type Event interface {
	// EventType returns the event type string for serialization.
	EventType() string
}

// SessionCreatedEvent is emitted when the session is successfully configured.
type SessionCreatedEvent struct {
	SessionID  string         `json:"session_id"`
	Config     *SessionConfig `json:"config"`
	SampleRate int            `json:"sample_rate"`
	Channels   int            `json:"channels"`
}

func (e *SessionCreatedEvent) EventType() string { return "session.created" }

// SessionUpdatedEvent is emitted when session configuration is updated mid-session.
type SessionUpdatedEvent struct {
	Config *SessionConfig `json:"config"`
}

func (e *SessionUpdatedEvent) EventType() string { return "session.updated" }

// SessionClosedEvent is emitted when the session ends.
type SessionClosedEvent struct {
	Reason string `json:"reason,omitempty"`
}

func (e *SessionClosedEvent) EventType() string { return "session.closed" }

// StateChangedEvent is emitted when the session state changes.
type StateChangedEvent struct {
	From SessionState `json:"from"`
	To   SessionState `json:"to"`
}

func (e *StateChangedEvent) EventType() string { return "state.changed" }

// VADListeningEvent is emitted when VAD enters listening mode.
type VADListeningEvent struct{}

func (e *VADListeningEvent) EventType() string { return "vad.listening" }

// VADAnalyzingEvent is emitted when VAD is performing semantic analysis.
type VADAnalyzingEvent struct {
	Transcript string `json:"transcript"`
}

func (e *VADAnalyzingEvent) EventType() string { return "vad.analyzing" }

// VADSilenceEvent is emitted when silence is detected.
type VADSilenceEvent struct {
	DurationMs int `json:"duration_ms"`
}

func (e *VADSilenceEvent) EventType() string { return "vad.silence" }

// VADCommittedEvent is emitted when VAD commits a turn.
type VADCommittedEvent struct {
	Transcript string `json:"transcript"`
	Forced     bool   `json:"forced,omitempty"` // True if max silence timeout forced commit
}

func (e *VADCommittedEvent) EventType() string { return "vad.committed" }

// TranscriptDeltaEvent is emitted as real-time transcription updates arrive.
type TranscriptDeltaEvent struct {
	Delta   string `json:"delta"`
	IsFinal bool   `json:"is_final,omitempty"`
}

func (e *TranscriptDeltaEvent) EventType() string { return "transcript.delta" }

// InputCommittedEvent is emitted when user input is finalized and ready for processing.
type InputCommittedEvent struct {
	Transcript string `json:"transcript"`
}

func (e *InputCommittedEvent) EventType() string { return "input.committed" }

// DiscreteInputReceivedEvent is emitted when a discrete text/content input is received.
// Discrete inputs bypass VAD and grace period - they are submitted as complete user turns.
type DiscreteInputReceivedEvent struct {
	Content []types.ContentBlock `json:"content"`
}

func (e *DiscreteInputReceivedEvent) EventType() string { return "input.discrete" }

// GracePeriodStartedEvent is emitted when the grace period begins.
type GracePeriodStartedEvent struct {
	Transcript string    `json:"transcript"`
	DurationMs int       `json:"duration_ms"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (e *GracePeriodStartedEvent) EventType() string { return "grace_period.started" }

// GracePeriodExtendedEvent is emitted when user speaks during grace period.
type GracePeriodExtendedEvent struct {
	PreviousTranscript string    `json:"previous_transcript"`
	NewTranscript      string    `json:"new_transcript"`
	DurationMs         int       `json:"duration_ms"`
	ExpiresAt          time.Time `json:"expires_at"`
}

func (e *GracePeriodExtendedEvent) EventType() string { return "grace_period.extended" }

// GracePeriodExpiredEvent is emitted when the grace period ends without continuation.
type GracePeriodExpiredEvent struct {
	Transcript string `json:"transcript"`
}

func (e *GracePeriodExpiredEvent) EventType() string { return "grace_period.expired" }

// MessageStartEvent is emitted when the agent starts generating a response.
type MessageStartEvent struct {
	Message *types.MessageResponse `json:"message"`
}

func (e *MessageStartEvent) EventType() string { return "message_start" }

// ContentBlockStartEvent is emitted when a content block starts.
type ContentBlockStartEvent struct {
	Index        int                `json:"index"`
	ContentBlock types.ContentBlock `json:"content_block"`
}

func (e *ContentBlockStartEvent) EventType() string { return "content_block_start" }

// ContentBlockDeltaEvent is emitted for incremental content updates.
type ContentBlockDeltaEvent struct {
	Index int    `json:"index"`
	Delta string `json:"delta"`
}

func (e *ContentBlockDeltaEvent) EventType() string { return "content_block_delta" }

// ContentBlockStopEvent is emitted when a content block completes.
type ContentBlockStopEvent struct {
	Index int `json:"index"`
}

func (e *ContentBlockStopEvent) EventType() string { return "content_block_stop" }

// MessageStopEvent is emitted when the agent finishes generating a response.
type MessageStopEvent struct{}

func (e *MessageStopEvent) EventType() string { return "message_stop" }

// ToolUseEvent is emitted when the agent invokes a tool.
type ToolUseEvent struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

func (e *ToolUseEvent) EventType() string { return "tool_use" }

// ToolResultEvent is emitted when a tool returns a result.
type ToolResultEvent struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

func (e *ToolResultEvent) EventType() string { return "tool_result" }

// AudioDeltaEvent is emitted for TTS audio chunks.
type AudioDeltaEvent struct {
	Data   []byte `json:"data"`
	Format string `json:"format,omitempty"` // e.g., "pcm_s16le"
}

func (e *AudioDeltaEvent) EventType() string { return "audio_delta" }

// AudioCommittedEvent is emitted when all TTS audio for a response is complete.
type AudioCommittedEvent struct {
	DurationMs int `json:"duration_ms"`
}

func (e *AudioCommittedEvent) EventType() string { return "audio.committed" }

// AudioFlushEvent signals that all pending/buffered audio should be discarded immediately.
// This is emitted when the user speaks during grace period (extending their turn)
// or when a real interrupt is confirmed. Clients should clear their audio buffers.
type AudioFlushEvent struct{}

func (e *AudioFlushEvent) EventType() string { return "audio.flush" }

// InterruptDetectingEvent is emitted when potential interrupt audio is detected.
type InterruptDetectingEvent struct{}

func (e *InterruptDetectingEvent) EventType() string { return "interrupt.detecting" }

// InterruptCapturedEvent is emitted after the capture window completes.
type InterruptCapturedEvent struct {
	Transcript string `json:"transcript"`
}

func (e *InterruptCapturedEvent) EventType() string { return "interrupt.captured" }

// InterruptDismissedEvent is emitted when detected audio is not a real interrupt.
type InterruptDismissedEvent struct {
	Transcript string `json:"transcript,omitempty"`
	Reason     string `json:"reason"` // "no_speech", "backchannel", "semantic_check"
}

func (e *InterruptDismissedEvent) EventType() string { return "interrupt.dismissed" }

// ResponseInterruptedEvent is emitted when a real interrupt is confirmed.
type ResponseInterruptedEvent struct {
	PartialText         string `json:"partial_text"`
	InterruptTranscript string `json:"interrupt_transcript"`
	AudioPositionMs     int    `json:"audio_position_ms"`
}

func (e *ResponseInterruptedEvent) EventType() string { return "response.interrupted" }

// TTSPausedEvent is emitted when TTS output is paused.
type TTSPausedEvent struct {
	PositionMs int `json:"position_ms"`
}

func (e *TTSPausedEvent) EventType() string { return "tts.paused" }

// TTSResumedEvent is emitted when TTS output is resumed.
type TTSResumedEvent struct {
	PositionMs int `json:"position_ms"`
}

func (e *TTSResumedEvent) EventType() string { return "tts.resumed" }

// TTSCancelledEvent is emitted when TTS output is cancelled.
type TTSCancelledEvent struct {
	PositionMs int `json:"position_ms"`
}

func (e *TTSCancelledEvent) EventType() string { return "tts.cancelled" }

// ErrorEvent is emitted when an error occurs.
type ErrorEvent struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorEvent) EventType() string { return "error" }

// DebugEvent is emitted for debugging information.
type DebugEvent struct {
	Category string `json:"category"` // AUDIO, STT, VAD, SEMANTIC, GRACE, LLM, TTS, INTERRUPT, SESSION
	Message  string `json:"message"`
}

func (e *DebugEvent) EventType() string { return "debug" }

// EnergyLevelEvent is emitted periodically with audio energy information.
type EnergyLevelEvent struct {
	RMS         float64 `json:"rms"`
	Peak        float64 `json:"peak"`
	IsSpeech    bool    `json:"is_speech"`
	DurationMs  int     `json:"duration_ms"` // Duration of audio analyzed
}

func (e *EnergyLevelEvent) EventType() string { return "energy.level" }
