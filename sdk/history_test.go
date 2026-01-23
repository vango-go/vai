package vai

import (
	"testing"

	"github.com/vango-go/vai/pkg/core/types"
)

func TestDefaultHistoryHandler(t *testing.T) {
	msgs := []types.Message{{Role: "user", Content: []types.ContentBlock{types.TextBlock{Type: "text", Text: "hi"}}}}
	handler := DefaultHistoryHandler(&msgs)

	delta := HistoryDeltaEvent{Append: []types.Message{
		{Role: "assistant", Content: []types.ContentBlock{types.TextBlock{Type: "text", Text: "hello"}}},
	}}
	handler(delta)

	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}

	handler(StepCompleteEvent{Index: 0, Response: nil})
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d after non-delta event, want 2", len(msgs))
	}
}
