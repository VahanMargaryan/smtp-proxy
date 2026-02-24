package sanitizer

import (
	"strings"
	"testing"
)

func TestSanitizeMessage_StripsReceivedHeaders(t *testing.T) {
	raw := "Received: from mail.example.com\r\n" +
		"Received: from smtp.local\r\n" +
		"From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Hello, World!"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if strings.Contains(result, "Received:") {
		t.Error("expected Received headers to be stripped")
	}
	if !strings.Contains(result, "From: sender@example.com") {
		t.Error("expected From header to be preserved")
	}
	if !strings.Contains(result, "To: recipient@example.com") {
		t.Error("expected To header to be preserved")
	}
	if !strings.Contains(result, "Subject: Test") {
		t.Error("expected Subject header to be preserved")
	}
	if !strings.Contains(result, "Hello, World!") {
		t.Error("expected body to be preserved")
	}
}

func TestSanitizeMessage_StripsSourceHeaders(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"X-Mailer: ThunderBird 91.0\r\n" +
		"X-Originating-IP: 192.168.1.100\r\n" +
		"User-Agent: Mozilla/5.0\r\n" +
		"X-Google-DKIM-Signature: v=1\r\n" +
		"DKIM-Signature: v=1; d=example.com\r\n" +
		"Authentication-Results: spf=pass\r\n" +
		"Return-Path: <bounce@source.com>\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	stripExpected := []string{
		"X-Mailer:",
		"X-Originating-IP:",
		"User-Agent:",
		"X-Google-DKIM-Signature:",
		"DKIM-Signature:",
		"Authentication-Results:",
		"Return-Path:",
	}
	for _, h := range stripExpected {
		if strings.Contains(result, h) {
			t.Errorf("expected %s to be stripped", h)
		}
	}
}

func TestSanitizeMessage_PreservesContentHeaders(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Cc: cc@example.com\r\n" +
		"Subject: Important\r\n" +
		"Date: Mon, 1 Jan 2024 00:00:00 +0000\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 7bit\r\n" +
		"Reply-To: reply@example.com\r\n" +
		"\r\n" +
		"Content here"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	preserveExpected := []string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Cc: cc@example.com",
		"Subject: Important",
		"MIME-Version: 1.0",
		"Content-Type: text/plain",
		"Content-Transfer-Encoding: 7bit",
		"Reply-To: reply@example.com",
	}
	for _, h := range preserveExpected {
		if !strings.Contains(result, h) {
			t.Errorf("expected %s to be preserved", h)
		}
	}
}

func TestSanitizeMessage_GeneratesMessageID(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if !strings.Contains(result, "Message-ID: <") {
		t.Error("expected Message-ID to be generated")
	}
	if !strings.Contains(result, "@proxy.local>") {
		t.Error("expected Message-ID to use proxy domain")
	}
}

func TestSanitizeMessage_ReplacesExistingMessageID(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"Message-ID: <original@source.local>\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if strings.Contains(result, "original@source.local") {
		t.Error("expected original Message-ID to be replaced")
	}
	if !strings.Contains(result, "@proxy.local>") {
		t.Error("expected new Message-ID with proxy domain")
	}
	count := strings.Count(result, "Message-ID:")
	if count != 1 {
		t.Errorf("expected exactly 1 Message-ID, got %d", count)
	}
}

func TestSanitizeMessage_HandlesFoldedHeaders(t *testing.T) {
	raw := "Received: from mail.example.com\r\n" +
		" by smtp.relay.com with ESMTP\r\n" +
		" id abc123\r\n" +
		"From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if strings.Contains(result, "mail.example.com") {
		t.Error("expected folded Received header to be stripped")
	}
	if strings.Contains(result, "smtp.relay.com") {
		t.Error("expected folded continuation lines to be stripped")
	}
	if !strings.Contains(result, "From: sender@example.com") {
		t.Error("expected From header to be preserved")
	}
}

func TestSanitizeMessage_FoldedHeaderWithColon(t *testing.T) {
	raw := "Received: from mail.example.com\r\n" +
		"\tby relay.example.com; Mon, 1 Jan 2024 00:00:00 +0000\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if strings.Contains(result, "relay.example.com") {
		t.Error("expected folded continuation with colon to be stripped with parent header")
	}
	if !strings.Contains(result, "Subject: Test") {
		t.Error("expected Subject to be preserved")
	}
}

func TestSanitizeMessage_PreservesBody(t *testing.T) {
	body := "This is a multiline\r\nbody with special chars: <>&\"\r\nand more lines."
	raw := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		body

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if !strings.Contains(result, body) {
		t.Error("expected body to be preserved exactly")
	}
}

func TestSanitizeMessage_BodyResemblingHeaders(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Received: this is body text, not a header\r\n" +
		"X-Mailer: also body text"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if !strings.Contains(result, "Received: this is body text") {
		t.Error("expected body text resembling headers to be preserved")
	}
	if !strings.Contains(result, "X-Mailer: also body text") {
		t.Error("expected body text with X-Mailer to be preserved")
	}
}

func TestSanitizeMessage_EmptyBody(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if !strings.Contains(result, "From: sender@example.com") {
		t.Error("expected headers to be preserved")
	}
	if !strings.HasSuffix(result, "\r\n\r\n") {
		t.Error("expected message to end with blank line separator")
	}
}

func TestSanitizeMessage_HeadersOnly(t *testing.T) {
	raw := "From: sender@example.com\r\n" +
		"Subject: Test"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if !strings.Contains(result, "From: sender@example.com") {
		t.Error("expected From header to be preserved")
	}
	if !strings.Contains(result, "Subject: Test") {
		t.Error("expected Subject header to be preserved")
	}
}

func TestSanitizeMessage_LFLineEndings(t *testing.T) {
	raw := "Received: from mail.example.com\n" +
		"From: sender@example.com\n" +
		"Subject: Test\n" +
		"\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if strings.Contains(result, "Received:") {
		t.Error("expected Received header to be stripped with LF endings")
	}
	if !strings.Contains(result, "From: sender@example.com") {
		t.Error("expected From header to be preserved")
	}
}

func TestSanitizeMessage_CaseInsensitiveHeaders(t *testing.T) {
	raw := "RECEIVED: from mail.example.com\r\n" +
		"x-mailer: ThunderBird\r\n" +
		"X-ORIGINATING-IP: 1.2.3.4\r\n" +
		"From: sender@example.com\r\n" +
		"Subject: Test\r\n" +
		"\r\n" +
		"Body"

	result := string(SanitizeMessage([]byte(raw), "proxy.local"))

	if strings.Contains(result, "RECEIVED:") {
		t.Error("expected uppercase RECEIVED to be stripped")
	}
	if strings.Contains(result, "x-mailer:") {
		t.Error("expected lowercase x-mailer to be stripped")
	}
	if strings.Contains(result, "X-ORIGINATING-IP:") {
		t.Error("expected X-ORIGINATING-IP to be stripped")
	}
}

func TestSanitizeMessage_UniqueMessageIDs(t *testing.T) {
	raw := "From: sender@example.com\r\nSubject: Test\r\n\r\nBody"

	result1 := string(SanitizeMessage([]byte(raw), "proxy.local"))
	result2 := string(SanitizeMessage([]byte(raw), "proxy.local"))

	// Extract Message-IDs
	extractMsgID := func(s string) string {
		start := strings.Index(s, "Message-ID: <")
		if start == -1 {
			return ""
		}
		end := strings.Index(s[start:], ">")
		if end == -1 {
			return ""
		}
		return s[start : start+end+1]
	}

	id1 := extractMsgID(result1)
	id2 := extractMsgID(result2)

	if id1 == "" || id2 == "" {
		t.Fatal("expected both messages to have Message-IDs")
	}
	if id1 == id2 {
		t.Error("expected different Message-IDs for different calls")
	}
}
