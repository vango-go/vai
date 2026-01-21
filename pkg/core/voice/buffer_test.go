package voice

import (
	"testing"
)

func TestSentenceBuffer_Add_SingleSentence(t *testing.T) {
	b := NewSentenceBuffer()

	sentences := b.Add("Hello world. ")
	if len(sentences) != 1 {
		t.Errorf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0] != "Hello world." {
		t.Errorf("expected 'Hello world.', got %q", sentences[0])
	}
}

func TestSentenceBuffer_Add_MultipleSentences(t *testing.T) {
	b := NewSentenceBuffer()

	sentences := b.Add("First sentence. Second sentence! Third? ")
	if len(sentences) != 3 {
		t.Errorf("expected 3 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestSentenceBuffer_Add_Partial(t *testing.T) {
	b := NewSentenceBuffer()

	// Add partial text
	sentences := b.Add("Hello wo")
	if len(sentences) != 0 {
		t.Errorf("expected 0 sentences for partial, got %d", len(sentences))
	}

	// Complete the sentence
	sentences = b.Add("rld. ")
	if len(sentences) != 1 {
		t.Errorf("expected 1 sentence, got %d", len(sentences))
	}
	if sentences[0] != "Hello world." {
		t.Errorf("expected 'Hello world.', got %q", sentences[0])
	}
}

func TestSentenceBuffer_Add_StreamingChunks(t *testing.T) {
	b := NewSentenceBuffer()

	// Simulate streaming chunks
	var allSentences []string
	chunks := []string{"The ", "quick ", "brown ", "fox. ", "Jumps ", "over. "}

	for _, chunk := range chunks {
		sentences := b.Add(chunk)
		allSentences = append(allSentences, sentences...)
	}

	if len(allSentences) != 2 {
		t.Errorf("expected 2 sentences, got %d: %v", len(allSentences), allSentences)
	}
}

func TestSentenceBuffer_Flush(t *testing.T) {
	b := NewSentenceBuffer()

	b.Add("Incomplete sentence without period")
	remaining := b.Flush()

	if remaining != "Incomplete sentence without period" {
		t.Errorf("expected remaining text, got %q", remaining)
	}

	// Buffer should be empty now
	if b.Pending() != "" {
		t.Errorf("expected empty buffer after flush, got %q", b.Pending())
	}
}

func TestSentenceBuffer_Abbreviations(t *testing.T) {
	b := NewSentenceBuffer()

	// Common abbreviations should not end sentences
	sentences := b.Add("Dr. Smith went to the store. ")
	if len(sentences) != 1 {
		t.Errorf("expected 1 sentence (Dr. should not split), got %d: %v", len(sentences), sentences)
	}

	b = NewSentenceBuffer()
	sentences = b.Add("Mr. Jones is here. ")
	if len(sentences) != 1 {
		t.Errorf("expected 1 sentence (Mr. should not split), got %d: %v", len(sentences), sentences)
	}
}

func TestSentenceBuffer_QuestionMark(t *testing.T) {
	b := NewSentenceBuffer()

	sentences := b.Add("How are you? I am fine. ")
	if len(sentences) != 2 {
		t.Errorf("expected 2 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestSentenceBuffer_ExclamationMark(t *testing.T) {
	b := NewSentenceBuffer()

	sentences := b.Add("Hello! Welcome! ")
	if len(sentences) != 2 {
		t.Errorf("expected 2 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestSentenceBuffer_EmptyInput(t *testing.T) {
	b := NewSentenceBuffer()

	sentences := b.Add("")
	if len(sentences) != 0 {
		t.Errorf("expected 0 sentences for empty input, got %d", len(sentences))
	}

	remaining := b.Flush()
	if remaining != "" {
		t.Errorf("expected empty flush, got %q", remaining)
	}
}

func TestSentenceBuffer_Pending(t *testing.T) {
	b := NewSentenceBuffer()

	b.Add("Partial text")
	if b.Pending() != "Partial text" {
		t.Errorf("expected pending text, got %q", b.Pending())
	}

	b.Add(" more text. Done")
	// After the sentence, " Done" (with leading space) should be pending
	// Flush trims whitespace, but Pending returns raw content
	pending := b.Pending()
	flushed := b.Flush()
	if flushed != "Done" {
		t.Errorf("expected 'Done' from flush, got %q (pending was %q)", flushed, pending)
	}
}
