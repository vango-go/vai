// Package tts provides text-to-speech functionality.
package tts

import (
	"context"
	"sync"
	"sync/atomic"
)

// Provider is the interface for text-to-speech services.
type Provider interface {
	// Name returns the provider identifier.
	Name() string

	// Synthesize converts text to audio.
	Synthesize(ctx context.Context, text string, opts SynthesizeOptions) (*Synthesis, error)

	// SynthesizeStream converts text to streaming audio.
	SynthesizeStream(ctx context.Context, text string, opts SynthesizeOptions) (*SynthesisStream, error)

	// NewStreamingContext creates a context for incremental text streaming.
	// Text can be sent in chunks, and audio is streamed back as it's generated.
	NewStreamingContext(ctx context.Context, opts StreamingContextOptions) (*StreamingContext, error)
}

// StreamingContextOptions configures a streaming context.
type StreamingContextOptions struct {
	Voice            string  // Voice identifier
	Speed            float64 // Speed multiplier (0.6-1.5)
	Volume           float64 // Volume multiplier (0.5-2.0)
	Emotion          string  // Emotion hint
	Language         string  // Language code
	Format           string  // Output format: "wav", "mp3", or "pcm"
	SampleRate       int     // Sample rate
	MaxBufferDelayMs int     // Max time to buffer text before generating (0-5000ms, default 500)
}

// StreamingContext manages an incremental TTS session.
// Text can be sent in chunks via SendText(), and audio chunks are received via Audio().
type StreamingContext struct {
	audio     chan []byte
	err       error
	errMu     sync.Mutex
	done      chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once

	// For implementations to use
	SendFunc  func(text string, isFinal bool) error
	CloseFunc func() error
}

// NewStreamingContext creates a new streaming context.
func NewStreamingContext() *StreamingContext {
	return &StreamingContext{
		audio: make(chan []byte, 100),
		done:  make(chan struct{}),
	}
}

// SendText sends a text chunk to be synthesized.
// Set isFinal=true for the last chunk to signal completion.
func (sc *StreamingContext) SendText(text string, isFinal bool) error {
	if sc.closed.Load() {
		return ErrContextClosed
	}
	if sc.SendFunc != nil {
		return sc.SendFunc(text, isFinal)
	}
	return nil
}

// Flush signals that all text has been sent and generation should complete.
func (sc *StreamingContext) Flush() error {
	return sc.SendText("", true)
}

// Audio returns the channel of audio chunks.
func (sc *StreamingContext) Audio() <-chan []byte {
	return sc.audio
}

// Err returns any error that occurred.
func (sc *StreamingContext) Err() error {
	sc.errMu.Lock()
	defer sc.errMu.Unlock()
	return sc.err
}

// Close closes the streaming context.
func (sc *StreamingContext) Close() error {
	var err error
	sc.closeOnce.Do(func() {
		sc.closed.Store(true)
		if sc.CloseFunc != nil {
			err = sc.CloseFunc()
		}
		close(sc.done)
	})
	return err
}

// Done returns a channel that's closed when the context is done.
func (sc *StreamingContext) Done() <-chan struct{} {
	return sc.done
}

// Internal methods for implementations

// PushAudio sends an audio chunk. Returns false if closed.
func (sc *StreamingContext) PushAudio(chunk []byte) bool {
	select {
	case sc.audio <- chunk:
		return true
	case <-sc.done:
		return false
	}
}

// SetError sets the context error.
func (sc *StreamingContext) SetError(err error) {
	sc.errMu.Lock()
	sc.err = err
	sc.errMu.Unlock()
}

// FinishAudio closes the audio channel.
func (sc *StreamingContext) FinishAudio() {
	close(sc.audio)
}

// ErrContextClosed is returned when sending to a closed context.
var ErrContextClosed = &contextClosedError{}

type contextClosedError struct{}

func (e *contextClosedError) Error() string { return "streaming context closed" }

// SynthesizeOptions configures synthesis.
type SynthesizeOptions struct {
	Voice      string  // Voice identifier (Cartesia voice ID)
	Speed      float64 // Speed multiplier (0.6-1.5, default 1.0)
	Volume     float64 // Volume multiplier (0.5-2.0, default 1.0)
	Emotion    string  // Emotion hint (neutral, happy, sad, angry, etc.)
	Language   string  // Language code
	Format     string  // Output format: "wav", "mp3", or "pcm"
	SampleRate int     // Sample rate: 8000, 16000, 22050, 24000, 44100, 48000
}

// Synthesis is the result of synthesis.
type Synthesis struct {
	Audio    []byte  // Audio data
	Format   string  // Audio format
	Duration float64 // Duration in seconds (if available)
}

// SynthesisStream provides streaming audio output.
type SynthesisStream struct {
	chunks chan []byte
	err    error
	done   chan struct{}
}

// NewSynthesisStream creates a new synthesis stream.
func NewSynthesisStream() *SynthesisStream {
	return &SynthesisStream{
		chunks: make(chan []byte, 100),
		done:   make(chan struct{}),
	}
}

// Chunks returns the channel of audio chunks.
func (s *SynthesisStream) Chunks() <-chan []byte {
	return s.chunks
}

// Err returns any error that occurred.
func (s *SynthesisStream) Err() error {
	<-s.done
	return s.err
}

// Close closes the stream.
func (s *SynthesisStream) Close() error {
	select {
	case <-s.done:
		// Already closed
	default:
		close(s.done)
	}
	return nil
}

// SetError sets the stream error.
func (s *SynthesisStream) SetError(err error) {
	s.err = err
}

// Send sends a chunk to the stream. Returns false if stream is closed.
func (s *SynthesisStream) Send(chunk []byte) bool {
	select {
	case s.chunks <- chunk:
		return true
	case <-s.done:
		return false
	}
}

// FinishSending closes the chunks channel to signal completion.
func (s *SynthesisStream) FinishSending() {
	close(s.chunks)
}
