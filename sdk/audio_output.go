package vai

import (
	"sync"
)

// AudioOutputConfig configures audio output buffering behavior.
type AudioOutputConfig struct {
	// MinBufferMs is the minimum audio to buffer before emitting the first chunk.
	// This prevents audio glitches when the first TTS chunk is small.
	// Default: 50ms. Set to 0 to disable pre-buffering.
	MinBufferMs int

	// ChannelSize is the buffer size for the audio chunks channel.
	// Default: 20.
	ChannelSize int
}

// DefaultAudioOutputConfig returns the default audio output configuration.
func DefaultAudioOutputConfig() AudioOutputConfig {
	return AudioOutputConfig{
		MinBufferMs: 50,
		ChannelSize: 20,
	}
}

// AudioOutput manages audio streaming with built-in buffering.
// It handles the complexities of audio pre-buffering and flush signals
// so clients can focus on just playing audio.
//
// Usage:
//
//	audio := session.AudioOutput()
//	for {
//	    select {
//	    case chunk := <-audio.Chunks():
//	        player.Write(chunk)
//	    case <-audio.Flush():
//	        player.Clear()
//	    }
//	}
type AudioOutput struct {
	config     AudioOutputConfig
	sampleRate int

	chunks chan []byte
	flush  chan struct{}

	mu          sync.Mutex
	buffer      []byte
	bufferReady bool
	closed      bool
}

// NewAudioOutput creates a new AudioOutput with the given sample rate and config.
func NewAudioOutput(sampleRate int, config AudioOutputConfig) *AudioOutput {
	if config.MinBufferMs == 0 && config.ChannelSize == 0 {
		config = DefaultAudioOutputConfig()
	}
	if config.ChannelSize == 0 {
		config.ChannelSize = 20
	}

	return &AudioOutput{
		config:     config,
		sampleRate: sampleRate,
		chunks:     make(chan []byte, config.ChannelSize),
		flush:      make(chan struct{}, 1),
	}
}

// Chunks returns a channel that emits audio chunks ready for playback.
// Audio is pre-buffered according to MinBufferMs before the first chunk is emitted.
// After each flush, pre-buffering resets for the next audio stream.
func (a *AudioOutput) Chunks() <-chan []byte {
	return a.chunks
}

// Flush returns a channel that signals when the client should clear its audio buffer.
// This happens when:
// - User speaks during grace period (continuing their turn)
// - User interrupts during agent speech
//
// When this signal is received, clients should immediately clear/stop their audio player
// to prevent stale audio from playing.
func (a *AudioOutput) Flush() <-chan struct{} {
	return a.flush
}

// HandleAudio is a convenience method that processes audio in a goroutine.
// It calls onChunk for each audio chunk and onFlush when audio should be cleared.
// Returns immediately; processing happens in the background.
//
// Usage:
//
//	session.AudioOutput().HandleAudio(
//	    func(data []byte) { player.Write(data) },
//	    func() { player.Clear() },
//	)
func (a *AudioOutput) HandleAudio(onChunk func([]byte), onFlush func()) {
	go func() {
		for {
			select {
			case chunk, ok := <-a.chunks:
				if !ok {
					return
				}
				if onChunk != nil {
					onChunk(chunk)
				}
			case _, ok := <-a.flush:
				if !ok {
					return
				}
				if onFlush != nil {
					onFlush()
				}
			}
		}
	}()
}

// Close closes the AudioOutput channels.
func (a *AudioOutput) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}
	a.closed = true
	close(a.chunks)
	close(a.flush)
}

// pushAudio is called internally when audio data arrives.
// It handles pre-buffering before emitting chunks.
func (a *AudioOutput) pushAudio(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return
	}

	a.buffer = append(a.buffer, data...)

	// Calculate min buffer size in bytes
	// At 16-bit mono: bytes = sampleRate * 2 * (ms / 1000)
	minBytes := (a.sampleRate * 2 * a.config.MinBufferMs) / 1000

	// Check if we've buffered enough to start emitting
	if !a.bufferReady && len(a.buffer) >= minBytes {
		a.bufferReady = true
	}

	// Emit buffered audio if ready
	if a.bufferReady && len(a.buffer) > 0 {
		chunk := a.buffer
		a.buffer = nil
		select {
		case a.chunks <- chunk:
		default:
			// Channel full - shouldn't happen in normal operation
			// Put the data back
			a.buffer = chunk
		}
	}
}

// doFlush is called internally when a flush event occurs.
// It clears internal buffers and signals the client.
func (a *AudioOutput) doFlush() {
	a.mu.Lock()

	if a.closed {
		a.mu.Unlock()
		return
	}

	// Clear internal buffer and reset pre-buffering state
	a.buffer = nil
	a.bufferReady = false
	a.mu.Unlock()

	// Drain any pending chunks in the channel
	for {
		select {
		case <-a.chunks:
		default:
			goto done
		}
	}
done:

	// Signal client to flush their player
	select {
	case a.flush <- struct{}{}:
	default:
		// Already have a pending flush signal
	}
}
