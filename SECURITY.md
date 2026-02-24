# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

## Reporting a Vulnerability

If you discover a security vulnerability in smtp-proxy, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please send an email with details of the vulnerability. Include:

- A description of the vulnerability
- Steps to reproduce the issue
- Potential impact
- Suggested fix (if any)

You should receive a response within 72 hours acknowledging your report. We will work with you to understand the issue and coordinate a fix before any public disclosure.

## Security Considerations

- The proxy listens in **plaintext** and is intended for local/trusted network use (localhost, LAN, Docker network). Do not expose it to the public internet without a TLS terminator.
- Upstream connections use TLS/STARTTLS based on the configured port (465 for implicit TLS, 587 for STARTTLS).
- Proxy credentials are compared using constant-time comparison (`crypto/subtle`) to prevent timing attacks.
- Relay errors are wrapped in generic SMTP status codes to avoid leaking upstream server details.
- Source-identifying headers (Received, X-Mailer, DKIM-Signature, etc.) are stripped before forwarding.
