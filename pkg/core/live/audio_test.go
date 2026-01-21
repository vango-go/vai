package live

import (
	"math"
	"testing"
)

func TestCalculateRMSEnergy(t *testing.T) {
	tests := []struct {
		name     string
		samples  []int16
		expected float64
	}{
		{
			name:     "silence",
			samples:  []int16{0, 0, 0, 0},
			expected: 0.0,
		},
		{
			name:     "max amplitude",
			samples:  []int16{32767, 32767, 32767, 32767},
			expected: 1.0,
		},
		{
			name:     "half amplitude",
			samples:  []int16{16384, 16384, 16384, 16384},
			expected: 0.5,
		},
		{
			name:     "mixed signal",
			samples:  []int16{16384, -16384, 16384, -16384},
			expected: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert samples to PCM bytes
			pcm := make([]byte, len(tt.samples)*2)
			for i, s := range tt.samples {
				pcm[i*2] = byte(s & 0xFF)
				pcm[i*2+1] = byte((s >> 8) & 0xFF)
			}

			result := CalculateRMSEnergy(pcm)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("expected RMS %.3f, got %.3f", tt.expected, result)
			}
		})
	}
}

func TestCalculatePeakAmplitude(t *testing.T) {
	tests := []struct {
		name     string
		samples  []int16
		expected float64
	}{
		{
			name:     "silence",
			samples:  []int16{0, 0, 0, 0},
			expected: 0.0,
		},
		{
			name:     "positive peak",
			samples:  []int16{0, 16384, 0, 0},
			expected: 0.5,
		},
		{
			name:     "negative peak",
			samples:  []int16{0, -32768, 0, 0},
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pcm := make([]byte, len(tt.samples)*2)
			for i, s := range tt.samples {
				pcm[i*2] = byte(s & 0xFF)
				pcm[i*2+1] = byte((s >> 8) & 0xFF)
			}

			result := CalculatePeakAmplitude(pcm)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("expected peak %.3f, got %.3f", tt.expected, result)
			}
		})
	}
}

func TestAudioConfig(t *testing.T) {
	cfg := DefaultAudioConfig()

	// 24kHz, mono, 16-bit = 48000 bytes/second
	if cfg.BytesPerSecond() != 48000 {
		t.Errorf("expected 48000 bytes/sec, got %d", cfg.BytesPerSecond())
	}

	// 1000ms = 48000 bytes
	if cfg.BytesForDurationMs(1000) != 48000 {
		t.Errorf("expected 48000 bytes for 1s, got %d", cfg.BytesForDurationMs(1000))
	}

	// 48000 bytes = 1000ms
	if cfg.DurationMs(48000) != 1000 {
		t.Errorf("expected 1000ms for 48000 bytes, got %d", cfg.DurationMs(48000))
	}
}

func TestAudioBuffer(t *testing.T) {
	cfg := DefaultAudioConfig()
	buf := NewAudioBuffer(cfg, 100) // 100ms buffer

	// Write 50ms of audio
	data50ms := make([]byte, cfg.BytesForDurationMs(50))
	for i := range data50ms {
		data50ms[i] = byte(i % 256)
	}
	buf.Write(data50ms)

	if buf.DurationMs() != 50 {
		t.Errorf("expected 50ms, got %dms", buf.DurationMs())
	}

	// Write another 100ms (should trim to 100ms total)
	data100ms := make([]byte, cfg.BytesForDurationMs(100))
	buf.Write(data100ms)

	if buf.DurationMs() != 100 {
		t.Errorf("expected 100ms (capped), got %dms", buf.DurationMs())
	}

	// Clear
	buf.Clear()
	if buf.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", buf.Len())
	}
}

func TestRingBuffer(t *testing.T) {
	cfg := DefaultAudioConfig()
	ring := NewRingBuffer(cfg, 100) // 100ms

	// Write 50ms
	data50ms := make([]byte, cfg.BytesForDurationMs(50))
	for i := range data50ms {
		data50ms[i] = byte(i % 256)
	}
	ring.Write(data50ms)

	if ring.Filled() != len(data50ms) {
		t.Errorf("expected %d filled, got %d", len(data50ms), ring.Filled())
	}

	// Read should return exactly what we wrote
	read := ring.Read()
	if len(read) != len(data50ms) {
		t.Errorf("expected %d bytes, got %d", len(data50ms), len(read))
	}

	// Write 100ms more (should wrap around)
	data100ms := make([]byte, cfg.BytesForDurationMs(100))
	for i := range data100ms {
		data100ms[i] = byte((i + 100) % 256)
	}
	ring.Write(data100ms)

	// Should now be full (100ms = size)
	read = ring.Read()
	expectedSize := cfg.BytesForDurationMs(100)
	if len(read) != expectedSize {
		t.Errorf("expected %d bytes (full), got %d", expectedSize, len(read))
	}

	// Clear
	ring.Clear()
	if ring.Filled() != 0 {
		t.Errorf("expected 0 filled after clear, got %d", ring.Filled())
	}
}
