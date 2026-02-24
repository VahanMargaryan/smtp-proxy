package sanitizer

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// stripHeaders lists headers that reveal source/relay information.
var stripHeaders = map[string]bool{
	"received":              true,
	"x-mailer":              true,
	"x-originating-ip":      true,
	"x-sender":              true,
	"user-agent":            true,
	"x-google-dkim-signature": true,
	"x-gm-message-state":    true,
	"x-google-smtp-source":  true,
	"x-received":            true,
	"x-forwarded-to":        true,
	"x-forwarded-for":       true,
	"x-original-to":         true,
	"x-ms-exchange-organization-authas":          true,
	"x-ms-exchange-organization-authmechanism":   true,
	"x-ms-exchange-organization-authsource":      true,
	"dkim-signature":            true,
	"arc-seal":                  true,
	"arc-message-signature":     true,
	"arc-authentication-results": true,
	"authentication-results":    true,
	"return-path":               true,
	"delivered-to":              true,
	"x-spam-status":             true,
	"x-spam-score":              true,
	"x-spam-flag":               true,
}

// SanitizeMessage strips source-identifying headers from an email message
// and generates a new Message-ID. The message body passes through unmodified.
// The domain parameter is used for generating the new Message-ID.
func SanitizeMessage(raw []byte, domain string) []byte {
	// Normalize line endings to \r\n
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))

	// Split headers from body at the first blank line BEFORE normalizing body
	headerEnd := bytes.Index(raw, []byte("\n\n"))
	var headerPart, body []byte
	if headerEnd == -1 {
		headerPart = raw
		body = nil
	} else {
		headerPart = raw[:headerEnd]
		body = raw[headerEnd:] // includes the \n\n separator
	}

	// Only normalize headers to \r\n; leave body as-is then normalize it separately
	headerPart = bytes.ReplaceAll(headerPart, []byte("\n"), []byte("\r\n"))
	if body != nil {
		body = bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))
	}

	// Parse headers into entries (handling folded/continuation lines)
	lines := bytes.Split(headerPart, []byte("\r\n"))
	type header struct {
		name  string // lowercase
		lines [][]byte
	}
	var headers []header
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		// Continuation line (starts with space or tab)
		if line[0] == ' ' || line[0] == '\t' {
			if len(headers) > 0 {
				headers[len(headers)-1].lines = append(headers[len(headers)-1].lines, line)
			}
			continue
		}
		// New header
		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx <= 0 {
			// Malformed header line (no colon or colon at position 0), keep it
			headers = append(headers, header{name: "", lines: [][]byte{line}})
			continue
		}
		name := strings.ToLower(strings.TrimSpace(string(line[:colonIdx])))
		headers = append(headers, header{name: name, lines: [][]byte{line}})
	}

	// Rebuild headers, stripping blocked ones and replacing Message-ID
	var result bytes.Buffer
	messageIDFound := false
	newMessageID := fmt.Sprintf("Message-ID: <%d.%d@%s>\r\n", time.Now().UnixNano(), rand.Int64(), domain)

	for _, h := range headers {
		if stripHeaders[h.name] {
			continue
		}
		if h.name == "message-id" {
			messageIDFound = true
			result.WriteString(newMessageID)
			continue
		}
		for _, l := range h.lines {
			result.Write(l)
			result.WriteString("\r\n")
		}
	}

	if !messageIDFound {
		result.WriteString(newMessageID)
	}

	// Append body (includes the blank line separator)
	if body != nil {
		result.Write(body)
	} else {
		result.WriteString("\r\n")
	}

	return result.Bytes()
}
