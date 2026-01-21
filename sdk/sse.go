package vai

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

type sseReader struct {
	reader *bufio.Reader
	body   io.Closer
}

func newSSEReader(body io.ReadCloser) *sseReader {
	return &sseReader{
		reader: bufio.NewReader(body),
		body:   body,
	}
}

func (s *sseReader) Next() (string, []byte, error) {
	var eventName string
	var data bytes.Buffer

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if data.Len() == 0 {
				if err == io.EOF {
					return "", nil, io.EOF
				}
				continue
			}

			payload := data.Bytes()
			if strings.TrimSpace(string(payload)) == "[DONE]" {
				return "", nil, io.EOF
			}
			return eventName, payload, nil
		}

		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			chunk := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(chunk)
		}

		if err == io.EOF {
			if data.Len() == 0 {
				return "", nil, io.EOF
			}
			payload := data.Bytes()
			if strings.TrimSpace(string(payload)) == "[DONE]" {
				return "", nil, io.EOF
			}
			return eventName, payload, nil
		}
	}
}

func (s *sseReader) Close() error {
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}
