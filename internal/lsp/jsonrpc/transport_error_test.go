// Transport error-path tests — malformed headers, oversized bodies,
// truncation, and extra-header tolerance. The server loop reacts to
// DecodeError differently than to raw I/O errors, so distinguishing the
// two is a correctness property.
//
// Does NOT cover: normal framing (transport_test.go).

package jsonrpc

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// TestTransport_InvalidContentLength: a non-numeric Content-Length must
// surface as a DecodeError so the server loop skips the message instead
// of aborting the session.
func TestTransport_InvalidContentLength(t *testing.T) {
	raw := "Content-Length: abc\r\n\r\n{}"
	tr := NewTransport(strings.NewReader(raw), nil)
	_, err := tr.Read()
	if err == nil {
		t.Fatal("expected error")
	}
	var decErr *DecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// TestTransport_OversizeContentLength: Content-Length beyond the guard
// must be rejected before allocation. This protects against memory-
// exhaustion DoS from a malicious client.
func TestTransport_OversizeContentLength(t *testing.T) {
	raw := "Content-Length: " + strconv.Itoa(maxContentLength+1) + "\r\n\r\n"
	tr := NewTransport(strings.NewReader(raw), nil)
	_, err := tr.Read()
	if err == nil {
		t.Fatal("expected error")
	}
	var decErr *DecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' message, got %v", err)
	}
}

// TestTransport_TruncatedBody: Content-Length exceeds the actual body
// bytes. io.ReadFull should fail, which propagates as a non-DecodeError
// (raw read error) because the session is no longer recoverable.
func TestTransport_TruncatedBody(t *testing.T) {
	raw := "Content-Length: 100\r\n\r\n{\"jsonrpc\":\"2.0\"}"
	tr := NewTransport(strings.NewReader(raw), nil)
	_, err := tr.Read()
	if err == nil {
		t.Fatal("expected error")
	}
	// Truncation is a transport-level failure, not a protocol decode error.
	// The server treats this as session-ending (stdin closed).
	var decErr *DecodeError
	if errors.As(err, &decErr) {
		t.Fatalf("truncation should NOT be DecodeError: %v", err)
	}
}

// TestTransport_InvalidJSONBody: Content-Length is correct, but the body
// is not valid JSON. Must surface as DecodeError so the server skips it.
func TestTransport_InvalidJSONBody(t *testing.T) {
	body := "{ not valid json"
	raw := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	tr := NewTransport(strings.NewReader(raw), nil)
	_, err := tr.Read()
	if err == nil {
		t.Fatal("expected error")
	}
	var decErr *DecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected *DecodeError, got %T: %v", err, err)
	}
}

// TestTransport_IgnoresExtraHeaders: Content-Type and other headers are
// required to be ignored by the base protocol — a well-behaved client
// may include them without breaking framing.
func TestTransport_IgnoresExtraHeaders(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"initialized","params":{}}`
	raw := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n" +
		"Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n" +
		"X-Custom-Header: anything\r\n" +
		"\r\n" + body
	tr := NewTransport(strings.NewReader(raw), nil)
	msg, err := tr.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Method != "initialized" {
		t.Errorf("expected initialized, got %q", msg.Method)
	}
}

// TestTransport_EOFBeforeHeader: pure EOF before any header bytes means
// the peer closed the pipe. This is a raw I/O error, not a DecodeError —
// the server's main loop uses this distinction to decide between skip
// and exit.
func TestTransport_EOFBeforeHeader(t *testing.T) {
	tr := NewTransport(strings.NewReader(""), nil)
	_, err := tr.Read()
	if err == nil {
		t.Fatal("expected error on empty input")
	}
	var decErr *DecodeError
	if errors.As(err, &decErr) {
		t.Fatalf("EOF should not be DecodeError: %v", err)
	}
}
