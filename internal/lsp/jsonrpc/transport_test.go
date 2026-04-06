package jsonrpc

// Transport tests — Content-Length framing, message roundtrip.
// Does NOT cover: message types (message.go is tested implicitly).

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// formatMessage builds a raw LSP message for testing.
func formatMessage(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}

func TestTransport_ReadRequest(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	raw := formatMessage(body)
	tr := NewTransport(strings.NewReader(raw), nil)

	msg, err := tr.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !msg.IsRequest() {
		t.Fatal("expected request")
	}
	if msg.Method != "initialize" {
		t.Fatalf("expected method 'initialize', got %q", msg.Method)
	}
}

func TestTransport_ReadNotification(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"initialized","params":{}}`
	raw := formatMessage(body)
	tr := NewTransport(strings.NewReader(raw), nil)

	msg, err := tr.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !msg.IsNotification() {
		t.Fatal("expected notification")
	}
}

func TestTransport_ReadMultiple(t *testing.T) {
	b1 := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	b2 := `{"jsonrpc":"2.0","method":"initialized","params":{}}`
	raw := formatMessage(b1) + formatMessage(b2)
	tr := NewTransport(strings.NewReader(raw), nil)

	msg1, err := tr.Read()
	if err != nil {
		t.Fatal(err)
	}
	if msg1.Method != "initialize" {
		t.Fatalf("msg1: expected 'initialize', got %q", msg1.Method)
	}

	msg2, err := tr.Read()
	if err != nil {
		t.Fatal(err)
	}
	if msg2.Method != "initialized" {
		t.Fatalf("msg2: expected 'initialized', got %q", msg2.Method)
	}
}

func TestTransport_ReadMissingContentLength(t *testing.T) {
	raw := "\r\n{}"
	tr := NewTransport(strings.NewReader(raw), nil)
	_, err := tr.Read()
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
}

func TestTransport_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	tr := NewTransport(nil, &buf)

	id := json.RawMessage(`1`)
	msg := Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "textDocument/hover",
		Params:  json.RawMessage(`{"textDocument":{"uri":"file:///test.gicel"}}`),
	}
	if err := tr.Write(msg); err != nil {
		t.Fatal(err)
	}

	// Read it back.
	tr2 := NewTransport(&buf, nil)
	got, err := tr2.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "textDocument/hover" {
		t.Fatalf("expected method 'textDocument/hover', got %q", got.Method)
	}
	var gotID int
	json.Unmarshal(*got.ID, &gotID)
	if gotID != 1 {
		t.Fatalf("expected id 1, got %d", gotID)
	}
}

func TestNewResponse(t *testing.T) {
	id := json.RawMessage(`42`)
	msg, err := NewResponse(&id, map[string]string{"name": "gicel"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.JSONRPC != "2.0" {
		t.Fatalf("expected jsonrpc 2.0, got %q", msg.JSONRPC)
	}
	if msg.Error != nil {
		t.Fatal("expected no error")
	}
}

func TestNewError(t *testing.T) {
	id := json.RawMessage(`1`)
	msg := NewError(&id, CodeMethodNotFound, "not found")
	if msg.Error == nil {
		t.Fatal("expected error")
	}
	if msg.Error.Code != CodeMethodNotFound {
		t.Fatalf("expected code %d, got %d", CodeMethodNotFound, msg.Error.Code)
	}
}

func TestNewNotification(t *testing.T) {
	msg, err := NewNotification("textDocument/publishDiagnostics", map[string]any{"uri": "file:///x"})
	if err != nil {
		t.Fatal(err)
	}
	if !msg.IsNotification() {
		t.Fatal("expected notification")
	}
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected method, got %q", msg.Method)
	}
}
