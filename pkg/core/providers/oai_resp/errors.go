package oai_resp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ErrorType categorizes errors.
type ErrorType string

const (
	ErrInvalidRequest ErrorType = "invalid_request_error"
	ErrAuthentication ErrorType = "authentication_error"
	ErrPermission     ErrorType = "permission_error"
	ErrNotFound       ErrorType = "not_found_error"
	ErrRateLimit      ErrorType = "rate_limit_error"
	ErrAPI            ErrorType = "api_error"
	ErrOverloaded     ErrorType = "overloaded_error"
	ErrProvider       ErrorType = "provider_error"
)

// Error represents an API error from OpenAI Responses API.
type Error struct {
	Type          ErrorType `json:"type"`
	Message       string    `json:"message"`
	Code          string    `json:"code,omitempty"`
	Param         string    `json:"param,omitempty"`
	ProviderError any       `json:"provider_error,omitempty"`
	RetryAfter    *int      `json:"retry_after,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("oai-resp: %s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("oai-resp: %s: %s", e.Type, e.Message)
}

// IsRetryable returns true if the error is retryable.
func (e *Error) IsRetryable() bool {
	switch e.Type {
	case ErrRateLimit, ErrOverloaded, ErrAPI:
		return true
	default:
		return false
	}
}

// openaiError represents an error response from OpenAI.
type openaiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// parseError parses an error response from OpenAI Responses API.
func (p *Provider) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var openaiErr openaiError
	if err := json.Unmarshal(body, &openaiErr); err != nil {
		// Can't parse error, return generic
		return &Error{
			Type:    ErrProvider,
			Message: string(body),
		}
	}

	// Map OpenAI error types to our error types
	var errType ErrorType
	switch openaiErr.Error.Type {
	case "invalid_request_error":
		errType = ErrInvalidRequest
	case "authentication_error":
		errType = ErrAuthentication
	case "permission_error", "insufficient_quota":
		errType = ErrPermission
	case "not_found_error":
		errType = ErrNotFound
	case "rate_limit_error":
		errType = ErrRateLimit
	case "server_error", "api_error":
		errType = ErrAPI
	case "overloaded_error", "service_unavailable":
		errType = ErrOverloaded
	default:
		errType = ErrProvider
	}

	// Also check HTTP status code for rate limits
	if resp.StatusCode == 429 {
		errType = ErrRateLimit
	}
	if resp.StatusCode == 503 {
		errType = ErrOverloaded
	}

	return &Error{
		Type:          errType,
		Message:       openaiErr.Error.Message,
		Code:          openaiErr.Error.Code,
		Param:         openaiErr.Error.Param,
		ProviderError: openaiErr.Error,
	}
}
