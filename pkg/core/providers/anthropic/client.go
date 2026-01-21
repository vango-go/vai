package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// doRequest sends a non-streaming request to Anthropic.
func (p *Provider) doRequest(ctx context.Context, req *anthropicRequest, stream bool) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	p.setHeaders(httpReq, stream)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode >= 400 {
		return nil, p.parseError(resp)
	}

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return respBody, nil
}

// doStreamRequest sends a streaming request to Anthropic.
func (p *Provider) doStreamRequest(ctx context.Context, req *anthropicRequest) (io.ReadCloser, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers for streaming
	p.setHeaders(httpReq, true)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	// Check for errors before returning stream
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, p.parseError(resp)
	}

	return resp.Body, nil
}

// setHeaders sets the required Anthropic API headers.
func (p *Provider) setHeaders(req *http.Request, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("anthropic-version", APIVersion)
	req.Header.Set("anthropic-beta", BetaHeader)

	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
}
