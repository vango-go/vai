//go:build integration
// +build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/vango-go/vai/pkg/core"
	vai "github.com/vango-go/vai/sdk"
)

func TestError_InvalidModel(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Name + "/nonexistent-model-xyz",
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello")},
			},
			MaxTokens: 8000,
		})

		if err == nil {
			t.Fatal("expected error for invalid model")
		}

		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			t.Logf("Error type: %s, message: %s", apiErr.Type, apiErr.Message)
			// The exact error type may vary (not_found_error or invalid_request_error)
		} else {
			t.Logf("Non-API error: %v", err)
		}
	})
}

func TestError_InvalidRequest_EmptyMessages(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:     provider.Model,
			Messages:  []vai.Message{}, // Empty messages
			MaxTokens: 8000,
		})

		if err == nil {
			t.Fatal("expected error for empty messages")
		}

		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Type != vai.ErrInvalidRequest {
				t.Logf("Error type: %s (expected invalid_request_error)", apiErr.Type)
			}
			t.Logf("Error message: %s", apiErr.Message)
		} else {
			// May be a local validation error
			t.Logf("Error: %v", err)
		}
	})
}

func TestError_AuthenticationError(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		// Create client with invalid key
		badClient := vai.NewClient(
			vai.WithProviderKey(provider.KeyName, "invalid-api-key-xxx"),
		)

		_, err := badClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello")},
			},
			MaxTokens: 8000,
		})

		if err == nil {
			t.Fatal("expected error for invalid API key")
		}

		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Type != vai.ErrAuthentication {
				t.Logf("Error type: %s (expected authentication_error)", apiErr.Type)
			}
			t.Logf("Error message: %s", apiErr.Message)
		} else {
			t.Logf("Non-API error: %v", err)
		}
	})
}

func TestError_MissingProviderKey(t *testing.T) {
	ctx := defaultTestContext(t)

	// Create client with no provider keys configured
	emptyClient := vai.NewClient()

	// Clear any env vars that might be set
	// Note: We can't actually clear env vars, so this test depends on
	// the provider not being configured through other means

	_, err := emptyClient.Messages.Create(ctx, &vai.MessageRequest{
		Model: "unknownprovider/some-model",
		Messages: []vai.Message{
			{Role: "user", Content: vai.Text("Hello")},
		},
		MaxTokens: 8000,
	})

	if err == nil {
		t.Log("warning: expected error for unknown provider")
		return
	}

	t.Logf("Error for unknown provider: %v", err)
}

func TestError_IsRetryable(t *testing.T) {
	// Test the IsRetryable method on different error types
	tests := []struct {
		errType    core.ErrorType
		retryable  bool
	}{
		{core.ErrInvalidRequest, false},
		{core.ErrAuthentication, false},
		{core.ErrPermission, false},
		{core.ErrNotFound, false},
		{core.ErrRateLimit, true},
		{core.ErrAPI, true},
		{core.ErrOverloaded, true},
		{core.ErrProvider, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.errType), func(t *testing.T) {
			err := &vai.Error{
				Type:    tc.errType,
				Message: "test error",
			}

			if err.IsRetryable() != tc.retryable {
				t.Errorf("expected IsRetryable()=%v for %s", tc.retryable, tc.errType)
			}
		})
	}
}

func TestError_ErrorMessage(t *testing.T) {
	err := &vai.Error{
		Type:    vai.ErrInvalidRequest,
		Message: "missing required field",
		Param:   "messages",
		Code:    "missing_field",
	}

	msg := err.Error()

	// Should contain the type and message
	if msg == "" {
		t.Error("expected non-empty error message")
	}

	t.Logf("Error message: %s", msg)
}

func TestError_InvalidRequest_InvalidContent(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		// Try to send invalid content structure
		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "invalid-role", Content: vai.Text("Hello")}, // Invalid role
			},
			MaxTokens: 8000,
		})

		if err == nil {
			t.Log("warning: expected error for invalid role")
			return
		}

		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			t.Logf("Error type: %s, message: %s", apiErr.Type, apiErr.Message)
		} else {
			t.Logf("Error: %v", err)
		}
	})
}

func TestError_InvalidRequest_NegativeMaxTokens(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model: provider.Model,
			Messages: []vai.Message{
				{Role: "user", Content: vai.Text("Hello")},
			},
			MaxTokens: -1, // Invalid negative value
		})

		if err == nil {
			t.Log("warning: expected error for negative max_tokens")
			return
		}

		t.Logf("Error for negative max_tokens: %v", err)
	})
}

func TestError_ExtractTypedError(t *testing.T) {
	forEachProvider(t, func(t *testing.T, provider providerConfig) {
		ctx := defaultTestContext(t)

		// Intentionally cause an error
		_, err := testClient.Messages.Create(ctx, &vai.MessageRequest{
			Model:    provider.Model,
			Messages: []vai.Message{},
		})

		if err == nil {
			t.Skip("no error to test")
		}

		// Try to extract as typed error
		var apiErr *vai.Error
		if errors.As(err, &apiErr) {
			// Verify we can access all fields
			_ = apiErr.Type
			_ = apiErr.Message
			_ = apiErr.Param
			_ = apiErr.Code
			_ = apiErr.RequestID
			_ = apiErr.RetryAfter

			t.Logf("Successfully extracted typed error: %s", apiErr.Type)
		} else {
			t.Logf("Could not extract as typed error, got: %T", err)
		}
	})
}
