package proxy

import (
	"crypto/subtle"
	"fmt"
	"io"
	"log/slog"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"smtp-proxy/internal/config"
	"smtp-proxy/internal/relay"
	"smtp-proxy/internal/sanitizer"
)

// Backend implements smtp.Backend.
type Backend struct {
	config *config.Config
	send   relay.SendFunc
}

// NewBackend creates a new proxy backend with the given config and send function.
func NewBackend(cfg *config.Config, send relay.SendFunc) *Backend {
	return &Backend{config: cfg, send: send}
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		config: b.config,
		send:   b.send,
	}, nil
}

// Session implements smtp.Session and smtp.AuthSession.
type Session struct {
	config     *config.Config
	send       relay.SendFunc
	auth       bool
	from       string
	recipients []string
}

// Ensure Session implements AuthSession at compile time.
var _ smtp.AuthSession = (*Session)(nil)

func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

func (s *Session) Auth(mech string) (sasl.Server, error) {
	validate := func(username, password string) error {
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(s.config.ProxyUsername)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(s.config.ProxyPassword)) == 1
		if !usernameMatch || !passwordMatch {
			slog.Warn("auth failed", "mechanism", mech)
			return smtp.ErrAuthFailed
		}
		s.auth = true
		slog.Info("client authenticated", "mechanism", mech)
		return nil
	}

	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return validate(username, password)
		}), nil
	case sasl.Login:
		return &loginServer{validate: validate}, nil
	default:
		return nil, smtp.ErrAuthUnknownMechanism
	}
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if !s.auth {
		return smtp.ErrAuthRequired
	}
	// Client from is accepted but always overridden by DestFrom for relay.
	// Clients may send MAIL FROM:<> or any valid address.
	s.from = from
	slog.Debug("MAIL FROM", "client_from", from, "relay_from", s.config.DestFrom)
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if !s.auth {
		return smtp.ErrAuthRequired
	}
	s.recipients = append(s.recipients, to)
	slog.Debug("RCPT TO", "to", to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	if !s.auth {
		return smtp.ErrAuthRequired
	}

	if len(s.recipients) == 0 {
		return &smtp.SMTPError{
			Code:         503,
			EnhancedCode: smtp.EnhancedCode{5, 5, 1},
			Message:      "No recipients specified",
		}
	}

	// Defense-in-depth: limit read size even though go-smtp enforces MaxMessageBytes
	raw, err := io.ReadAll(io.LimitReader(r, s.config.MaxMessageSize+1))
	if err != nil {
		slog.Error("failed to read message data", "error", err)
		return err
	}
	if int64(len(raw)) > s.config.MaxMessageSize {
		return &smtp.SMTPError{
			Code:         552,
			EnhancedCode: smtp.EnhancedCode{5, 3, 4},
			Message:      "Message too large",
		}
	}

	// Use DestFrom as envelope sender (falls back to DestUsername via config)
	envelopeFrom := s.config.DestFrom

	slog.Info("processing message",
		"client_from", s.from,
		"envelope_from", envelopeFrom,
		"recipients", s.recipients,
		"size", len(raw),
	)

	sanitized := sanitizer.SanitizeMessage(raw, s.config.DestDomain)

	if err := s.send(s.config, s.recipients, sanitized); err != nil {
		slog.Error("relay failed", "error", err)
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 0, 0},
			Message:      fmt.Sprintf("Temporary relay error: %v", err),
		}
	}

	slog.Info("message relayed", "from", envelopeFrom, "recipients", s.recipients)
	return nil
}

// Reset clears the mail transaction state.
// Per RFC 5321, RSET clears the sender and recipients but NOT the auth state.
func (s *Session) Reset() {
	s.from = ""
	s.recipients = nil
}

func (s *Session) Logout() error {
	return nil
}
