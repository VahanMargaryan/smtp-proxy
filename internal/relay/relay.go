package relay

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"smtp-proxy/internal/config"
)

// SendFunc is the function signature for sending messages upstream.
// Extracted as a type to allow injection in tests.
type SendFunc func(cfg *config.Config, recipients []string, message []byte) error

// Send connects to the upstream SMTP server and forwards a sanitized message.
// The envelope sender is always replaced with cfg.DestFrom.
func Send(cfg *config.Config, recipients []string, message []byte) error {
	addr := fmt.Sprintf("%s:%d", cfg.DestHost, cfg.DestPort)
	tlsConfig := &tls.Config{ServerName: cfg.DestHost}

	slog.Debug("connecting to upstream", "addr", addr)

	var client *smtp.Client
	var err error

	switch cfg.DestPort {
	case 465:
		client, err = smtp.DialTLS(addr, tlsConfig)
	case 587:
		client, err = smtp.DialStartTLS(addr, tlsConfig)
	default:
		client, err = smtp.Dial(addr)
	}
	if err != nil {
		return fmt.Errorf("relay: connect to %s: %w", addr, err)
	}
	defer client.Close()

	auth := sasl.NewPlainClient("", cfg.DestUsername, cfg.DestPassword)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("relay: auth: %w", err)
	}

	slog.Debug("relay authenticated")

	if err := client.SendMail(cfg.DestFrom, recipients, bytes.NewReader(message)); err != nil {
		return fmt.Errorf("relay: send: %w", err)
	}

	slog.Debug("relay sent", "recipients", recipients)

	// Message was accepted by upstream. Quit error is non-fatal since
	// the message is already delivered.
	if err := client.Quit(); err != nil {
		slog.Warn("relay: quit error (message already accepted)", "error", err)
	}

	return nil
}
