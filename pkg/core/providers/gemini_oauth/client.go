package gemini_oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// doRequest sends a non-streaming request to Cloud Code Assist.
func (p *Provider) doRequest(ctx context.Context, model string, req *geminiRequest) ([]byte, error) {
	// Marshal the Gemini request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Wrap for Cloud Code Assist format
	wrappedBody, err := WrapRequest(body, p.projectID, model)
	if err != nil {
		return nil, fmt.Errorf("wrap request: %w", err)
	}

	if debugRequests {
		log.Printf("[GEMINI-OAUTH] Request body: %s", string(wrappedBody))
	}

	// Build URL for Cloud Code Assist
	url := fmt.Sprintf("%s/v1internal:generateContent", CloudCodeEndpoint)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(wrappedBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Get valid OAuth token
	token, err := p.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	// Set headers
	p.setHeaders(httpReq, token, false)

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

	if debugRequests {
		log.Printf("[GEMINI-OAUTH] Raw response: %s", string(respBody))
	}

	// Unwrap Cloud Code response
	unwrapped, err := UnwrapResponse(respBody)
	if err != nil {
		return nil, fmt.Errorf("unwrap response: %w", err)
	}

	if debugRequests {
		log.Printf("[GEMINI-OAUTH] Unwrapped response: %s", string(unwrapped))
	}

	return unwrapped, nil
}

// doStreamRequest sends a streaming request to Cloud Code Assist.
func (p *Provider) doStreamRequest(ctx context.Context, model string, req *geminiRequest) (io.ReadCloser, error) {
	// Marshal the Gemini request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Wrap for Cloud Code Assist format
	wrappedBody, err := WrapRequest(body, p.projectID, model)
	if err != nil {
		return nil, fmt.Errorf("wrap request: %w", err)
	}

	// Build URL for streaming
	url := fmt.Sprintf("%s/v1internal:streamGenerateContent?alt=sse", CloudCodeEndpoint)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(wrappedBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Get valid OAuth token
	token, err := p.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	// Set headers for streaming
	p.setHeaders(httpReq, token, true)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	// Check for errors before returning stream
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, p.parseError(resp)
	}

	// Wrap response body in TransformingReader to unwrap SSE events
	return NewTransformingReader(resp.Body), nil
}

// setHeaders sets the required Cloud Code Assist API headers.
func (p *Provider) setHeaders(req *http.Request, token string, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Add spoof headers for compatibility
	for k, v := range SpoofHeaders {
		req.Header.Set(k, v)
	}

	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
}
