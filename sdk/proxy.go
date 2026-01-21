package vai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vango-go/vai/pkg/core"
)

type proxyErrorResponse struct {
	Type  string     `json:"type"`
	Error core.Error `json:"error"`
}

func (c *Client) proxyURL(path string) string {
	return strings.TrimRight(c.baseURL, "/") + path
}

func (c *Client) addAuthHeaders(req *http.Request) {
	if c.apiKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func (c *Client) doProxyJSON(ctx context.Context, method, path string, body any, out any) error {
	attempt := 0
	backoff := c.retryBackoff

	for {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.proxyURL(path), bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.addAuthHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if shouldRetry(ctx, attempt, c.maxRetries) {
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				attempt++
				continue
			}
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			apiErr := parseProxyError(resp.StatusCode, respBody)
			if shouldRetryProxyError(ctx, attempt, c.maxRetries, apiErr) {
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				attempt++
				continue
			}
			return apiErr
		}

		if out != nil {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}

		return nil
	}
}

func (c *Client) doProxyGET(ctx context.Context, path string, out any) error {
	attempt := 0
	backoff := c.retryBackoff

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.proxyURL(path), nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		c.addAuthHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if shouldRetry(ctx, attempt, c.maxRetries) {
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				attempt++
				continue
			}
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			apiErr := parseProxyError(resp.StatusCode, respBody)
			if shouldRetryProxyError(ctx, attempt, c.maxRetries, apiErr) {
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				attempt++
				continue
			}
			return apiErr
		}

		if out != nil {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}

		return nil
	}
}

func (c *Client) openProxyStream(ctx context.Context, path string, body any) (*http.Response, error) {
	attempt := 0
	backoff := c.retryBackoff

	for {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.proxyURL(path), bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.addAuthHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if shouldRetry(ctx, attempt, c.maxRetries) {
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				attempt++
				continue
			}
			return nil, err
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			apiErr := parseProxyError(resp.StatusCode, respBody)
			if shouldRetryProxyError(ctx, attempt, c.maxRetries, apiErr) {
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				attempt++
				continue
			}
			return nil, apiErr
		}

		return resp, nil
	}
}

func parseProxyError(status int, body []byte) error {
	var resp proxyErrorResponse
	if err := json.Unmarshal(body, &resp); err == nil && resp.Error.Type != "" {
		return &resp.Error
	}

	message := strings.TrimSpace(string(body))
	if message == "" {
		message = fmt.Sprintf("proxy error (%d)", status)
	}
	return core.NewAPIError(message)
}

func shouldRetryProxyError(ctx context.Context, attempt, maxRetries int, err error) bool {
	if !shouldRetry(ctx, attempt, maxRetries) {
		return false
	}
	if apiErr, ok := err.(*core.Error); ok {
		return apiErr.IsRetryable()
	}
	return false
}

func shouldRetry(ctx context.Context, attempt, maxRetries int) bool {
	if ctx.Err() != nil {
		return false
	}
	return attempt < maxRetries
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next == 0 {
		return time.Second
	}
	return next
}
