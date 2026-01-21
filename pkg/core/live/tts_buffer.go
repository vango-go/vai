package live

import (
	"strings"
	"sync"
)

// TTSBuffer accumulates LLM text deltas and emits chunks suitable for TTS.
// It sends text on:
// 1. Punctuation: . , ! ?
// 2. Word count threshold (5 words) when at a word boundary
type TTSBuffer struct {
	mu          sync.Mutex
	text        strings.Builder
	minWords    int
	punctuation string
}

// NewTTSBuffer creates a new TTS buffer with default settings.
func NewTTSBuffer() *TTSBuffer {
	return &TTSBuffer{
		minWords:    5,
		punctuation: ",.!?",
	}
}

// Add adds a text delta and returns text to send to TTS (if any).
// Returns empty string if more text should be buffered.
func (b *TTSBuffer) Add(delta string) string {
	if delta == "" {
		return ""
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if delta starts with space (confirms previous word is complete)
	startsWithSpace := delta[0] == ' ' || delta[0] == '\n'

	// Get previous buffer state before adding
	prevContent := b.text.String()
	prevWordCount := len(strings.Fields(prevContent))

	// Add delta to buffer
	b.text.WriteString(delta)
	content := b.text.String()

	// Priority 1: Punctuation triggers immediate send
	if strings.ContainsAny(delta, b.punctuation) {
		// Find last punctuation in buffer
		lastPunct := strings.LastIndexAny(content, b.punctuation)
		if lastPunct >= 0 {
			toSend := strings.TrimSpace(content[:lastPunct+1])
			remainder := strings.TrimSpace(content[lastPunct+1:])
			b.text.Reset()
			if remainder != "" {
				b.text.WriteString(remainder)
			}
			return toSend
		}
	}

	// Priority 2: Word count threshold + confirmed word boundary
	// Send if previous buffer had 5+ words AND this delta starts with space
	if prevWordCount >= b.minWords && startsWithSpace {
		toSend := strings.TrimSpace(prevContent)
		b.text.Reset()
		// Keep the new delta (without leading space) in buffer
		b.text.WriteString(strings.TrimLeft(delta, " \n"))
		return toSend
	}

	return ""
}

// Flush returns any remaining buffered text and resets the buffer.
// Call this when the LLM stream ends.
func (b *TTSBuffer) Flush() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := strings.TrimSpace(b.text.String())
	b.text.Reset()
	return result
}

// Reset clears the buffer without returning content.
// Call this on interrupt/cancellation.
func (b *TTSBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.text.Reset()
}

// Len returns the current buffer length.
func (b *TTSBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Len()
}
