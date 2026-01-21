package gemini_oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Error types for the provider.
const (
	ErrInvalidRequest = "invalid_request_error"
	ErrAuthentication = "authentication_error"
	ErrPermission     = "permission_error"
	ErrNotFound       = "not_found_error"
	ErrRateLimit      = "rate_limit_error"
	ErrAPI            = "api_error"
	ErrOverloaded     = "overloaded_error"
	ErrProvider       = "provider_error"
)

// Error represents a Gemini OAuth API error.
type Error struct {
	Type          string
	Message       string
	Code          string
	ProviderError any
	RetryAfter    *int
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("gemini-oauth: %s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("gemini-oauth: %s: %s", e.Type, e.Message)
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

// geminiErrorResponse represents a Gemini API error response.
type geminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// parseError parses an HTTP error response into an Error.
func (p *Provider) parseError(resp *http.Response) *Error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Error{
			Type:    ErrProvider,
			Message: fmt.Sprintf("failed to read error response: %v", err),
		}
	}

	var geminiErr geminiErrorResponse
	if err := json.Unmarshal(body, &geminiErr); err != nil {
		return &Error{
			Type:          ErrProvider,
			Message:       string(body),
			ProviderError: string(body),
		}
	}

	// Map Gemini status to error type
	errType := mapStatusToErrorType(geminiErr.Error.Status, resp.StatusCode)

	return &Error{
		Type:    errType,
		Message: geminiErr.Error.Message,
		Code:    geminiErr.Error.Status,
		ProviderError: map[string]any{
			"code":    geminiErr.Error.Code,
			"message": geminiErr.Error.Message,
			"status":  geminiErr.Error.Status,
		},
	}
}

// mapStatusToErrorType maps Gemini status codes to error types.
func mapStatusToErrorType(status string, httpStatus int) string {
	// First check HTTP status for overrides
	switch httpStatus {
	case 429:
		return ErrRateLimit
	case 503:
		return ErrOverloaded
	case 401, 403:
		return ErrAuthentication
	}

	// Then check Gemini status
	switch status {
	case "INVALID_ARGUMENT":
		return ErrInvalidRequest
	case "UNAUTHENTICATED":
		return ErrAuthentication
	case "PERMISSION_DENIED":
		return ErrPermission
	case "NOT_FOUND":
		return ErrNotFound
	case "RESOURCE_EXHAUSTED":
		return ErrRateLimit
	case "INTERNAL":
		return ErrAPI
	case "UNAVAILABLE":
		return ErrOverloaded
	case "FAILED_PRECONDITION":
		return ErrInvalidRequest
	default:
		return ErrProvider
	}
}
