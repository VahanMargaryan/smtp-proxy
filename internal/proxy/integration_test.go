package proxy_test

import (
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"smtp-proxy/internal/config"
	"smtp-proxy/internal/proxy"
)

// mockUpstream captures messages received by a mock upstream SMTP server.
type mockUpstream struct {
	from       string
	recipients []string
	data       string
	received   chan struct{}
}

type mockUpstreamBackend struct {
	mock *mockUpstream
}

func (b *mockUpstreamBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &mockUpstreamSession{mock: b.mock}, nil
}

type mockUpstreamSession struct {
	mock *mockUpstream
}

func (s *mockUpstreamSession) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *mockUpstreamSession) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		if username != "upstream@example.com" || password != "upstreampass" {
			return smtp.ErrAuthFailed
		}
		return nil
	}), nil
}

func (s *mockUpstreamSession) Mail(from string, _ *smtp.MailOptions) error {
	s.mock.from = from
	return nil
}

func (s *mockUpstreamSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.mock.recipients = append(s.mock.recipients, to)
	return nil
}

func (s *mockUpstreamSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mock.data = string(b)
	close(s.mock.received)
	return nil
}

func (s *mockUpstreamSession) Reset() {}

func (s *mockUpstreamSession) Logout() error { return nil }

var _ smtp.AuthSession = (*mockUpstreamSession)(nil)

func startMockUpstream(t *testing.T) (*mockUpstream, string) {
	t.Helper()

	mock := &mockUpstream{received: make(chan struct{})}
	backend := &mockUpstreamBackend{mock: mock}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	s := smtp.NewServer(backend)
	s.Domain = "upstream.local"
	s.AllowInsecureAuth = true
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second

	go func() {
		_ = s.Serve(ln)
	}()

	t.Cleanup(func() {
		_ = s.Close()
	})

	return mock, ln.Addr().String()
}

func startProxy(t *testing.T, upstreamAddr string) string {
	t.Helper()

	host, portStr, _ := net.SplitHostPort(upstreamAddr)
	destPort, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("invalid port: %v", err)
	}

	cfg := &config.Config{
		ListenAddr:     "127.0.0.1:0",
		ProxyUsername:  "proxyuser",
		ProxyPassword:  "proxypass",
		DestHost:       host,
		DestPort:       destPort,
		DestUsername:   "upstream@example.com",
		DestPassword:   "upstreampass",
		DestFrom:       "upstream@example.com",
		DestDomain:     "example.com",
		ServerDomain:   "proxy.local",
		MaxMessageSize: 1024 * 1024,
	}

	// Use a plaintext relay for testing (mock upstream has no TLS)
	plainSend := func(cfg *config.Config, recipients []string, message []byte) error {
		addr := net.JoinHostPort(cfg.DestHost, portStr)
		client, err := smtp.Dial(addr)
		if err != nil {
			return err
		}
		defer client.Close()

		auth := sasl.NewPlainClient("", cfg.DestUsername, cfg.DestPassword)
		if err := client.Auth(auth); err != nil {
			return err
		}

		if err := client.SendMail(cfg.DestFrom, recipients, strings.NewReader(string(message))); err != nil {
			return err
		}

		return client.Quit()
	}

	backend := proxy.NewBackend(cfg, plainSend)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	s := smtp.NewServer(backend)
	s.Domain = cfg.ServerDomain
	s.AllowInsecureAuth = true
	s.MaxMessageBytes = cfg.MaxMessageSize
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second

	go func() {
		_ = s.Serve(ln)
	}()

	t.Cleanup(func() {
		_ = s.Close()
	})

	return ln.Addr().String()
}

func TestIntegration_FullFlow(t *testing.T) {
	mock, upstreamAddr := startMockUpstream(t)
	proxyAddr := startProxy(t, upstreamAddr)

	client, err := smtp.Dial(proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	auth := sasl.NewPlainClient("", "proxyuser", "proxypass")
	if err := client.Auth(auth); err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	msg := "From: app@thirdparty.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: Integration Test\r\n" +
		"Received: from thirdparty.internal (10.0.0.5)\r\n" +
		"X-Mailer: ThirdPartyApp/1.0\r\n" +
		"X-Originating-IP: 10.0.0.5\r\n" +
		"User-Agent: Mozilla/5.0\r\n" +
		"DKIM-Signature: v=1; d=thirdparty.com\r\n" +
		"Authentication-Results: spf=pass\r\n" +
		"Message-ID: <original@thirdparty.com>\r\n" +
		"\r\n" +
		"Hello from integration test!"

	err = client.SendMail("app@thirdparty.com", []string{"recipient@example.com"}, strings.NewReader(msg))
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	_ = client.Quit()

	select {
	case <-mock.received:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message at upstream")
	}

	// Verify envelope sender was replaced
	if mock.from != "upstream@example.com" {
		t.Errorf("expected envelope from upstream@example.com, got %s", mock.from)
	}

	// Verify recipients were forwarded
	if len(mock.recipients) != 1 || mock.recipients[0] != "recipient@example.com" {
		t.Errorf("unexpected recipients: %v", mock.recipients)
	}

	// Verify source headers were stripped
	strippedHeaders := []string{
		"Received:", "X-Mailer:", "X-Originating-IP:",
		"User-Agent:", "DKIM-Signature:", "Authentication-Results:",
	}
	for _, h := range strippedHeaders {
		if strings.Contains(mock.data, h) {
			t.Errorf("expected %s to be stripped", h)
		}
	}
	if strings.Contains(mock.data, "original@thirdparty.com") {
		t.Error("expected original Message-ID to be replaced")
	}

	// Verify content headers were preserved
	preservedHeaders := []string{
		"From: app@thirdparty.com",
		"To: recipient@example.com",
		"Subject: Integration Test",
	}
	for _, h := range preservedHeaders {
		if !strings.Contains(mock.data, h) {
			t.Errorf("expected %s to be preserved", h)
		}
	}

	// Verify new Message-ID
	if !strings.Contains(mock.data, "Message-ID: <") {
		t.Error("expected new Message-ID to be generated")
	}
	if !strings.Contains(mock.data, "@example.com>") {
		t.Error("expected Message-ID to use upstream domain")
	}

	// Verify body is intact
	if !strings.Contains(mock.data, "Hello from integration test!") {
		t.Error("expected message body to be intact")
	}
}

func TestIntegration_AuthFailure(t *testing.T) {
	_, upstreamAddr := startMockUpstream(t)
	proxyAddr := startProxy(t, upstreamAddr)

	client, err := smtp.Dial(proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	auth := sasl.NewPlainClient("", "wronguser", "wrongpass")
	err = client.Auth(auth)
	if err == nil {
		t.Fatal("expected auth to fail with wrong credentials")
	}

	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code != 535 {
		t.Errorf("expected SMTP 535 error, got %v", err)
	}
}

func TestIntegration_MultipleRecipients(t *testing.T) {
	mock, upstreamAddr := startMockUpstream(t)
	proxyAddr := startProxy(t, upstreamAddr)

	client, err := smtp.Dial(proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	auth := sasl.NewPlainClient("", "proxyuser", "proxypass")
	if err := client.Auth(auth); err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	msg := "From: sender@test.com\r\nSubject: Multi\r\n\r\nBody"
	recipients := []string{"r1@example.com", "r2@example.com", "r3@example.com"}

	err = client.SendMail("sender@test.com", recipients, strings.NewReader(msg))
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	_ = client.Quit()

	select {
	case <-mock.received:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	if len(mock.recipients) != 3 {
		t.Errorf("expected 3 recipients, got %d: %v", len(mock.recipients), mock.recipients)
	}
}
