package vai

import (
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestNewClient_DefaultMode(t *testing.T) {
	client := NewClient()
	if !client.IsDirectMode() {
		t.Error("Expected Direct Mode by default")
	}
	if client.IsProxyMode() {
		t.Error("Should not be Proxy Mode")
	}
}

func TestNewClient_ProxyMode(t *testing.T) {
	client := NewClient(
		WithBaseURL("http://localhost:8080"),
		WithAPIKey("vango_sk_test"),
	)
	if !client.IsProxyMode() {
		t.Error("Expected Proxy Mode with BaseURL")
	}
	if client.IsDirectMode() {
		t.Error("Should not be Direct Mode")
	}
}

func TestWithProviderKey(t *testing.T) {
	client := NewClient(
		WithProviderKey("anthropic", "sk-ant-test"),
		WithProviderKey("openai", "sk-test"),
	)

	if client.providerKeys["anthropic"] != "sk-ant-test" {
		t.Error("Anthropic key not set")
	}
	if client.providerKeys["openai"] != "sk-test" {
		t.Error("OpenAI key not set")
	}
}

func TestWithTimeout(t *testing.T) {
	client := NewClient(WithTimeout(30 * time.Second))
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", client.httpClient.Timeout)
	}
}

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	client := NewClient(WithHTTPClient(customClient))
	if client.httpClient != customClient {
		t.Error("HTTP client not set correctly")
	}
}

func TestWithLogger(t *testing.T) {
	logger := slog.Default()
	client := NewClient(WithLogger(logger))
	if client.logger != logger {
		t.Error("Logger not set correctly")
	}
}

func TestWithRetries(t *testing.T) {
	client := NewClient(WithRetries(3))
	if client.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", client.maxRetries)
	}
}

func TestWithRetryBackoff(t *testing.T) {
	client := NewClient(WithRetryBackoff(2 * time.Second))
	if client.retryBackoff != 2*time.Second {
		t.Errorf("retryBackoff = %v, want 2s", client.retryBackoff)
	}
}

func TestClient_Services(t *testing.T) {
	client := NewClient()

	if client.Messages == nil {
		t.Error("Messages service is nil")
	}
	if client.Models == nil {
		t.Error("Models service is nil")
	}
}

func TestClient_Engine(t *testing.T) {
	// Direct mode should have engine
	directClient := NewClient()
	if directClient.Engine() == nil {
		t.Error("Direct mode client should have engine")
	}

	// Proxy mode should not have engine
	proxyClient := NewClient(WithBaseURL("http://localhost:8080"))
	if proxyClient.Engine() != nil {
		t.Error("Proxy mode client should not have engine")
	}
}
