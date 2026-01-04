// Package stt provides speech-to-text functionality.
package stt

import (
	"context"
	"io"
)

// Provider is the interface for speech-to-text services.
type Provider interface {
	// Name returns the provider identifier.
	Name() string

	// Transcribe converts audio to text.
	Transcribe(ctx context.Context, audio io.Reader, opts TranscribeOptions) (*Transcript, error)

	// TranscribeStream transcribes streaming audio.
	// Returns a channel that emits transcript updates.
	TranscribeStream(ctx context.Context, audio io.Reader, opts TranscribeOptions) (<-chan TranscriptDelta, error)

	// NewStreamingSTT creates a new streaming STT session via WebSocket.
	// This provides more control over the streaming process than TranscribeStream.
	NewStreamingSTT(ctx context.Context, opts TranscribeOptions) (*StreamingSTT, error)
}

// TranscribeOptions configures transcription.
type TranscribeOptions struct {
	Model      string // Provider-specific model (default: "ink-whisper")
	Language   string // ISO language code (default: "en")
	Format     string // Audio format hint (wav, mp3, webm, etc.)
	SampleRate int    // Audio sample rate in Hz
	Timestamps bool   // Include word-level timestamps
}

// Transcript is the result of transcription.
type Transcript struct {
	Text     string  // Full transcribed text
	Language string  // Detected or specified language
	Duration float64 // Audio duration in seconds
	Words    []Word  // Word-level details (if timestamps requested)
}

// Word represents a single transcribed word with timing.
type Word struct {
	Word  string  // The word
	Start float64 // Start time in seconds
	End   float64 // End time in seconds
}

// TranscriptDelta is a streaming transcript update.
type TranscriptDelta struct {
	Text      string  // Partial transcript
	IsFinal   bool    // True if this is a final segment
	Timestamp float64 // Timestamp in seconds
}
