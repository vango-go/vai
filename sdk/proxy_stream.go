package vai

import (
	"io"

	"github.com/vango-go/vai/pkg/core/types"
)

type proxyEventStream struct {
	reader *sseReader
}

func newProxyEventStream(body io.ReadCloser) *proxyEventStream {
	return &proxyEventStream{
		reader: newSSEReader(body),
	}
}

func (s *proxyEventStream) Next() (types.StreamEvent, error) {
	_, payload, err := s.reader.Next()
	if err != nil {
		return nil, err
	}
	event, err := types.UnmarshalStreamEvent(payload)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (s *proxyEventStream) Close() error {
	return s.reader.Close()
}
