package anthropic

import (
	"bufio"
	"io"
	"strings"

	"github.com/vango-go/vai/pkg/core/types"
)

// eventStream implements core.EventStream for Anthropic SSE responses.
type eventStream struct {
	reader *bufio.Reader
	closer io.Closer
	err    error
}

// newEventStream creates a new event stream from an HTTP response body.
func newEventStream(body io.ReadCloser) *eventStream {
	return &eventStream{
		reader: bufio.NewReader(body),
		closer: body,
	}
}

// Next returns the next event from the stream.
// Returns nil, io.EOF when the stream is complete.
func (s *eventStream) Next() (types.StreamEvent, error) {
	if s.err != nil {
		return nil, s.err
	}

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil, io.EOF
			}
			s.err = err
			return nil, err
		}

		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse SSE format: "event: <type>" followed by "data: <json>"
		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")

			// Read the data line
			dataLine, err := s.reader.ReadString('\n')
			if err != nil {
				s.err = err
				return nil, err
			}
			dataLine = strings.TrimSpace(dataLine)

			if !strings.HasPrefix(dataLine, "data: ") {
				continue
			}

			data := strings.TrimPrefix(dataLine, "data: ")

			// Skip ping events
			if eventType == "ping" {
				continue
			}

			event, err := parseStreamEvent(eventType, []byte(data))
			if err != nil {
				// Log the error but continue processing
				continue
			}

			if event != nil {
				return event, nil
			}
			continue
		}

		// Handle "data:" only format (some SSE implementations)
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Try to determine event type from the data itself
			event, err := parseStreamEventFromData([]byte(data))
			if err != nil {
				continue // Skip unparseable events
			}
			if event != nil {
				return event, nil
			}
		}
	}
}

// Close releases resources associated with the stream.
func (s *eventStream) Close() error {
	return s.closer.Close()
}

// parseStreamEvent parses a stream event from its type and JSON data.
func parseStreamEvent(eventType string, data []byte) (types.StreamEvent, error) {
	return types.UnmarshalStreamEvent(data)
}

// parseStreamEventFromData parses a stream event from JSON data,
// determining the event type from the "type" field in the data.
func parseStreamEventFromData(data []byte) (types.StreamEvent, error) {
	return types.UnmarshalStreamEvent(data)
}
