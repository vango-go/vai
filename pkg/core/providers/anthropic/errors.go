package anthropic

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

// Error represents an API error from Anthropic.
type Error struct {
	Type          ErrorType `json:"type"`
	Message       string    `json:"message"`
	Code          string    `json:"code,omitempty"`
	ProviderError any       `json:"provider_error,omitempty"`
	RetryAfter    *int      `json:"retry_after,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("anthropic: %s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("anthropic: %s: %s", e.Type, e.Message)
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

// anthropicError represents an error response from Anthropic.
type anthropicError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// parseError parses an error response from Anthropic.
func (p *Provider) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var anthErr anthropicError
	if err := json.Unmarshal(body, &anthErr); err != nil {
		// Can't parse error, return generic
		return &Error{
			Type:    ErrProvider,
			Message: string(body),
		}
	}

	// Map Anthropic error types to our error types
	var errType ErrorType
	switch anthErr.Error.Type {
	case "invalid_request_error":
		errType = ErrInvalidRequest
	case "authentication_error":
		errType = ErrAuthentication
	case "permission_error":
		errType = ErrPermission
	case "not_found_error":
		errType = ErrNotFound
	case "rate_limit_error":
		errType = ErrRateLimit
	case "api_error":
		errType = ErrAPI
	case "overloaded_error":
		errType = ErrOverloaded
	default:
		errType = ErrProvider
	}

	return &Error{
		Type:          errType,
		Message:       anthErr.Error.Message,
		ProviderError: anthErr.Error,
	}
}
