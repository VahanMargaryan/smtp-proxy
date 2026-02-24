package proxy

import (
	"errors"
	"strings"
	"testing"

	"github.com/emersion/go-smtp"

	"smtp-proxy/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		ListenAddr:     ":0",
		ProxyUsername:  "testuser",
		ProxyPassword:  "testpass",
		DestHost:       "smtp.example.com",
		DestPort:       587,
		DestUsername:   "upstream@example.com",
		DestPassword:   "upstreampass",
		DestFrom:       "upstream@example.com",
		DestDomain:     "example.com",
		ServerDomain:   "localhost",
		MaxMessageSize: 25 * 1024 * 1024,
	}
}

func noopSend(_ *config.Config, _ []string, _ []byte) error {
	return nil
}

func TestSession_AuthRequired(t *testing.T) {
	cfg := testConfig()
	session := &Session{config: cfg, send: noopSend}

	err := session.Mail("sender@example.com", nil)
	if !errors.Is(err, smtp.ErrAuthRequired) {
		t.Errorf("expected ErrAuthRequired for Mail, got %v", err)
	}

	err = session.Rcpt("recipient@example.com", nil)
	if !errors.Is(err, smtp.ErrAuthRequired) {
		t.Errorf("expected ErrAuthRequired for Rcpt, got %v", err)
	}

	err = session.Data(strings.NewReader("test"))
	if !errors.Is(err, smtp.ErrAuthRequired) {
		t.Errorf("expected ErrAuthRequired for Data, got %v", err)
	}
}

func TestSession_AuthMechanisms(t *testing.T) {
	cfg := testConfig()
	session := &Session{config: cfg, send: noopSend}

	mechs := session.AuthMechanisms()
	if len(mechs) != 2 {
		t.Fatalf("expected 2 auth mechanisms, got %d", len(mechs))
	}
	if mechs[0] != "PLAIN" {
		t.Errorf("expected PLAIN, got %s", mechs[0])
	}
	if mechs[1] != "LOGIN" {
		t.Errorf("expected LOGIN, got %s", mechs[1])
	}
}

func TestSession_Reset(t *testing.T) {
	cfg := testConfig()
	session := &Session{
		config:     cfg,
		send:       noopSend,
		auth:       true,
		from:       "sender@example.com",
		recipients: []string{"r1@example.com", "r2@example.com"},
	}

	session.Reset()

	if session.from != "" {
		t.Error("expected from to be cleared")
	}
	if session.recipients != nil {
		t.Error("expected recipients to be cleared")
	}
	if !session.auth {
		t.Error("expected auth to persist through reset (per RFC 5321)")
	}
}

func TestSession_DataCallsSend(t *testing.T) {
	cfg := testConfig()

	var sentRecipients []string
	var sentMessage []byte
	mockSend := func(c *config.Config, recipients []string, message []byte) error {
		sentRecipients = recipients
		sentMessage = message
		return nil
	}

	session := &Session{
		config: cfg,
		send:   mockSend,
		auth:   true,
	}

	_ = session.Mail("sender@test.com", nil)
	_ = session.Rcpt("r1@example.com", nil)
	_ = session.Rcpt("r2@example.com", nil)

	msg := "From: sender@test.com\r\nSubject: Test\r\n\r\nBody"
	err := session.Data(strings.NewReader(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sentRecipients) != 2 {
		t.Fatalf("expected 2 recipients, got %d", len(sentRecipients))
	}
	if sentRecipients[0] != "r1@example.com" {
		t.Errorf("expected r1@example.com, got %s", sentRecipients[0])
	}
	if !strings.Contains(string(sentMessage), "Body") {
		t.Error("expected message body to be present")
	}
}

func TestSession_DataRelayError(t *testing.T) {
	cfg := testConfig()
	mockSend := func(_ *config.Config, _ []string, _ []byte) error {
		return errors.New("upstream down")
	}

	session := &Session{
		config: cfg,
		send:   mockSend,
		auth:   true,
	}

	_ = session.Mail("sender@test.com", nil)
	_ = session.Rcpt("r1@example.com", nil)

	msg := "From: sender@test.com\r\nSubject: Test\r\n\r\nBody"
	err := session.Data(strings.NewReader(msg))
	if err == nil {
		t.Fatal("expected error from relay failure")
	}

	// Error should be wrapped as SMTP 451
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) {
		t.Fatalf("expected smtp.SMTPError, got %T: %v", err, err)
	}
	if smtpErr.Code != 451 {
		t.Errorf("expected SMTP code 451, got %d", smtpErr.Code)
	}
}

func TestSession_DataNoRecipients(t *testing.T) {
	cfg := testConfig()
	session := &Session{
		config: cfg,
		send:   noopSend,
		auth:   true,
	}

	_ = session.Mail("sender@test.com", nil)
	// No Rcpt call

	msg := "From: sender@test.com\r\nSubject: Test\r\n\r\nBody"
	err := session.Data(strings.NewReader(msg))
	if err == nil {
		t.Fatal("expected error when no recipients")
	}

	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) {
		t.Fatalf("expected smtp.SMTPError, got %T: %v", err, err)
	}
	if smtpErr.Code != 503 {
		t.Errorf("expected SMTP code 503, got %d", smtpErr.Code)
	}
}

func TestBackend_NewSession(t *testing.T) {
	cfg := testConfig()
	backend := NewBackend(cfg, noopSend)

	session, err := backend.NewSession(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session == nil {
		t.Fatal("expected non-nil session")
	}
}

func TestLoginServer_FullHandshake(t *testing.T) {
	var authedUser, authedPass string
	ls := &loginServer{
		validate: func(username, password string) error {
			authedUser = username
			authedPass = password
			return nil
		},
	}

	// Step 0: initial challenge
	challenge, done, err := ls.Next(nil)
	if err != nil {
		t.Fatalf("step 0 error: %v", err)
	}
	if done {
		t.Fatal("step 0 should not be done")
	}
	if string(challenge) != "Username:" {
		t.Errorf("expected 'Username:' challenge, got %q", challenge)
	}

	// Step 1: send username
	challenge, done, err = ls.Next([]byte("testuser"))
	if err != nil {
		t.Fatalf("step 1 error: %v", err)
	}
	if done {
		t.Fatal("step 1 should not be done")
	}
	if string(challenge) != "Password:" {
		t.Errorf("expected 'Password:' challenge, got %q", challenge)
	}

	// Step 2: send password
	_, done, err = ls.Next([]byte("testpass"))
	if err != nil {
		t.Fatalf("step 2 error: %v", err)
	}
	if !done {
		t.Fatal("step 2 should be done")
	}
	if authedUser != "testuser" {
		t.Errorf("expected username testuser, got %s", authedUser)
	}
	if authedPass != "testpass" {
		t.Errorf("expected password testpass, got %s", authedPass)
	}
}

func TestLoginServer_InvalidCredentials(t *testing.T) {
	ls := &loginServer{
		validate: func(username, password string) error {
			return smtp.ErrAuthFailed
		},
	}

	ls.Next(nil)                  // Username challenge
	ls.Next([]byte("wronguser"))  // Password challenge
	_, _, err := ls.Next([]byte("wrongpass"))
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !errors.Is(err, smtp.ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

func TestLoginServer_ExtraStep(t *testing.T) {
	ls := &loginServer{
		validate: func(username, password string) error { return nil },
	}

	ls.Next(nil)
	ls.Next([]byte("user"))
	ls.Next([]byte("pass"))

	// Extra step should error
	_, _, err := ls.Next([]byte("extra"))
	if err == nil {
		t.Fatal("expected error on extra step")
	}
}
