package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vango-go/vai/pkg/proxy"
)

func main() {
	configPath := flag.String("config", "", "Path to config file (yaml/json)")
	flag.Parse()

	cfg, err := proxy.LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg)
	slog.SetDefault(logger)

	options := []proxy.ConfigOption{
		proxy.WithHost(cfg.Host),
		proxy.WithPort(cfg.Port),
		proxy.WithLogger(logger),
		proxy.WithAuthMode(cfg.AuthMode),
		proxy.WithAPIKeys(cfg.APIKeys),
		proxy.WithProviderKeys(cfg.ProviderKeys),
		proxy.WithRateLimitConfig(cfg.RateLimit),
		proxy.WithObservability(cfg.Observability),
		proxy.WithAllowedOrigins(cfg.AllowedOrigins),
		proxy.WithRequestBodyLimit(cfg.MaxRequestBodyBytes),
		proxy.WithTimeouts(cfg.ReadTimeout, cfg.WriteTimeout, cfg.ShutdownTimeout),
	}
	if cfg.TLSEnabled {
		options = append(options, proxy.WithTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
	}

	server, err := proxy.NewServer(options...)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := server.Start(); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

func setupLogger(cfg *proxy.Config) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Observability.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.Observability.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
