package vai

import "github.com/vango-go/vai/pkg/core/types"

// DefaultHistoryHandler returns a handler that appends HistoryDeltaEvent messages
// to the provided message slice.
func DefaultHistoryHandler(messages *[]types.Message) func(RunStreamEvent) {
	return func(event RunStreamEvent) {
		if messages == nil {
			return
		}
		switch e := event.(type) {
		case HistoryDeltaEvent:
			if len(e.Append) == 0 {
				return
			}
			*messages = append(*messages, e.Append...)
		}
	}
}
