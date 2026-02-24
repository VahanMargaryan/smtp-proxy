package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/joho/godotenv"

	"smtp-proxy/internal/config"
	"smtp-proxy/internal/proxy"
	"smtp-proxy/internal/relay"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	// Load .env file if present (ignore error if missing)
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	// Set up structured logging
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
	slog.SetDefault(slog.New(handler))

	backend := proxy.NewBackend(cfg, relay.Send)

	s := smtp.NewServer(backend)
	s.Addr = cfg.ListenAddr
	s.Domain = cfg.ServerDomain
	s.AllowInsecureAuth = true
	s.MaxMessageBytes = cfg.MaxMessageSize
	s.MaxRecipients = 100
	s.ReadTimeout = 60 * time.Second
	s.WriteTimeout = 60 * time.Second

	slog.Info("starting smtp proxy",
		"listen", cfg.ListenAddr,
		"upstream", cfg.DestHost,
		"upstream_port", cfg.DestPort,
		"from", cfg.DestFrom,
	)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe()
	}()

	// Wait for signal or server error
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		slog.Error("server error", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutting down...")
	}

	// Graceful shutdown with 30s timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}
