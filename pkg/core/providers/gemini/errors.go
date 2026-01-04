package gemini

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

// Error represents an API error from Gemini.
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
		return fmt.Sprintf("gemini: %s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("gemini: %s: %s", e.Type, e.Message)
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

// geminiError represents an error response from Gemini API.
type geminiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type     string `json:"@type"`
			Reason   string `json:"reason,omitempty"`
			Domain   string `json:"domain,omitempty"`
			Metadata map[string]string `json:"metadata,omitempty"`
		} `json:"details,omitempty"`
	} `json:"error"`
}

// parseError parses an error response from Gemini.
func (p *Provider) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var geminiErr geminiError
	if err := json.Unmarshal(body, &geminiErr); err != nil {
		// Can't parse error, return generic
		return &Error{
			Type:    ErrProvider,
			Message: string(body),
		}
	}

	// Map Gemini status codes to our error types
	var errType ErrorType
	switch geminiErr.Error.Status {
	case "INVALID_ARGUMENT":
		errType = ErrInvalidRequest
	case "UNAUTHENTICATED":
		errType = ErrAuthentication
	case "PERMISSION_DENIED":
		errType = ErrPermission
	case "NOT_FOUND":
		errType = ErrNotFound
	case "RESOURCE_EXHAUSTED":
		errType = ErrRateLimit
	case "INTERNAL":
		errType = ErrAPI
	case "UNAVAILABLE":
		errType = ErrOverloaded
	case "FAILED_PRECONDITION":
		errType = ErrInvalidRequest
	default:
		errType = ErrProvider
	}

	// Also check HTTP status code
	if resp.StatusCode == 429 {
		errType = ErrRateLimit
	}
	if resp.StatusCode == 503 {
		errType = ErrOverloaded
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		errType = ErrAuthentication
	}

	return &Error{
		Type:          errType,
		Message:       geminiErr.Error.Message,
		Code:          geminiErr.Error.Status,
		ProviderError: geminiErr.Error,
	}
}
