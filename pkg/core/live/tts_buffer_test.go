package live

import (
	"testing"
)

func TestTTSBuffer_Punctuation(t *testing.T) {
	b := NewTTSBuffer()

	// Punctuation should trigger immediate send
	tests := []struct {
		delta    string
		expected string
	}{
		{"Hello", ""},
		{" world", ""},
		{"!", "Hello world!"},
	}

	for _, tt := range tests {
		result := b.Add(tt.delta)
		if result != tt.expected {
			t.Errorf("Add(%q) = %q, want %q", tt.delta, result, tt.expected)
		}
	}
}

func TestTTSBuffer_WordCount(t *testing.T) {
	b := NewTTSBuffer()

	// Should send after 5 words when at word boundary
	deltas := []string{
		"The",      // 1 word
		" bird",    // 2 words
		" was",     // 3 words
		" chirp",   // 4 words (incomplete)
		"ing",      // still 4 words (completed "chirping")
		" loudly",  // 5 words
		" today",   // 6 words - should trigger send of previous 5
	}

	var results []string
	for _, d := range deltas {
		if r := b.Add(d); r != "" {
			results = append(results, r)
		}
	}

	// Should have sent "The bird was chirping loudly" (5 words)
	if len(results) != 1 {
		t.Fatalf("expected 1 send, got %d: %v", len(results), results)
	}
	if results[0] != "The bird was chirping loudly" {
		t.Errorf("expected 'The bird was chirping loudly', got %q", results[0])
	}

	// Flush should return remaining
	remainder := b.Flush()
	if remainder != "today" {
		t.Errorf("expected 'today' in buffer, got %q", remainder)
	}
}

func TestTTSBuffer_PartialWords(t *testing.T) {
	b := NewTTSBuffer()

	// Test that partial words aren't split
	deltas := []string{
		"The",
		" bird",
		" was",
		" chirp", // partial word
		"ing",    // completes "chirping"
		" loud",  // partial word
		"ly",     // completes "loudly" - now 5 words but no space yet
		" in",    // space confirms "loudly" complete, triggers send
	}

	var results []string
	for _, d := range deltas {
		if r := b.Add(d); r != "" {
			results = append(results, r)
		}
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 send, got %d: %v", len(results), results)
	}
	if results[0] != "The bird was chirping loudly" {
		t.Errorf("expected 'The bird was chirping loudly', got %q", results[0])
	}
}

func TestTTSBuffer_CommaPunctuation(t *testing.T) {
	b := NewTTSBuffer()

	// Comma should also trigger send
	deltas := []string{"Hello", ",", " how"}

	var results []string
	for _, d := range deltas {
		if r := b.Add(d); r != "" {
			results = append(results, r)
		}
	}

	if len(results) != 1 || results[0] != "Hello," {
		t.Errorf("expected ['Hello,'], got %v", results)
	}

	remainder := b.Flush()
	if remainder != "how" {
		t.Errorf("expected 'how' remaining, got %q", remainder)
	}
}

func TestTTSBuffer_MixedPunctuationAndWords(t *testing.T) {
	b := NewTTSBuffer()

	// Realistic LLM output
	deltas := []string{
		"Hey",
		" there",
		"!",
		" How",
		"'s",
		" it",
		" going",
		"?",
	}

	var results []string
	for _, d := range deltas {
		if r := b.Add(d); r != "" {
			results = append(results, r)
		}
	}

	expected := []string{"Hey there!", "How's it going?"}
	if len(results) != len(expected) {
		t.Fatalf("expected %d sends, got %d: %v", len(expected), len(results), results)
	}
	for i, e := range expected {
		if results[i] != e {
			t.Errorf("result[%d] = %q, want %q", i, results[i], e)
		}
	}
}

func TestTTSBuffer_LongSentenceNoPunctuation(t *testing.T) {
	b := NewTTSBuffer()

	// Long sentence without punctuation - should chunk at 5 words
	deltas := []string{
		"I", " think", " that", " we", " should",
		" probably", " go", " to", " the", " store",
		" and", " buy", " some", " groceries", " today",
	}

	var results []string
	for _, d := range deltas {
		if r := b.Add(d); r != "" {
			results = append(results, r)
		}
	}

	// Should send chunks of ~5 words
	// "I think that we should" (5) -> sent when " probably" arrives
	// "probably go to the store" (5) -> sent when " and" arrives
	// "and buy some groceries today" -> in buffer

	if len(results) != 2 {
		t.Fatalf("expected 2 sends, got %d: %v", len(results), results)
	}

	remainder := b.Flush()
	if remainder == "" {
		t.Error("expected some remainder")
	}
}
