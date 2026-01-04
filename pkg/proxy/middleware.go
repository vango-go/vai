package proxy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// contextKey is a type for context keys.
type contextKey string

const (
	// ContextKeyUserID is the context key for the authenticated user ID.
	ContextKeyUserID contextKey = "user_id"
	// ContextKeyAPIKeyName is the context key for the API key name.
	ContextKeyAPIKeyName contextKey = "api_key_name"
	// ContextKeyRequestID is the context key for the request ID.
	ContextKeyRequestID contextKey = "request_id"
)

// AuthMiddleware provides authentication middleware.
type AuthMiddleware struct {
	keys    map[string]APIKeyConfig
	logger  *slog.Logger
	metrics *Metrics
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(keys []APIKeyConfig, logger *slog.Logger, metrics *Metrics) *AuthMiddleware {
	keyMap := make(map[string]APIKeyConfig)
	for _, k := range keys {
		keyMap[k.Key] = k
	}
	return &AuthMiddleware{
		keys:    keyMap,
		logger:  logger,
		metrics: metrics,
	}
}

// Authenticate is the HTTP middleware handler.
func (a *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractAPIKey(r)
		if key == "" {
			a.writeError(w, http.StatusUnauthorized, "authentication_error", "Missing API key")
			return
		}

		keyConfig, ok := a.keys[key]
		if !ok {
			a.writeError(w, http.StatusUnauthorized, "authentication_error", "Invalid API key")
			return
		}

		// Add user info to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, ContextKeyUserID, keyConfig.UserID)
		ctx = context.WithValue(ctx, ContextKeyAPIKeyName, keyConfig.Name)

		if a.logger != nil {
			a.logger.Debug("request authenticated",
				"user_id", keyConfig.UserID,
				"key_name", keyConfig.Name,
				"path", r.URL.Path,
			)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthenticateWebSocket extracts and validates the API key for WebSocket connections.
// WebSocket connections can pass the API key as a query parameter.
func (a *AuthMiddleware) AuthenticateWebSocket(r *http.Request) (APIKeyConfig, bool) {
	key := extractAPIKey(r)
	if key == "" {
		// Try query parameter for WebSocket
		key = r.URL.Query().Get("api_key")
	}
	if key == "" {
		return APIKeyConfig{}, false
	}

	keyConfig, ok := a.keys[key]
	return keyConfig, ok
}

func extractAPIKey(r *http.Request) string {
	// Check Authorization header (Bearer token)
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check X-API-Key header
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	return ""
}

func (a *AuthMiddleware) writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

// RateLimiter provides rate limiting.
type RateLimiter struct {
	config  RateLimitConfig
	logger  *slog.Logger
	metrics *Metrics

	// Per-user token buckets
	mu      sync.RWMutex
	buckets map[string]*tokenBucket

	// Global counter
	globalMu    sync.Mutex
	globalCount int
	globalReset time.Time
}

// tokenBucket implements a simple token bucket rate limiter.
type tokenBucket struct {
	tokens     int
	lastRefill time.Time
	limit      int
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config RateLimitConfig, logger *slog.Logger, metrics *Metrics) *RateLimiter {
	return &RateLimiter{
		config:      config,
		logger:      logger,
		metrics:     metrics,
		buckets:     make(map[string]*tokenBucket),
		globalReset: time.Now().Add(time.Minute),
	}
}

// RateLimit is the HTTP middleware handler.
func (rl *RateLimiter) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(ContextKeyUserID).(string)
		if userID == "" {
			userID = r.RemoteAddr
		}

		// Check global rate limit
		if !rl.checkGlobalLimit() {
			rl.metrics.RecordRateLimitHit(userID, "global")
			rl.writeRateLimitError(w, rl.secondsUntilReset())
			return
		}

		// Check per-user rate limit
		if !rl.checkUserLimit(userID) {
			rl.metrics.RecordRateLimitHit(userID, "user")
			rl.writeRateLimitError(w, rl.secondsUntilReset())
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) checkGlobalLimit() bool {
	rl.globalMu.Lock()
	defer rl.globalMu.Unlock()

	now := time.Now()
	if now.After(rl.globalReset) {
		rl.globalCount = 0
		rl.globalReset = now.Add(time.Minute)
	}

	if rl.globalCount >= rl.config.GlobalRequestsPerMinute {
		return false
	}

	rl.globalCount++
	return true
}

func (rl *RateLimiter) checkUserLimit(userID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[userID]
	if !exists {
		bucket = &tokenBucket{
			tokens:     rl.config.UserRequestsPerMinute,
			lastRefill: time.Now(),
			limit:      rl.config.UserRequestsPerMinute,
		}
		rl.buckets[userID] = bucket
	}

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	tokensToAdd := int(elapsed.Minutes() * float64(bucket.limit))
	if tokensToAdd > 0 {
		bucket.tokens = min(bucket.tokens+tokensToAdd, bucket.limit)
		bucket.lastRefill = now
	}

	// Check if we have tokens
	if bucket.tokens <= 0 {
		return false
	}

	bucket.tokens--
	return true
}

// CheckLiveSessionLimit checks if a new live session can be started.
func (rl *RateLimiter) CheckLiveSessionLimit(currentSessions int) bool {
	return currentSessions < rl.config.MaxConcurrentSessions
}

func (rl *RateLimiter) secondsUntilReset() int {
	rl.globalMu.Lock()
	defer rl.globalMu.Unlock()
	return int(time.Until(rl.globalReset).Seconds())
}

func (rl *RateLimiter) writeRateLimitError(w http.ResponseWriter, retryAfter int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", string(rune(retryAfter)))
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":        "rate_limit_error",
			"message":     "Rate limit exceeded. Please retry after the specified time.",
			"retry_after": retryAfter,
		},
	})
}

// Cleanup removes stale buckets. Call periodically.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	for userID, bucket := range rl.buckets {
		if bucket.lastRefill.Before(cutoff) && bucket.tokens >= bucket.limit {
			delete(rl.buckets, userID)
		}
	}
}

// LoggingMiddleware provides request logging.
type LoggingMiddleware struct {
	logger *slog.Logger
}

// NewLoggingMiddleware creates a new logging middleware.
func NewLoggingMiddleware(logger *slog.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{logger: logger}
}

// Log is the HTTP middleware handler.
func (l *LoggingMiddleware) Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := generateRequestID()
		ctx := context.WithValue(r.Context(), ContextKeyRequestID, requestID)
		r = r.WithContext(ctx)

		// Set response headers
		w.Header().Set("X-Request-ID", requestID)

		// Wrap response writer
		rw := NewResponseWriter(w)

		// Log request start
		if l.logger != nil {
			l.logger.Debug("request started",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		}

		// Call next handler
		next.ServeHTTP(rw, r)

		// Log request completion
		if l.logger != nil {
			duration := time.Since(start)
			l.logger.Info("request completed",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.StatusCode,
				"bytes", rw.BytesWritten,
				"duration_ms", duration.Milliseconds(),
			)
		}
	})
}

// CORSMiddleware adds CORS headers.
type CORSMiddleware struct {
	allowedOrigins []string
}

// NewCORSMiddleware creates a new CORS middleware.
func NewCORSMiddleware(allowedOrigins []string) *CORSMiddleware {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}
	return &CORSMiddleware{allowedOrigins: allowedOrigins}
}

// Handle is the HTTP middleware handler.
func (c *CORSMiddleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range c.allowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware recovers from panics.
type RecoveryMiddleware struct {
	logger  *slog.Logger
	metrics *Metrics
}

// NewRecoveryMiddleware creates a new recovery middleware.
func NewRecoveryMiddleware(logger *slog.Logger, metrics *Metrics) *RecoveryMiddleware {
	return &RecoveryMiddleware{logger: logger, metrics: metrics}
}

// Recover is the HTTP middleware handler.
func (rm *RecoveryMiddleware) Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				if rm.logger != nil {
					rm.logger.Error("panic recovered",
						"error", err,
						"path", r.URL.Path,
					)
				}
				if rm.metrics != nil {
					rm.metrics.RecordError("", "panic")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"type": "error",
					"error": map[string]any{
						"type":    "api_error",
						"message": "Internal server error",
					},
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

var requestCounter uint64
var requestCounterMu sync.Mutex

func generateRequestID() string {
	requestCounterMu.Lock()
	requestCounter++
	count := requestCounter
	requestCounterMu.Unlock()
	return "req_" + time.Now().Format("20060102150405") + "_" + uintToString(count)
}

func uintToString(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}
	return string(buf[i:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
