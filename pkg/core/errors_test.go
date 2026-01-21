package core

import (
	"testing"
)

func TestError_Error(t *testing.T) {
	err := &Error{
		Type:    ErrInvalidRequest,
		Message: "invalid model format",
	}

	expected := "invalid_request_error: invalid model format"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestError_WithCode(t *testing.T) {
	err := &Error{
		Type:    ErrRateLimit,
		Message: "too many requests",
		Code:    "rate_limit_exceeded",
	}

	expected := "rate_limit_error: too many requests (code: rate_limit_exceeded)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestNewInvalidRequestError(t *testing.T) {
	err := NewInvalidRequestError("bad request")
	if err.Type != ErrInvalidRequest {
		t.Errorf("Type = %v, want %v", err.Type, ErrInvalidRequest)
	}
	if err.Message != "bad request" {
		t.Errorf("Message = %q, want %q", err.Message, "bad request")
	}
}

func TestNewAuthenticationError(t *testing.T) {
	err := NewAuthenticationError("invalid API key")
	if err.Type != ErrAuthentication {
		t.Errorf("Type = %v, want %v", err.Type, ErrAuthentication)
	}
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError("rate limit exceeded", 60)
	if err.Type != ErrRateLimit {
		t.Errorf("Type = %v, want %v", err.Type, ErrRateLimit)
	}
	if err.RetryAfter == nil || *err.RetryAfter != 60 {
		t.Errorf("RetryAfter = %v, want 60", err.RetryAfter)
	}
}

func TestNewProviderError(t *testing.T) {
	underlying := NewAPIError("upstream error")
	err := NewProviderError("anthropic", underlying)

	if err.Type != ErrProvider {
		t.Errorf("Type = %v, want %v", err.Type, ErrProvider)
	}
	if err.ProviderError == nil {
		t.Error("ProviderError should not be nil")
	}
}

func TestError_IsRetryable(t *testing.T) {
	tests := []struct {
		errType ErrorType
		want    bool
	}{
		{ErrRateLimit, true},
		{ErrOverloaded, true},
		{ErrAPI, true},
		{ErrInvalidRequest, false},
		{ErrAuthentication, false},
		{ErrPermission, false},
		{ErrNotFound, false},
		{ErrProvider, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.errType), func(t *testing.T) {
			err := &Error{Type: tt.errType, Message: "test"}
			if got := err.IsRetryable(); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseModelString(t *testing.T) {
	tests := []struct {
		model     string
		wantProv  string
		wantModel string
		wantErr   bool
	}{
		{"anthropic/claude-sonnet-4", "anthropic", "claude-sonnet-4", false},
		{"openai/gpt-4o", "openai", "gpt-4o", false},
		{"gemini/gemini-2.0-flash", "gemini", "gemini-2.0-flash", false},
		{"groq/llama-3.3-70b", "groq", "llama-3.3-70b", false},
		{"invalid", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			prov, model, err := ParseModelString(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseModelString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if prov != tt.wantProv {
				t.Errorf("provider = %q, want %q", prov, tt.wantProv)
			}
			if model != tt.wantModel {
				t.Errorf("model = %q, want %q", model, tt.wantModel)
			}
		})
	}
}
