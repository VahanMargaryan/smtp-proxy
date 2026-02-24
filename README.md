# smtp-proxy

A lightweight SMTP proxy relay written in Go. It accepts incoming SMTP connections from third-party applications and forwards emails through a configured upstream SMTP server, stripping source-identifying headers for privacy.

## How It Works

```
Third-party App  ──SMTP──▶  smtp-proxy  ──SMTP──▶  Upstream Server  ──▶  Recipient
                           (sanitize +
                            re-envelope)
```

1. Your app connects to the proxy using the proxy credentials
2. The proxy receives the email and strips headers that reveal the source (Received, X-Mailer, User-Agent, DKIM-Signature, etc.)
3. The envelope sender is replaced with the configured upstream address
4. A new Message-ID is generated
5. The sanitized email is forwarded to the upstream SMTP server

## Quick Start

```bash
# Clone and build
git clone <repo-url> && cd smtp-proxy
go build -o smtp-proxy .

# Configure
cp .env.example .env
# Edit .env with your upstream SMTP credentials and proxy credentials

# Run
./smtp-proxy
```

Then configure your application to use the proxy as its SMTP server:
- **Host**: `localhost` (or wherever the proxy runs)
- **Port**: `2525` (default)
- **Username**: value of `SMTP_PROXY_USERNAME`
- **Password**: value of `SMTP_PROXY_PASSWORD`

## Docker

```bash
# Build
docker build -t smtp-proxy .

# Run (pass env vars via file)
docker run --rm --env-file .env -p 2525:2525 smtp-proxy
```

The image uses a multi-stage build with a `scratch` base, resulting in a ~10-15MB final image containing only the static binary and CA certificates.

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SMTP_LISTEN_ADDR` | No | `:2525` | Address and port the proxy listens on |
| `SMTP_PROXY_USERNAME` | Yes | - | Username for apps connecting to the proxy |
| `SMTP_PROXY_PASSWORD` | Yes | - | Password for apps connecting to the proxy |
| `SMTP_DEST_HOST` | Yes | - | Upstream SMTP server hostname |
| `SMTP_DEST_PORT` | No | `587` | Upstream SMTP server port |
| `SMTP_DEST_USERNAME` | Yes | - | Username to authenticate with upstream |
| `SMTP_DEST_PASSWORD` | Yes | - | Password to authenticate with upstream |
| `SMTP_DEST_FROM` | No | `SMTP_DEST_USERNAME` | Envelope sender for all outgoing emails |
| `SMTP_SERVER_DOMAIN` | No | `localhost` | Domain used in EHLO greeting |
| `SMTP_MAX_MESSAGE_SIZE` | No | `26214400` (25MB) | Maximum message size in bytes |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |

## TLS Behavior

The proxy auto-selects the TLS mode based on the upstream port:

| Port | Mode |
|------|------|
| 465 | Implicit TLS |
| 587 | STARTTLS |
| Other | Plain (no TLS) |

## Headers Stripped

The following headers are removed before forwarding to protect source identity:

- `Received` (all instances)
- `X-Mailer`, `X-Originating-IP`, `X-Sender`, `User-Agent`
- `X-Received`, `X-Forwarded-To`, `X-Forwarded-For`, `X-Original-To`
- `DKIM-Signature`, `Authentication-Results`
- `ARC-Seal`, `ARC-Message-Signature`, `ARC-Authentication-Results`
- `Return-Path`, `Delivered-To`
- `X-Spam-Status`, `X-Spam-Score`, `X-Spam-Flag`
- `X-Google-DKIM-Signature`, `X-Gm-Message-State`, `X-Google-Smtp-Source`
- `X-MS-Exchange-Organization-AuthAs`, `X-MS-Exchange-Organization-AuthMechanism`, `X-MS-Exchange-Organization-AuthSource`

Additionally, `Message-ID` is replaced with a newly generated one.

## Authentication

The proxy supports PLAIN and LOGIN authentication mechanisms. Third-party apps must authenticate with the proxy credentials before sending mail.

## Project Structure

```
smtp-proxy/
├── main.go                              # Entry point
├── internal/
│   ├── config/
│   │   ├── config.go                    # Configuration loading from .env
│   │   └── config_test.go
│   ├── proxy/
│   │   ├── proxy.go                     # SMTP backend and session
│   │   ├── login.go                     # LOGIN SASL mechanism
│   │   ├── proxy_test.go
│   │   └── integration_test.go
│   ├── relay/
│   │   ├── relay.go                     # Upstream SMTP client
│   │   └── relay_test.go
│   └── sanitizer/
│       ├── sanitizer.go                 # Email header stripping
│       └── sanitizer_test.go
├── .env.example
├── .gitignore
├── CLAUDE.md
└── README.md
```

## Security Considerations

- The local proxy listens in **plaintext** with `AllowInsecureAuth = true`. It is intended for local/trusted network use (localhost, LAN, Docker network). Do not expose it to the public internet without a TLS terminator in front.
- Upstream connections use TLS/STARTTLS based on port (see above).
- Proxy credentials should be strong and unique.
- Credential comparison uses constant-time comparison to prevent timing attacks.
- Relay errors are wrapped in generic SMTP status codes to avoid leaking upstream server details.

## License

MIT
