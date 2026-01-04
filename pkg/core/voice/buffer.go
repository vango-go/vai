package voice

import (
	"strings"
)

// SentenceBuffer accumulates text and extracts complete sentences.
// This enables low-latency TTS by synthesizing sentences as they complete.
type SentenceBuffer struct {
	buffer strings.Builder
}

// NewSentenceBuffer creates a new sentence buffer.
func NewSentenceBuffer() *SentenceBuffer {
	return &SentenceBuffer{}
}

// Add adds text to the buffer and returns any complete sentences.
func (b *SentenceBuffer) Add(text string) []string {
	b.buffer.WriteString(text)

	content := b.buffer.String()
	var sentences []string

	// Find sentence boundaries
	lastEnd := 0
	for i := 0; i < len(content); i++ {
		if isSentenceEnd(content, i) {
			sentence := strings.TrimSpace(content[lastEnd : i+1])
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			lastEnd = i + 1
		}
	}

	// Keep remainder in buffer
	if lastEnd > 0 {
		b.buffer.Reset()
		b.buffer.WriteString(content[lastEnd:])
	}

	return sentences
}

// Flush returns any remaining text and clears the buffer.
func (b *SentenceBuffer) Flush() string {
	result := strings.TrimSpace(b.buffer.String())
	b.buffer.Reset()
	return result
}

// Pending returns the current pending text without clearing.
func (b *SentenceBuffer) Pending() string {
	return b.buffer.String()
}

// isSentenceEnd checks if position i is a sentence boundary.
func isSentenceEnd(s string, i int) bool {
	if i >= len(s) {
		return false
	}

	c := s[i]
	if c != '.' && c != '!' && c != '?' {
		return false
	}

	// Check it's not an abbreviation (Dr., Mr., etc.)
	if c == '.' && isAbbreviation(s, i) {
		return false
	}

	// Check there's whitespace or end of string after
	if i+1 < len(s) && s[i+1] != ' ' && s[i+1] != '\n' && s[i+1] != '\r' && s[i+1] != '\t' {
		return false
	}

	return true
}

// isAbbreviation checks if the period at position i is likely an abbreviation.
func isAbbreviation(s string, i int) bool {
	if i < 1 {
		return false
	}

	// Common abbreviations
	commonAbbreviations := []string{
		"Dr.", "Mr.", "Mrs.", "Ms.", "Jr.", "Sr.",
		"Prof.", "Rev.", "Gen.", "Col.", "Lt.", "Sgt.",
		"Inc.", "Ltd.", "Corp.", "Co.", "vs.", "etc.",
		"i.e.", "e.g.", "a.m.", "p.m.", "U.S.", "U.K.",
	}

	// Get the word ending at i (including the period)
	start := i
	for start > 0 && s[start-1] != ' ' && s[start-1] != '\n' {
		start--
	}
	word := s[start : i+1]

	for _, abbr := range commonAbbreviations {
		if strings.EqualFold(word, abbr) {
			return true
		}
	}

	// Check if it's a single uppercase letter followed by period (initials)
	if i >= 1 && s[i-1] >= 'A' && s[i-1] <= 'Z' {
		if i < 2 || s[i-2] == ' ' || s[i-2] == '\n' {
			return true
		}
	}

	return false
}
