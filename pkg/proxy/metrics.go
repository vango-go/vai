package proxy

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the proxy.
type Metrics struct {
	registry *prometheus.Registry

	// Request metrics
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec

	// Token metrics
	TokensTotal *prometheus.CounterVec

	// Cost metrics
	CostUSDTotal *prometheus.CounterVec

	// Live session metrics
	LiveSessionsActive  prometheus.Gauge
	LiveSessionsTotal   *prometheus.CounterVec
	LiveSessionDuration *prometheus.HistogramVec
	LiveAudioBytesTotal *prometheus.CounterVec

	// Error metrics
	ErrorsTotal *prometheus.CounterVec

	// Rate limit metrics
	RateLimitHits *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with all Prometheus metrics registered.
func NewMetrics(namespace string) *Metrics {
	if namespace == "" {
		namespace = "vango"
	}

	registry := prometheus.NewRegistry()

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "requests_total",
			Help:      "Total number of API requests",
		},
		[]string{"provider", "model", "endpoint", "status"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		},
		[]string{"provider", "model", "endpoint"},
	)

	tokensTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tokens_total",
			Help:      "Total tokens processed",
		},
		[]string{"provider", "model", "direction"},
	)

	costUSDTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cost_usd_total",
			Help:      "Total cost in USD",
		},
		[]string{"provider", "model"},
	)

	liveSessionsActive := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "live_sessions_active",
			Help:      "Number of active live sessions",
		},
	)

	liveSessionsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "live_sessions_total",
			Help:      "Total number of live sessions",
		},
		[]string{"status"},
	)

	liveSessionDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "live_session_duration_seconds",
			Help:      "Live session duration in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"provider", "model"},
	)

	liveAudioBytesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "live_audio_bytes_total",
			Help:      "Total audio bytes processed in live sessions",
		},
		[]string{"direction"},
	)

	errorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "errors_total",
			Help:      "Total number of errors",
		},
		[]string{"provider", "error_type"},
	)

	rateLimitHits := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rate_limit_hits_total",
			Help:      "Total number of rate limit hits",
		},
		[]string{"user_id", "limit_type"},
	)

	// Register all metrics
	registry.MustRegister(
		requestsTotal,
		requestDuration,
		tokensTotal,
		costUSDTotal,
		liveSessionsActive,
		liveSessionsTotal,
		liveSessionDuration,
		liveAudioBytesTotal,
		errorsTotal,
		rateLimitHits,
	)

	return &Metrics{
		registry:            registry,
		RequestsTotal:       requestsTotal,
		RequestDuration:     requestDuration,
		TokensTotal:         tokensTotal,
		CostUSDTotal:        costUSDTotal,
		LiveSessionsActive:  liveSessionsActive,
		LiveSessionsTotal:   liveSessionsTotal,
		LiveSessionDuration: liveSessionDuration,
		LiveAudioBytesTotal: liveAudioBytesTotal,
		ErrorsTotal:         errorsTotal,
		RateLimitHits:       rateLimitHits,
	}
}

// Handler returns an HTTP handler for the metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RecordRequest records a completed request.
func (m *Metrics) RecordRequest(provider, model, endpoint, status string, duration time.Duration) {
	m.RequestsTotal.WithLabelValues(provider, model, endpoint, status).Inc()
	m.RequestDuration.WithLabelValues(provider, model, endpoint).Observe(duration.Seconds())
}

// RecordTokens records token usage.
func (m *Metrics) RecordTokens(provider, model string, inputTokens, outputTokens int) {
	if inputTokens > 0 {
		m.TokensTotal.WithLabelValues(provider, model, "input").Add(float64(inputTokens))
	}
	if outputTokens > 0 {
		m.TokensTotal.WithLabelValues(provider, model, "output").Add(float64(outputTokens))
	}
}

// RecordCost records request cost.
func (m *Metrics) RecordCost(provider, model string, costUSD float64) {
	if costUSD > 0 {
		m.CostUSDTotal.WithLabelValues(provider, model).Add(costUSD)
	}
}

// RecordLiveSessionStart records a new live session starting.
func (m *Metrics) RecordLiveSessionStart() {
	m.LiveSessionsActive.Inc()
}

// RecordLiveSessionEnd records a live session ending.
func (m *Metrics) RecordLiveSessionEnd(provider, model, status string, duration time.Duration) {
	m.LiveSessionsActive.Dec()
	m.LiveSessionsTotal.WithLabelValues(status).Inc()
	m.LiveSessionDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
}

// RecordLiveAudio records audio bytes in a live session.
func (m *Metrics) RecordLiveAudio(direction string, bytes int) {
	m.LiveAudioBytesTotal.WithLabelValues(direction).Add(float64(bytes))
}

// RecordError records an error.
func (m *Metrics) RecordError(provider, errorType string) {
	m.ErrorsTotal.WithLabelValues(provider, errorType).Inc()
}

// RecordRateLimitHit records a rate limit hit.
func (m *Metrics) RecordRateLimitHit(userID, limitType string) {
	m.RateLimitHits.WithLabelValues(userID, limitType).Inc()
}

// ResponseWriter wraps http.ResponseWriter to capture status code and size.
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode  int
	BytesWritten int
}

// NewResponseWriter creates a new ResponseWriter.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code.
func (rw *ResponseWriter) WriteHeader(code int) {
	rw.StatusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures bytes written.
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.BytesWritten += n
	return n, err
}

// Flush implements http.Flusher.
func (rw *ResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrades.
func (rw *ResponseWriter) Hijack() (c interface{}, rw2 interface{}, err error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// StatusString returns the status code as a string.
func (rw *ResponseWriter) StatusString() string {
	return strconv.Itoa(rw.StatusCode)
}
