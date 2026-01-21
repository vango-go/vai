package proxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func requireTCPListenServer(t testing.TB) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping test: TCP listen not permitted in this environment: %v", err)
	}
	ln.Close()
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.RateLimit.MaxConcurrentSessions != 1000 {
		t.Errorf("expected 1000 max sessions, got %d", cfg.RateLimit.MaxConcurrentSessions)
	}
	if cfg.Observability.MetricsEnabled != true {
		t.Error("expected metrics enabled")
	}
}

func TestConfigOptions(t *testing.T) {
	cfg := DefaultConfig()

	WithHost("localhost")(cfg)
	if cfg.Host != "localhost" {
		t.Errorf("expected localhost, got %s", cfg.Host)
	}

	WithPort(9090)(cfg)
	if cfg.Port != 9090 {
		t.Errorf("expected 9090, got %d", cfg.Port)
	}

	WithProviderKey("anthropic", "test-key")(cfg)
	if cfg.ProviderKeys["anthropic"] != "test-key" {
		t.Errorf("expected test-key, got %s", cfg.ProviderKeys["anthropic"])
	}

	WithRateLimit(100, 10)(cfg)
	if cfg.RateLimit.GlobalRequestsPerMinute != 100 {
		t.Errorf("expected 100, got %d", cfg.RateLimit.GlobalRequestsPerMinute)
	}

	WithMetrics(false)(cfg)
	if cfg.Observability.MetricsEnabled != false {
		t.Error("expected metrics disabled")
	}
}

func TestResponseWriter(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	rw.WriteHeader(http.StatusCreated)
	if rw.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", rw.StatusCode)
	}

	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes, got %d", n)
	}
	if rw.BytesWritten != 5 {
		t.Errorf("expected 5 bytes written, got %d", rw.BytesWritten)
	}
}

func TestAuthMiddleware(t *testing.T) {
	keys := []APIKeyConfig{
		{Key: "valid-key", Name: "test", UserID: "user1"},
	}
	auth := NewAuthMiddleware("api_key", keys, nil, nil)

	handler := auth.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(ContextKeyUserID).(string)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(userID))
	}))

	t.Run("valid key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer valid-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if w.Body.String() != "user1" {
			t.Errorf("expected user1, got %s", w.Body.String())
		}
	})

	t.Run("missing key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("invalid key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Code)
		}
	})

	t.Run("x-api-key header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-API-Key", "valid-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestAuthMiddleware_WebSocket(t *testing.T) {
	keys := []APIKeyConfig{
		{Key: "ws-key", Name: "ws-test", UserID: "ws-user"},
	}
	auth := NewAuthMiddleware("api_key", keys, nil, nil)

	t.Run("query param", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?api_key=ws-key", nil)
		cfg, ok := auth.AuthenticateWebSocket(req)
		if !ok {
			t.Error("expected authentication to succeed")
		}
		if cfg.UserID != "ws-user" {
			t.Errorf("expected ws-user, got %s", cfg.UserID)
		}
	})

	t.Run("header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer ws-key")
		cfg, ok := auth.AuthenticateWebSocket(req)
		if !ok {
			t.Error("expected authentication to succeed")
		}
		if cfg.UserID != "ws-user" {
			t.Errorf("expected ws-user, got %s", cfg.UserID)
		}
	})
}

func TestRateLimiter(t *testing.T) {
	cfg := RateLimitConfig{
		GlobalRequestsPerMinute: 2,
		UserRequestsPerMinute:   1,
		MaxConcurrentSessions:   5,
	}
	rl := NewRateLimiter(cfg, nil, NewMetrics("test"))

	t.Run("under limit", func(t *testing.T) {
		if !rl.checkGlobalLimit() {
			t.Error("expected global limit check to pass")
		}
	})

	t.Run("over global limit", func(t *testing.T) {
		// First two should pass
		rl.checkGlobalLimit()
		// Third should fail
		if rl.checkGlobalLimit() {
			t.Error("expected global limit check to fail")
		}
	})

	t.Run("live session limit", func(t *testing.T) {
		if !rl.CheckLiveSessionLimit(4) {
			t.Error("expected 4 sessions to be under limit")
		}
		if rl.CheckLiveSessionLimit(5) {
			t.Error("expected 5 sessions to be at limit")
		}
	})
}

func TestLoggingMiddleware(t *testing.T) {
	logger := NewLoggingMiddleware(nil)

	handler := logger.Log(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Context().Value(ContextKeyRequestID).(string)
		if !strings.HasPrefix(requestID, "req_") {
			t.Errorf("expected request ID to start with req_, got %s", requestID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Check X-Request-ID header
	requestID := w.Header().Get("X-Request-ID")
	if !strings.HasPrefix(requestID, "req_") {
		t.Errorf("expected request ID header to start with req_, got %s", requestID)
	}
}

func TestCORSMiddleware(t *testing.T) {
	cors := NewCORSMiddleware([]string{"https://example.com"})

	handler := cors.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("preflight", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
			t.Errorf("expected CORS header")
		}
	})

	t.Run("regular request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("disallowed origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "https://other.com")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("expected no CORS header for disallowed origin")
		}
	})
}

func TestRecoveryMiddleware(t *testing.T) {
	recovery := NewRecoveryMiddleware(nil, NewMetrics("test"))

	handler := recovery.Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["type"] != "error" {
		t.Errorf("expected error type")
	}
}

func TestMetrics(t *testing.T) {
	m := NewMetrics("test")

	t.Run("record request", func(t *testing.T) {
		m.RecordRequest("anthropic", "claude-3", "/v1/messages", "success", time.Second)
	})

	t.Run("record tokens", func(t *testing.T) {
		m.RecordTokens("anthropic", "claude-3", 100, 200)
	})

	t.Run("record live session", func(t *testing.T) {
		m.RecordLiveSessionStart()
		m.RecordLiveSessionEnd("anthropic", "claude-3", "success", time.Minute)
	})

	t.Run("record error", func(t *testing.T) {
		m.RecordError("anthropic", "timeout")
	})

	t.Run("record rate limit", func(t *testing.T) {
		m.RecordRateLimitHit("user1", "global")
	})
}

func TestServer_Health(t *testing.T) {
	requireTCPListenServer(t)
	// Create server with minimal config
	server, err := NewServer(
		WithAPIKey("test-key", "test", "user1", 100),
		WithProviderKey("anthropic", "sk-test"),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Create test server
	ts := httptest.NewServer(server.mux)
	defer ts.Close()

	// Test health endpoint
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var health map[string]any
	json.NewDecoder(resp.Body).Decode(&health)
	if health["status"] != "healthy" {
		t.Errorf("expected healthy status")
	}
}

func TestServer_Shutdown(t *testing.T) {
	server, err := NewServer(
		WithAPIKey("test-key", "test", "user1", 100),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Shutdown should succeed even if server wasn't started
	err = server.Shutdown(ctx)
	if err != nil && err != http.ErrServerClosed {
		t.Errorf("unexpected shutdown error: %v", err)
	}
}
