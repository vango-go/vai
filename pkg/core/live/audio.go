package live

import (
	"math"
	"sync"
)

// CalculateRMSEnergy computes the root-mean-square energy of PCM audio.
// Input is assumed to be 16-bit signed little-endian PCM.
// Returns a value between 0.0 and 1.0.
func CalculateRMSEnergy(pcm []byte) float64 {
	samples := len(pcm) / 2
	if samples == 0 {
		return 0
	}

	var sum float64
	for i := 0; i < len(pcm)-1; i += 2 {
		// Little-endian 16-bit signed integer
		sample := int16(pcm[i]) | int16(pcm[i+1])<<8
		// Normalize to -1.0 to 1.0
		normalized := float64(sample) / 32768.0
		sum += normalized * normalized
	}

	return math.Sqrt(sum / float64(samples))
}

// CalculatePeakAmplitude returns the maximum absolute amplitude in the PCM data.
// Returns a value between 0.0 and 1.0.
func CalculatePeakAmplitude(pcm []byte) float64 {
	if len(pcm) < 2 {
		return 0
	}

	var maxAbs float64
	for i := 0; i < len(pcm)-1; i += 2 {
		sample := int16(pcm[i]) | int16(pcm[i+1])<<8
		// Use float64 to avoid overflow when negating -32768
		abs := math.Abs(float64(sample))
		if abs > maxAbs {
			maxAbs = abs
		}
	}

	return maxAbs / 32768.0
}

// AudioBuffer accumulates PCM audio chunks with a configurable maximum size.
type AudioBuffer struct {
	mu       sync.Mutex
	data     []byte
	maxBytes int
	config   AudioConfig
}

// NewAudioBuffer creates a buffer that holds up to maxDurationMs of audio.
func NewAudioBuffer(config AudioConfig, maxDurationMs int) *AudioBuffer {
	maxBytes := config.BytesForDurationMs(maxDurationMs)
	return &AudioBuffer{
		data:     make([]byte, 0, maxBytes),
		maxBytes: maxBytes,
		config:   config,
	}
}

// Write appends audio data to the buffer.
// If the buffer would exceed maxBytes, older data is discarded.
func (b *AudioBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, data...)

	// Trim from the beginning if we exceed max size
	if len(b.data) > b.maxBytes {
		excess := len(b.data) - b.maxBytes
		b.data = b.data[excess:]
	}
}

// Read returns a copy of all buffered audio data.
func (b *AudioBuffer) Read() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]byte, len(b.data))
	copy(result, b.data)
	return result
}

// ReadLast returns the last durationMs of audio.
func (b *AudioBuffer) ReadLast(durationMs int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	bytes := b.config.BytesForDurationMs(durationMs)
	if bytes > len(b.data) {
		bytes = len(b.data)
	}

	result := make([]byte, bytes)
	copy(result, b.data[len(b.data)-bytes:])
	return result
}

// Len returns the current buffer size in bytes.
func (b *AudioBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data)
}

// DurationMs returns the current buffer duration in milliseconds.
func (b *AudioBuffer) DurationMs() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.config.DurationMs(len(b.data))
}

// Clear empties the buffer.
func (b *AudioBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = b.data[:0]
}

// RMSEnergy calculates the RMS energy of the buffered audio.
func (b *AudioBuffer) RMSEnergy() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return CalculateRMSEnergy(b.data)
}

// PeakAmplitude returns the peak amplitude of the buffered audio.
func (b *AudioBuffer) PeakAmplitude() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return CalculatePeakAmplitude(b.data)
}

// RingBuffer is a fixed-size circular buffer for audio data.
// It automatically overwrites old data when full.
type RingBuffer struct {
	mu       sync.Mutex
	data     []byte
	size     int
	writePos int
	filled   int // How much of the buffer has been written to
}

// NewRingBuffer creates a ring buffer that holds exactly durationMs of audio.
func NewRingBuffer(config AudioConfig, durationMs int) *RingBuffer {
	size := config.BytesForDurationMs(durationMs)
	return &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
}

// Write adds data to the ring buffer, overwriting old data if necessary.
func (r *RingBuffer) Write(data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, b := range data {
		r.data[r.writePos] = b
		r.writePos = (r.writePos + 1) % r.size
		if r.filled < r.size {
			r.filled++
		}
	}
}

// Read returns all data in the buffer in chronological order.
func (r *RingBuffer) Read() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.filled < r.size {
		// Buffer not yet full, return from start
		result := make([]byte, r.filled)
		copy(result, r.data[:r.filled])
		return result
	}

	// Buffer is full, need to reorder
	result := make([]byte, r.size)
	// Copy from writePos to end
	firstPart := r.size - r.writePos
	copy(result[:firstPart], r.data[r.writePos:])
	// Copy from start to writePos
	copy(result[firstPart:], r.data[:r.writePos])
	return result
}

// Clear resets the ring buffer.
func (r *RingBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writePos = 0
	r.filled = 0
}

// Filled returns how many bytes have been written.
func (r *RingBuffer) Filled() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.filled
}
