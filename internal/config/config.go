package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Local proxy server
	ListenAddr    string
	ProxyUsername string
	ProxyPassword string

	// Upstream SMTP server
	DestHost     string
	DestPort     int
	DestUsername string
	DestPassword string
	DestFrom     string
	DestDomain   string // extracted from DestFrom

	// Optional
	ServerDomain   string
	MaxMessageSize int64
	LogLevel       slog.Level
}

func Load() (*Config, error) {
	cfg := &Config{
		ListenAddr:     envOrDefault("SMTP_LISTEN_ADDR", ":2525"),
		ServerDomain:   envOrDefault("SMTP_SERVER_DOMAIN", "localhost"),
		MaxMessageSize: 25 * 1024 * 1024, // 25MB
		LogLevel:       slog.LevelInfo,
	}

	// Required fields — use a slice for deterministic error reporting
	type required struct {
		env string
		ptr *string
	}
	requiredVars := []required{
		{"SMTP_PROXY_USERNAME", &cfg.ProxyUsername},
		{"SMTP_PROXY_PASSWORD", &cfg.ProxyPassword},
		{"SMTP_DEST_HOST", &cfg.DestHost},
		{"SMTP_DEST_USERNAME", &cfg.DestUsername},
		{"SMTP_DEST_PASSWORD", &cfg.DestPassword},
	}

	var missing []string
	for _, r := range requiredVars {
		val := os.Getenv(r.env)
		if val == "" {
			missing = append(missing, r.env)
			continue
		}
		*r.ptr = val
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("required environment variables not set: %s", strings.Join(missing, ", "))
	}

	// Destination port
	portStr := envOrDefault("SMTP_DEST_PORT", "587")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SMTP_DEST_PORT: %s", portStr)
	}
	cfg.DestPort = port

	// From address defaults to dest username
	cfg.DestFrom = envOrDefault("SMTP_DEST_FROM", cfg.DestUsername)

	// Validate DestFrom contains @
	if !strings.Contains(cfg.DestFrom, "@") {
		return nil, fmt.Errorf("SMTP_DEST_FROM must be a valid email address: %s", cfg.DestFrom)
	}

	// Extract domain from DestFrom for Message-ID generation
	cfg.DestDomain = cfg.ServerDomain
	if at := strings.LastIndex(cfg.DestFrom, "@"); at >= 0 {
		cfg.DestDomain = cfg.DestFrom[at+1:]
	}

	// Max message size
	if v := os.Getenv("SMTP_MAX_MESSAGE_SIZE"); v != "" {
		size, err := strconv.ParseInt(v, 10, 64)
		if err != nil || size < 1 {
			return nil, fmt.Errorf("invalid SMTP_MAX_MESSAGE_SIZE: %s", v)
		}
		cfg.MaxMessageSize = size
	}

	// Log level
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return nil, fmt.Errorf("invalid LOG_LEVEL: %s (must be debug, info, warn, or error)", v)
		}
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
