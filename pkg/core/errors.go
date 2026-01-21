package core

import (
	"fmt"
)

// Error represents an API error.
type Error struct {
	Type          ErrorType `json:"type"`
	Message       string    `json:"message"`
	Param         string    `json:"param,omitempty"`
	Code          string    `json:"code,omitempty"`
	RequestID     string    `json:"request_id,omitempty"`
	ProviderError any       `json:"provider_error,omitempty"`
	RetryAfter    *int      `json:"retry_after,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (code: %s)", e.Type, e.Message, e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

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

// NewInvalidRequestError creates an invalid request error.
func NewInvalidRequestError(message string) *Error {
	return &Error{
		Type:    ErrInvalidRequest,
		Message: message,
	}
}

// NewInvalidRequestErrorWithParam creates an invalid request error with a parameter.
func NewInvalidRequestErrorWithParam(message, param string) *Error {
	return &Error{
		Type:    ErrInvalidRequest,
		Message: message,
		Param:   param,
	}
}

// NewAuthenticationError creates an authentication error.
func NewAuthenticationError(message string) *Error {
	return &Error{
		Type:    ErrAuthentication,
		Message: message,
	}
}

// NewPermissionError creates a permission error.
func NewPermissionError(message string) *Error {
	return &Error{
		Type:    ErrPermission,
		Message: message,
	}
}

// NewNotFoundError creates a not found error.
func NewNotFoundError(message string) *Error {
	return &Error{
		Type:    ErrNotFound,
		Message: message,
	}
}

// NewRateLimitError creates a rate limit error.
func NewRateLimitError(message string, retryAfter int) *Error {
	return &Error{
		Type:       ErrRateLimit,
		Message:    message,
		RetryAfter: &retryAfter,
	}
}

// NewAPIError creates a generic API error.
func NewAPIError(message string) *Error {
	return &Error{
		Type:    ErrAPI,
		Message: message,
	}
}

// NewOverloadedError creates an overloaded error.
func NewOverloadedError(message string) *Error {
	return &Error{
		Type:    ErrOverloaded,
		Message: message,
	}
}

// NewProviderError creates a provider-specific error.
func NewProviderError(provider string, underlying error) *Error {
	return &Error{
		Type:          ErrProvider,
		Message:       fmt.Sprintf("%s: %v", provider, underlying),
		ProviderError: underlying.Error(),
	}
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

// Unwrap returns the underlying error for error wrapping.
func (e *Error) Unwrap() error {
	if ue, ok := e.ProviderError.(error); ok {
		return ue
	}
	return nil
}
