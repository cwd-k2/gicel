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
	// With HoverIndex, binding sites now show "main :: Int".
	if !strings.Contains(hover.Contents.Value, "main :: Int") {
		t.Fatalf("expected hover to contain 'main :: Int', got %q", hover.Contents.Value)
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

// hoverAt is a test helper that opens a document, waits for analysis,
// then returns the hover string at the given line/character.
func hoverAt(t *testing.T, env *testEnv, uri protocol.DocumentURI, source string, line, char int) string {
	t.Helper()
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "gicel",
			Version:    1,
			Text:       source,
		},
	})
	env.readNotification(t, 5*time.Second)
	result := env.request(t, "textDocument/hover", protocol.HoverParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position:     protocol.Position{Line: line, Character: char},
	})
	if string(result) == "null" {
		return ""
	}
	var hover protocol.Hover
	mustUnmarshal(t, result, &hover)
	return hover.Contents.Value
}

func TestServer_HoverOnFormDecl(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	source := "form Color := Red | Green | Blue\nmain := Red"
	hover := hoverAt(t, env, "file:///form.gicel", source, 0, 5)
	if hover == "" {
		t.Fatal("hover on form declaration returned null")
	}
	if !strings.Contains(hover, "form Color") {
		t.Fatalf("expected hover to contain 'form Color', got %q", hover)
	}
}

func TestServer_HoverOnConstructor(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	// Hover on "Red" in usage position (line 1).
	source := "import Prelude\nform Color := Red | Green | Blue\nmain := Red"
	hover := hoverAt(t, env, "file:///con.gicel", source, 2, 8)
	if hover == "" {
		t.Fatal("hover on constructor usage returned null")
	}
	if !strings.Contains(hover, "Color") {
		t.Fatalf("expected hover to contain 'Color', got %q", hover)
	}
}

func TestServer_HoverOnTypeAnnotation(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	source := "import Prelude\nf :: Int -> Int\nf := \\x. x + 1\nmain := f 42"
	// Hover on "f" in the :: annotation line (line 1, char 0).
	hover := hoverAt(t, env, "file:///ann.gicel", source, 1, 0)
	if hover == "" {
		t.Fatal("hover on type annotation returned null")
	}
	if !strings.Contains(hover, "f :: ") && !strings.Contains(hover, "Int -> Int") {
		t.Fatalf("expected hover to contain type annotation info, got %q", hover)
	}
}

func TestServer_HoverOnImport(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	source := "import Prelude\nmain := 42"
	hover := hoverAt(t, env, "file:///imp.gicel", source, 0, 3)
	if hover == "" {
		t.Fatal("hover on import returned null")
	}
	if !strings.Contains(hover, "import Prelude") {
		t.Fatalf("expected hover to contain 'import Prelude', got %q", hover)
	}
}

func TestServer_HoverOnTypeAlias(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	source := "import Prelude\ntype Pair := \\a b. { fst: a; snd: b }\nmain := { fst := 1; snd := 2 }"
	// Hover on "Pair" (line 1, char 5).
	hover := hoverAt(t, env, "file:///alias.gicel", source, 1, 5)
	if hover == "" {
		t.Fatal("hover on type alias returned null")
	}
	if !strings.Contains(hover, "type Pair") {
		t.Fatalf("expected hover to contain 'type Pair', got %q", hover)
	}
}

func TestServer_CompletionBasic(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///comp.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "import Prelude\nform Color := Red | Green | Blue\nf :: Int -> Int\nf := \\x. x + 1\nmain := f 42",
		},
	})
	env.readNotification(t, 5*time.Second)

	result := env.request(t, "textDocument/completion", protocol.CompletionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///comp.gicel"},
		Position:     protocol.Position{Line: 4, Character: 8},
	})

	var list protocol.CompletionList
	mustUnmarshal(t, result, &list)
	if len(list.Items) == 0 {
		t.Fatal("expected at least one completion item")
	}

	// Check that user bindings appear.
	found := map[string]bool{}
	for _, item := range list.Items {
		found[item.Label] = true
	}
	for _, name := range []string{"f", "main", "Color", "Red", "Green", "Blue"} {
		if !found[name] {
			t.Errorf("expected completion item %q", name)
		}
	}
}

func TestServer_CompletionEmpty(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	// Open a document with errors — completion should still return empty list, not error.
	env.sendNotification(t, "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///empty.gicel",
			LanguageID: "gicel",
			Version:    1,
			Text:       "main := (",
		},
	})
	env.readNotification(t, 5*time.Second)

	result := env.request(t, "textDocument/completion", protocol.CompletionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///empty.gicel"},
		Position:     protocol.Position{Line: 0, Character: 9},
	})

	var list protocol.CompletionList
	mustUnmarshal(t, result, &list)
	// Should not error — empty or minimal list is fine.
}

func TestServer_HoverOnImpl(t *testing.T) {
	env := newTestEnv(t)
	env.request(t, "initialize", protocol.InitializeParams{})
	env.sendNotification(t, "initialized", nil)

	source := "import Prelude\nform MyEq := \\a. { myEq: a -> a -> Bool }\nimpl MyEq Int := { myEq := \\x y. x == y }\nmain := myEq 1 2"
	// Hover on "impl" (line 2, char 0).
	hover := hoverAt(t, env, "file:///impl.gicel", source, 2, 0)
	if hover == "" {
		t.Fatal("hover on impl returned null")
	}
	if !strings.Contains(hover, "impl") {
		t.Fatalf("expected hover to contain 'impl', got %q", hover)
	}
}
