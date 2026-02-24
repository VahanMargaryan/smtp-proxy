# smtp-proxy

SMTP proxy relay that accepts incoming SMTP connections and forwards emails through a configured upstream SMTP server with header sanitization.

## Build & Run

```bash
go build -o smtp-proxy .
./smtp-proxy
# or directly:
go run .
```

## Test

```bash
go test ./...                        # all tests
go test -race ./...                  # with race detector (recommended)
go test -v ./internal/sanitizer/...  # single package
go test -run TestIntegration ./...   # single test pattern
go test -cover ./...                 # coverage summary
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out  # coverage report
```

## Lint

```bash
go vet ./...
gofmt -s -w .
```

## Project Structure

```
main.go                          - Entry point: .env loading, server setup, graceful shutdown
internal/
  config/config.go               - Configuration struct and .env loading
  proxy/proxy.go                 - SMTP Backend and Session (core proxy logic)
  proxy/login.go                 - LOGIN SASL server implementation
  relay/relay.go                 - Upstream SMTP client: connect, authenticate, forward
  sanitizer/sanitizer.go         - Email header stripping/sanitization
```

## Dependencies

- `github.com/emersion/go-smtp` - SMTP server and client
- `github.com/emersion/go-sasl` - SASL authentication mechanisms
- `github.com/joho/godotenv` - .env file loading

## Code Conventions

- Standard Go formatting (gofmt)
- `log/slog` for structured logging
- Errors wrapped with `fmt.Errorf("context: %w", err)`
- No `any` type usage
- `internal/` packages for all private application code
- `relay.SendFunc` type for dependency injection in tests
- Constant-time credential comparison via `crypto/subtle`
