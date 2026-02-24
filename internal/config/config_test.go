package config

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SMTP_PROXY_USERNAME", "testuser")
	t.Setenv("SMTP_PROXY_PASSWORD", "testpass")
	t.Setenv("SMTP_DEST_HOST", "smtp.example.com")
	t.Setenv("SMTP_DEST_USERNAME", "user@example.com")
	t.Setenv("SMTP_DEST_PASSWORD", "destpass")
}

func TestLoad_Defaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddr != ":2525" {
		t.Errorf("expected default ListenAddr :2525, got %s", cfg.ListenAddr)
	}
	if cfg.ServerDomain != "localhost" {
		t.Errorf("expected default ServerDomain localhost, got %s", cfg.ServerDomain)
	}
	if cfg.DestPort != 587 {
		t.Errorf("expected default DestPort 587, got %d", cfg.DestPort)
	}
	if cfg.MaxMessageSize != 25*1024*1024 {
		t.Errorf("expected default MaxMessageSize 25MB, got %d", cfg.MaxMessageSize)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("expected default LogLevel info, got %v", cfg.LogLevel)
	}
	if cfg.DestFrom != "user@example.com" {
		t.Errorf("expected DestFrom to default to DestUsername, got %s", cfg.DestFrom)
	}
	if cfg.DestDomain != "example.com" {
		t.Errorf("expected DestDomain example.com, got %s", cfg.DestDomain)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	// Clear all env vars
	for _, key := range []string{
		"SMTP_PROXY_USERNAME", "SMTP_PROXY_PASSWORD",
		"SMTP_DEST_HOST", "SMTP_DEST_USERNAME", "SMTP_DEST_PASSWORD",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required vars")
	}
	// Should report all missing vars
	if !strings.Contains(err.Error(), "SMTP_PROXY_USERNAME") {
		t.Error("expected error to mention SMTP_PROXY_USERNAME")
	}
	if !strings.Contains(err.Error(), "SMTP_DEST_HOST") {
		t.Error("expected error to mention SMTP_DEST_HOST")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_DEST_PORT", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
	if !strings.Contains(err.Error(), "SMTP_DEST_PORT") {
		t.Errorf("expected error to mention SMTP_DEST_PORT, got %v", err)
	}
}

func TestLoad_PortOutOfRange(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_DEST_PORT", "99999")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Errorf("expected error to mention LOG_LEVEL, got %v", err)
	}
}

func TestLoad_ValidLogLevels(t *testing.T) {
	levels := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	for name, expected := range levels {
		t.Run(name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("LOG_LEVEL", name)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.LogLevel != expected {
				t.Errorf("expected log level %v, got %v", expected, cfg.LogLevel)
			}
		})
	}
}

func TestLoad_InvalidDestFrom(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_DEST_FROM", "not-an-email")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SMTP_DEST_FROM")
	}
	if !strings.Contains(err.Error(), "SMTP_DEST_FROM") {
		t.Errorf("expected error to mention SMTP_DEST_FROM, got %v", err)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_LISTEN_ADDR", ":5587")
	t.Setenv("SMTP_DEST_PORT", "465")
	t.Setenv("SMTP_DEST_FROM", "sender@custom.com")
	t.Setenv("SMTP_SERVER_DOMAIN", "mail.proxy.com")
	t.Setenv("SMTP_MAX_MESSAGE_SIZE", "1048576")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddr != ":5587" {
		t.Errorf("expected ListenAddr :5587, got %s", cfg.ListenAddr)
	}
	if cfg.DestPort != 465 {
		t.Errorf("expected DestPort 465, got %d", cfg.DestPort)
	}
	if cfg.DestFrom != "sender@custom.com" {
		t.Errorf("expected DestFrom sender@custom.com, got %s", cfg.DestFrom)
	}
	if cfg.DestDomain != "custom.com" {
		t.Errorf("expected DestDomain custom.com, got %s", cfg.DestDomain)
	}
	if cfg.ServerDomain != "mail.proxy.com" {
		t.Errorf("expected ServerDomain mail.proxy.com, got %s", cfg.ServerDomain)
	}
	if cfg.MaxMessageSize != 1048576 {
		t.Errorf("expected MaxMessageSize 1048576, got %d", cfg.MaxMessageSize)
	}
}

func TestLoad_InvalidMaxMessageSize(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SMTP_MAX_MESSAGE_SIZE", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid max message size")
	}
}
