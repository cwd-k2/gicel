package lsp

import (
	"encoding/json"
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/lsp/jsonrpc"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

// testEnv provides an in-process LSP server with piped transport.
type testEnv struct {
	server *Server
	client *jsonrpc.Transport
	done   chan error
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()

	srv := NewServer(ServerConfig{
		Transport: jsonrpc.NewTransport(serverR, serverW),
		Logger:    log.New(io.Discard, "", 0),
		EngineSetup: func() *engine.Engine {
			eng := engine.NewEngine()
			eng.Use(stdlib.Prelude)
			return eng
		},
		DebounceMS: 10, // fast debounce for tests
	})

	env := &testEnv{
		server: srv,
		client: jsonrpc.NewTransport(clientR, clientW),
		done:   make(chan error, 1),
	}
	go func() {
		env.done <- srv.Run(context.Background())
	}()
	return env
}

func (env *testEnv) close(t *testing.T) {
	t.Helper()
	// shutdown
	env.request(t, "shutdown", nil)
	// exit
	msg, _ := jsonrpc.NewNotification("exit", nil)
	env.client.Write(msg)
	select {
	case <-env.done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit")
	}
}

var nextID int

func (env *testEnv) request(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	nextID++
	id := json.RawMessage(json.RawMessage(`` + intStr(nextID)))
	data, _ := json.Marshal(params)
	msg := jsonrpc.Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  data,
	}
	if err := env.client.Write(msg); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp, err := env.client.Read()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("error response: %d %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result
}

func (env *testEnv) sendNotification(t *testing.T, method string, params any) {
	t.Helper()
	msg, err := jsonrpc.NewNotification(method, params)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.client.Write(msg); err != nil {
		t.Fatal(err)
	}
}

func (env *testEnv) readNotification(t *testing.T, timeout time.Duration) *jsonrpc.Message {
	t.Helper()
	ch := make(chan *jsonrpc.Message, 1)
	go func() {
		msg, _ := env.client.Read()
		ch <- msg
	}()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatal("timeout waiting for notification")
		return nil
	}
}

func intStr(n int) string {
	buf := make([]byte, 0, 4)
	if n == 0 {
		return "0"
	}
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func TestServer_InitializeShutdown(t *testing.T) {
	env := newTestEnv(t)

	// Initialize.
	result := env.request(t, "initialize", protocol.InitializeParams{})
	var initResult protocol.InitializeResult
	json.Unmarshal(result, &initResult)
	if !initResult.Capabilities.HoverProvider {
		t.Fatal("expected HoverProvider capability")
	}
	if initResult.ServerInfo == nil || initResult.ServerInfo.Name != "gicel-lsp" {
		t.Fatal("expected server info")
	}

	// Initialized notification.
	env.sendNotification(t, "initialized", nil)

	env.close(t)
}

func TestServer_Diagnostics(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	// Open a file with a type error.
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///test.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nmain := 1 + \"hello\"",
		},
	})

	// Read the publishDiagnostics notification.
	notif := env.readNotification(t, 5*time.Second)
	if notif == nil {
		t.Fatal("expected diagnostics notification")
	}
	if notif.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got %q", notif.Method)
	}
	var diagParams protocol.PublishDiagnosticsParams
	json.Unmarshal(notif.Params, &diagParams)
	if len(diagParams.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	if diagParams.Diagnostics[0].Source != "gicel" {
		t.Fatalf("expected source 'gicel', got %q", diagParams.Diagnostics[0].Source)
	}

	env.close(t)
}

func TestServer_HoverOnLiteral(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	// Open a valid file.
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///hover.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nmain := 42",
		},
	})

	// Wait for diagnostics (confirms analysis completed).
	env.readNotification(t, 5*time.Second)

	// Hover on "42" (line 1, character 8 — "main := 42", '4' is at col 8).
	hoverResult := env.request(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///hover.gicel"},
		Position:     protocol.Position{Line: 1, Character: 8},
	})

	// hoverResult may be "null" if no type at that position.
	if string(hoverResult) == "null" {
		t.Fatal("hover returned null — expected type at position")
	}
	var hover protocol.Hover
	if err := json.Unmarshal(hoverResult, &hover); err != nil {
		t.Fatalf("unmarshal hover: %v", err)
	}
	if hover.Contents.Value == "" {
		t.Fatal("expected non-empty hover contents")
	}
	t.Logf("hover: %s", hover.Contents.Value)
}
