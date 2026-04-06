// LSP server tests — lifecycle, diagnostics, hover.
// Does NOT cover: jsonrpc transport (jsonrpc/transport_test.go), protocol types (protocol/uri_test.go).

package lsp

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"strings"
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
	nextID int
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
		DebounceMS: 10,
	})

	env := &testEnv{
		server: srv,
		client: jsonrpc.NewTransport(clientR, clientW),
		done:   make(chan error, 1),
	}
	go func() {
		env.done <- srv.Run(context.Background())
	}()
	t.Cleanup(func() { env.close(t) })
	return env
}

func (env *testEnv) close(t *testing.T) {
	t.Helper()
	env.request(t, "shutdown", nil)
	msg, _ := jsonrpc.NewNotification("exit", nil)
	env.client.Write(msg)
	select {
	case <-env.done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit")
	}
}

func (env *testEnv) request(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	env.nextID++
	id := json.RawMessage(strconv.Itoa(env.nextID))
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

func mustUnmarshal(t *testing.T, data json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestServer_InitializeShutdown(t *testing.T) {
	env := newTestEnv(t)

	result := env.request(t, "initialize", protocol.InitializeParams{})
	var initResult protocol.InitializeResult
	mustUnmarshal(t, result, &initResult)
	if !initResult.Capabilities.HoverProvider {
		t.Fatal("expected HoverProvider capability")
	}
	if initResult.ServerInfo == nil || initResult.ServerInfo.Name != "gicel-lsp" {
		t.Fatal("expected server info")
	}

	env.sendNotification(t, "initialized", nil)
}

func TestServer_Diagnostics(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///test.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nmain := 1 + \"hello\"",
		},
	})

	notif := env.readNotification(t, 5*time.Second)
	if notif == nil {
		t.Fatal("expected diagnostics notification")
	}
	if notif.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got %q", notif.Method)
	}
	var diagParams protocol.PublishDiagnosticsParams
	mustUnmarshal(t, notif.Params, &diagParams)
	if len(diagParams.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	if diagParams.Diagnostics[0].Source != "gicel" {
		t.Fatalf("expected source 'gicel', got %q", diagParams.Diagnostics[0].Source)
	}
}

func TestServer_HoverOnLiteral(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///hover.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nmain := 42",
		},
	})
	env.readNotification(t, 5*time.Second)

	hoverResult := env.request(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///hover.gicel"},
		Position:     protocol.Position{Line: 1, Character: 8},
	})

	if string(hoverResult) == "null" {
		t.Fatal("hover returned null — expected type at position")
	}
	var hover protocol.Hover
	mustUnmarshal(t, hoverResult, &hover)
	if !strings.Contains(hover.Contents.Value, "Int") {
		t.Fatalf("expected hover to contain 'Int', got %q", hover.Contents.Value)
	}
}

func TestServer_HoverOnDefinitionSite(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///defsite.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nmain := 42",
		},
	})
	env.readNotification(t, 5*time.Second)

	hoverResult := env.request(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///defsite.gicel"},
		Position:     protocol.Position{Line: 1, Character: 0},
	})
	if string(hoverResult) == "null" {
		t.Fatal("hover on definition site 'main' returned null — expected type")
	}
	var hover protocol.Hover
	mustUnmarshal(t, hoverResult, &hover)
	if !strings.Contains(hover.Contents.Value, "Int") {
		t.Fatalf("expected hover to contain 'Int', got %q", hover.Contents.Value)
	}
}

func TestServer_HoverOnWhitespace(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	// Source with a blank line at line 2.
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///ws.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\n\nmain := 42",
		},
	})
	env.readNotification(t, 5*time.Second)

	// Hover on the blank line (line 1, character 0).
	hoverResult := env.request(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///ws.gicel"},
		Position:     protocol.Position{Line: 1, Character: 0},
	})
	if string(hoverResult) != "null" {
		t.Fatalf("expected null hover on blank line, got %s", string(hoverResult))
	}
}

func TestServer_DiagnosticsClearOnFix(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///fix.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nmain := 1 + \"hello\"",
		},
	})
	notif := env.readNotification(t, 5*time.Second)
	var diag1 protocol.PublishDiagnosticsParams
	mustUnmarshal(t, notif.Params, &diag1)
	if len(diag1.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for type error")
	}

	env.sendNotification(t, "textDocument/didChange", protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			URI:     "file:///fix.gicel",
			Version: 2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: "import Prelude\nmain := 1 + 2"},
		},
	})
	notif2 := env.readNotification(t, 5*time.Second)
	var diag2 protocol.PublishDiagnosticsParams
	mustUnmarshal(t, notif2.Params, &diag2)
	if len(diag2.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics after fix, got %d", len(diag2.Diagnostics))
	}
}
