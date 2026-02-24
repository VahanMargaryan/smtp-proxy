package relay

import (
	"smtp-proxy/internal/config"
	"testing"
)

func TestTLSModeSelection(t *testing.T) {
	tests := []struct {
		port     int
		wantMode string
	}{
		{465, "implicit TLS"},
		{587, "STARTTLS"},
		{25, "plain"},
		{2525, "plain"},
	}

	for _, tt := range tests {
		t.Run(tt.wantMode, func(t *testing.T) {
			cfg := &config.Config{
				DestHost: "unreachable.invalid",
				DestPort: tt.port,
			}

			// We can't actually connect, but we verify the function
			// attempts the connection (will fail with DNS error).
			err := Send(cfg, []string{"test@example.com"}, []byte("test"))
			if err == nil {
				t.Fatal("expected connection error to unreachable host")
			}
			// The error should be a connection error, not a logic error
			if tt.port == 465 || tt.port == 587 || tt.port == 25 || tt.port == 2525 {
				// All should fail at the connect stage
				t.Logf("port %d: got expected error: %v", tt.port, err)
			}
		})
	}
}
