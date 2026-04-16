// LSP lifecycle tests — request rejection while uninitialized / shutting
// down, notifications that clear document state, and didChange edge cases.
// Does NOT cover: hover, completion, definition, documentSymbol (server_test.go).

package lsp

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/lsp/jsonrpc"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

// newBareEnv returns a test environment without automatic shutdown on
// cleanup. Tests that deliberately leave the server in an unusual state
// (uninitialized, shutdown-but-not-exited) must use this to avoid
// cleanup failures.
func newBareEnv(t *testing.T) *testEnv {
	t.Helper()
	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()

	srv := NewServer(ServerConfig{
		Transport: jsonrpc.NewTransport(serverR, serverW),
		Logger:    log.New(io.Discard, "", 0),
		EngineSetup: func() AnalysisEngine {
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
	t.Cleanup(func() {
		// Best-effort exit: send notification, close pipes. The server
		// will wake up when stdin closes.
		msg, _ := jsonrpc.NewNotification("exit", nil)
		_ = env.client.Write(msg)
		clientR.Close()
		clientW.Close()
		select {
		case <-env.done:
		case <-time.After(2 * time.Second):
			// Don't fail on leaked goroutine — we're in a cleanup edge
			// case where the server state is intentionally unusual.
		}
	})
	return env
}

// requestRaw issues a request and returns the full response (including
// possible error payload) without failing the test.
func (env *testEnv) requestRaw(t *testing.T, method string, params any) *jsonrpc.Message {
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
		t.Fatalf("write: %v", err)
	}
	resp, err := env.client.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp
}

// TestServer_UninitializedRequest: requests before `initialize` must be
// rejected with code -32002 (server-not-initialized). This is required by
// the LSP spec — clients rely on it to detect misordered handshakes.
func TestServer_UninitializedRequest(t *testing.T) {
	env := newBareEnv(t)
	resp := env.requestRaw(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///x.gicel"},
	})
	if resp.Error == nil {
		t.Fatal("expected error on pre-init request")
	}
	if resp.Error.Code != -32002 {
		t.Errorf("expected code -32002, got %d: %s", resp.Error.Code, resp.Error.Message)
	}
}

// TestServer_DoubleInitialize: re-sending `initialize` after it already
// succeeded must error, not silently reset state.
func TestServer_DoubleInitialize(t *testing.T) {
	env := newBareEnv(t)
	_ = env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)
	resp := env.requestRaw(t, "initialize", protocol.InitializeParams{})
	if resp.Error == nil {
		t.Fatal("expected error on double initialize")
	}
}

// TestServer_UnknownMethod: methods the server does not implement must
// be rejected with MethodNotFound, not silently swallowed.
func TestServer_UnknownMethod(t *testing.T) {
	env := newBareEnv(t)
	_ = env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)
	resp := env.requestRaw(t, "textDocument/codeAction", map[string]any{})
	if resp.Error == nil {
		t.Fatal("expected error on unknown method")
	}
	if resp.Error.Code != jsonrpc.CodeMethodNotFound {
		t.Errorf("expected MethodNotFound, got %d", resp.Error.Code)
	}
}

// TestServer_DidCloseClearsDiagnostics: closing a document must publish
// an empty diagnostics array so the editor drops any stale squiggles.
func TestServer_DidCloseClearsDiagnostics(t *testing.T) {
	env := newBareEnv(t)
	_ = env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	uri := protocol.DocumentURI("file:///close.gicel")
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: uri, LanguageID: "gicel", Version: 1,
			Text: "import Prelude\nmain := 1 + \"bad\"",
		},
	})
	// Drain the open-time diagnostics before sending didClose, otherwise
	// we might read the error diagnostic instead of the clear.
	env.readNotification(t, 5*time.Second)

	env.sendNotification(t, "textDocument/didClose", protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	notif := env.readNotification(t, 5*time.Second)
	if notif.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got %q", notif.Method)
	}
	var params protocol.PublishDiagnosticsParams
	mustUnmarshal(t, notif.Params, &params)
	if len(params.Diagnostics) != 0 {
		t.Errorf("expected empty diagnostics after close, got %d", len(params.Diagnostics))
	}
}

// TestServer_DidChangeNoChanges: didChange with an empty ContentChanges
// array must not panic. Some clients send these under race conditions.
func TestServer_DidChangeNoChanges(t *testing.T) {
	env := newBareEnv(t)
	_ = env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	uri := protocol.DocumentURI("file:///empty-change.gicel")
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: uri, LanguageID: "gicel", Version: 1,
			Text: "import Prelude\nmain := 1",
		},
	})
	env.readNotification(t, 5*time.Second)

	// Empty changes — server should still re-diagnose, but must not crash.
	env.sendNotification(t, "textDocument/didChange", protocol.DidChangeTextDocumentParams{
		TextDocument:   protocol.VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: nil,
	})
	// The key property is no panic. Give the server time to re-diagnose;
	// we do not require a notification since nothing substantive changed.
	time.Sleep(500 * time.Millisecond)
}

// TestServer_DidSaveWithText: didSave with a non-nil text field must apply
// that text as the current document state and re-diagnose.
func TestServer_DidSaveWithText(t *testing.T) {
	env := newBareEnv(t)
	_ = env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	uri := protocol.DocumentURI("file:///save.gicel")
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI: uri, LanguageID: "gicel", Version: 1,
			Text: "import Prelude\nmain := 1",
		},
	})
	env.readNotification(t, 5*time.Second)

	badText := "import Prelude\nmain := 1 + \"bad\""
	env.sendNotification(t, "textDocument/didSave", protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Text:         &badText,
	})
	notif := env.readNotification(t, 5*time.Second)
	var params protocol.PublishDiagnosticsParams
	mustUnmarshal(t, notif.Params, &params)
	if len(params.Diagnostics) == 0 {
		t.Error("expected diagnostics after didSave with bad text")
	}
}

// TestServer_ShutdownRejectsFurtherRequests: after shutdown, the server
// must reject further requests with InvalidRequest.
func TestServer_ShutdownRejectsFurtherRequests(t *testing.T) {
	env := newBareEnv(t)
	_ = env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)
	_ = env.request(t, "shutdown", nil)

	resp := env.requestRaw(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///x.gicel"},
	})
	if resp.Error == nil {
		t.Fatal("expected error on request after shutdown")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("expected InvalidRequest, got %d", resp.Error.Code)
	}
}
