// Package lsp implements the GICEL Language Server Protocol server.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"unicode"
	"unicode/utf8"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/app/header"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lsp/jsonrpc"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

// Server is the GICEL LSP server.
type Server struct {
	transport *jsonrpc.Transport
	docs      *documentStore
	logger    *log.Logger

	// Engine factory — called per diagnose to create a fresh Engine.
	// Must not return nil.
	engineSetup func() *engine.Engine

	// Debounce state (guarded by mu).
	mu             sync.Mutex
	debounceTimers map[protocol.DocumentURI]*time.Timer
	diagCancels    map[protocol.DocumentURI]context.CancelFunc // per-URI in-flight cancel

	debounceDelay time.Duration
	cancel        context.CancelFunc // cancels all pending diagnose goroutines (shutdown)

	// Lifecycle state (accessed only from the main goroutine).
	initialized       bool
	shutdownRequested bool
	exitCode          int
	exitOnce          sync.Once
	exitCh            chan struct{}
}

// ServerConfig configures the LSP server.
type ServerConfig struct {
	Transport   *jsonrpc.Transport
	Logger      *log.Logger
	EngineSetup func() *engine.Engine
	DebounceMS  int // debounce delay in ms (default: 300)
}

// NewServer creates a new LSP server.
func NewServer(cfg ServerConfig) *Server {
	delay := 300
	if cfg.DebounceMS > 0 {
		delay = cfg.DebounceMS
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Server{
		transport:      cfg.Transport,
		docs:           newDocumentStore(),
		logger:         logger,
		engineSetup:    cfg.EngineSetup,
		debounceTimers: make(map[protocol.DocumentURI]*time.Timer),
		diagCancels:    make(map[protocol.DocumentURI]context.CancelFunc),
		debounceDelay:  time.Duration(delay) * time.Millisecond,
		exitCode:       1, // default: no shutdown received
		exitCh:         make(chan struct{}),
	}
}

// ExitCode returns the exit code: 0 if shutdown was received, 1 otherwise.
// Call after Run returns.
func (s *Server) ExitCode() int { return s.exitCode }

// Run reads messages in a loop until exit or context cancellation.
// Pending diagnose goroutines are cancelled when Run returns.
// Run starts the main message loop. The loop checks ctx.Done() and exitCh
// between messages but blocks inside transport.Read() — cancellation while
// waiting for the next message requires the client to close stdin or send
// a message. This is inherent to stdio-based LSP transports.
func (s *Server) Run(ctx context.Context) error {
	diagnoseCtx, diagnoseCancel := context.WithCancel(ctx)
	s.cancel = diagnoseCancel
	defer s.drainTimers()
	defer diagnoseCancel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.exitCh:
			return nil
		default:
		}

		msg, err := s.transport.Read()
		if err != nil {
			var decErr *jsonrpc.DecodeError
			if errors.As(err, &decErr) {
				s.logger.Printf("malformed message (skipping): %v", decErr)
				continue
			}
			select {
			case <-s.exitCh:
				return nil
			default:
			}
			return err
		}
		s.dispatch(diagnoseCtx, msg)
	}
}

func (s *Server) dispatch(ctx context.Context, msg *jsonrpc.Message) {
	if msg.IsRequest() {
		s.handleRequest(msg)
	} else if msg.IsNotification() {
		s.handleNotification(ctx, msg)
	}
}

const codeServerNotInitialized = -32002

func (s *Server) handleRequest(msg *jsonrpc.Message) {
	if s.shutdownRequested {
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidRequest,
			"server is shutting down"))
		return
	}
	if !s.initialized && msg.Method != "initialize" {
		s.respond(jsonrpc.NewError(msg.ID, codeServerNotInitialized,
			"server not initialized"))
		return
	}

	switch msg.Method {
	case "initialize":
		if s.initialized {
			s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidRequest,
				"server already initialized"))
			return
		}
		s.handleInitialize(msg)
	case "shutdown":
		s.handleShutdown(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	default:
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeMethodNotFound,
			"method not found: "+msg.Method))
	}
}

func (s *Server) handleNotification(ctx context.Context, msg *jsonrpc.Message) {
	switch msg.Method {
	case "initialized":
		// no-op
	case "exit":
		s.exitOnce.Do(func() { close(s.exitCh) })
	case "textDocument/didOpen":
		s.handleDidOpen(ctx, msg)
	case "textDocument/didChange":
		s.handleDidChange(ctx, msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/didSave":
		s.handleDidSave(ctx, msg)
	}
}

func (s *Server) respond(msg jsonrpc.Message) {
	if err := s.transport.Write(msg); err != nil {
		s.logger.Printf("write error: %v", err)
	}
}

func (s *Server) respondResult(id *json.RawMessage, result any) {
	resp, err := jsonrpc.NewResponse(id, result)
	if err != nil {
		s.logger.Printf("encode response: %v", err)
		s.respond(jsonrpc.NewError(id, jsonrpc.CodeInternalError, "internal error"))
		return
	}
	s.respond(resp)
}

func (s *Server) notify(method string, params any) {
	msg, err := jsonrpc.NewNotification(method, params)
	if err != nil {
		s.logger.Printf("notify encode error: %v", err)
		return
	}
	s.respond(msg)
}

// ---- Initialize / Shutdown ----

func (s *Server) handleInitialize(msg *jsonrpc.Message) {
	s.initialized = true
	result := protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			PositionEncoding: "utf-8",
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.SyncFull,
				Save:      &protocol.SaveOptions{IncludeText: true},
			},
			HoverProvider: true,
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{"."},
			},
			DocumentSymbolProvider: true,
			DefinitionProvider:     true,
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "gicel-lsp",
			Version: "0.1.0",
		},
	}
	s.respondResult(msg.ID, result)
}

func (s *Server) handleShutdown(msg *jsonrpc.Message) {
	s.shutdownRequested = true
	s.exitCode = 0
	s.respondResult(msg.ID, nil)
}

// ---- Document Sync ----

func (s *Server) handleDidOpen(ctx context.Context, msg *jsonrpc.Message) {
	var params protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didOpen unmarshal: %v", err)
		return
	}
	s.docs.open(params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version)
	s.scheduleDiagnose(ctx, params.TextDocument.URI)
}

func (s *Server) handleDidChange(ctx context.Context, msg *jsonrpc.Message) {
	var params protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didChange unmarshal: %v", err)
		return
	}
	if len(params.ContentChanges) > 0 {
		s.docs.update(params.TextDocument.URI, params.ContentChanges[0].Text, params.TextDocument.Version)
	}
	s.scheduleDiagnose(ctx, params.TextDocument.URI)
}

func (s *Server) handleDidClose(msg *jsonrpc.Message) {
	var params protocol.DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didClose unmarshal: %v", err)
		return
	}
	uri := params.TextDocument.URI
	s.docs.close(uri)
	// Clean up debounce state to prevent leaking timers and cancel funcs
	// across open/close cycles on the same or different URIs.
	s.mu.Lock()
	if timer, ok := s.debounceTimers[uri]; ok {
		timer.Stop()
		delete(s.debounceTimers, uri)
	}
	if cancel, ok := s.diagCancels[uri]; ok {
		cancel()
		delete(s.diagCancels, uri)
	}
	s.mu.Unlock()
	s.notify("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: []protocol.Diagnostic{},
	})
}

func (s *Server) handleDidSave(ctx context.Context, msg *jsonrpc.Message) {
	var params protocol.DidSaveTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didSave unmarshal: %v", err)
		return
	}
	if params.Text != nil {
		s.docs.update(params.TextDocument.URI, *params.Text, -1)
	}
	s.scheduleDiagnose(ctx, params.TextDocument.URI)
}

// ---- Diagnostics ----

func (s *Server) scheduleDiagnose(ctx context.Context, uri protocol.DocumentURI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Stop pending debounce timer.
	if timer, ok := s.debounceTimers[uri]; ok {
		timer.Stop()
	}
	// Cancel any in-flight diagnose goroutine for this URI.
	if cancel, ok := s.diagCancels[uri]; ok {
		cancel()
	}
	// Create a dedicated context for the new diagnose goroutine.
	diagCtx, diagCancel := context.WithCancel(ctx)
	s.diagCancels[uri] = diagCancel
	s.debounceTimers[uri] = time.AfterFunc(s.debounceDelay, func() {
		s.diagnose(diagCtx, uri)
	})
}

// drainTimers stops all pending debounce timers and cancels in-flight
// diagnose goroutines.
func (s *Server) drainTimers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for uri, timer := range s.debounceTimers {
		timer.Stop()
		delete(s.debounceTimers, uri)
	}
	for uri, cancel := range s.diagCancels {
		cancel()
		delete(s.diagCancels, uri)
	}
}

func (s *Server) diagnose(ctx context.Context, uri protocol.DocumentURI) {
	if ctx.Err() != nil {
		return // cancelled or server shutting down
	}

	doc, ok := s.docs.get(uri)
	if !ok {
		return
	}
	// Capture document version at launch time for stale-check after analysis.
	capturedVersion := doc.Version

	eng := s.engineSetup()
	if eng == nil {
		s.logger.Printf("engine factory returned nil")
		return
	}
	eng.EnableHoverIndex()

	docPath := protocol.URIToPath(uri)
	res, err := header.Resolve(doc.Text, docPath)
	if err != nil {
		s.logger.Printf("header resolve: %v", err)
	} else {
		for _, w := range res.Warnings {
			s.logger.Printf("header warning: %s", w)
		}
		if res.Recursion {
			eng.EnableRecursion()
		}
		for _, mod := range res.Modules {
			if err := eng.RegisterModule(mod.Name, mod.Source); err != nil {
				s.logger.Printf("header module %s: %v", mod.Name, err)
			}
		}
	}

	analyzeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ar := eng.Analyze(analyzeCtx, doc.Text)

	// Check if the document was edited while analysis was running.
	// If the version has advanced, discard this stale result — a newer
	// diagnose goroutine will produce fresh diagnostics.
	if current, ok := s.docs.get(uri); ok && current.Version != capturedVersion {
		return
	}

	s.docs.setAnalysis(uri, ar)

	diags := convertDiagnostics(ar)
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}
	s.notify("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

// ---- Hover ----

func (s *Server) handleHover(msg *jsonrpc.Message) {
	var params protocol.HoverParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidParams, err.Error()))
		return
	}

	doc, ok := s.docs.get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.HoverIndex == nil || doc.Analysis.Source == nil {
		s.respondResult(msg.ID, nil)
		return
	}

	offset := posToOffset(doc.Analysis.Source, params.Position)
	hover := doc.Analysis.HoverIndex.HoverAt(offset)
	if hover == "" {
		s.respondResult(msg.ID, nil)
		return
	}

	s.respondResult(msg.ID, protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: "```gicel\n" + hover + "\n```",
		},
	})
}

// ---- Completion ----

func (s *Server) handleCompletion(msg *jsonrpc.Message) {
	var params protocol.CompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidParams, err.Error()))
		return
	}

	doc, ok := s.docs.get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Program == nil {
		s.respondResult(msg.ID, protocol.CompletionList{})
		return
	}

	// Detect qualified prefix: extract "Module" from "Module." before cursor.
	var qualPrefix string
	if doc.Analysis.Source != nil {
		offset := posToOffset(doc.Analysis.Source, params.Position)
		qualPrefix = extractQualifiedPrefix(doc.Text, int(offset))
	}

	var items []protocol.CompletionItem
	for _, e := range doc.Analysis.CompletionEntries {
		// When a qualified prefix is active, show only entries from that module.
		if qualPrefix != "" {
			if e.Module != qualPrefix {
				continue
			}
		}
		item := protocol.CompletionItem{
			Label:  e.Label,
			Kind:   protocol.CompletionItemKind(e.Kind),
			Detail: e.Detail,
		}
		if e.Documentation != "" {
			item.Documentation = &protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: e.Documentation,
			}
		}
		items = append(items, item)
	}
	s.respondResult(msg.ID, protocol.CompletionList{Items: items})
}

// extractQualifiedPrefix returns the module name if the cursor is immediately
// after "Module." (an uppercase identifier followed by a dot). Returns "" if
// no qualified prefix is detected. offset is the byte position of the cursor.
// Uses rune-level scanning consistent with identifierAtOffset.
func extractQualifiedPrefix(text string, offset int) string {
	if offset <= 0 || offset > len(text) {
		return ""
	}
	// Cursor is right after the dot (trigger character).
	if text[offset-1] != '.' {
		return ""
	}
	// Walk backwards from the dot to find the module name (rune-level).
	end := offset - 1
	start := end
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if !isIdentRune(r) {
			break
		}
		start -= size
	}
	if start >= end {
		return ""
	}
	name := text[start:end]
	// Module names start with an uppercase letter.
	firstRune, _ := utf8.DecodeRuneInString(name)
	if !unicode.IsUpper(firstRune) {
		return ""
	}
	return name
}

// ---- Document Symbols ----

func (s *Server) handleDocumentSymbol(msg *jsonrpc.Message) {
	var params protocol.DocumentSymbolParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidParams, err.Error()))
		return
	}

	doc, ok := s.docs.get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Program == nil || doc.Analysis.Source == nil {
		s.respondResult(msg.ID, []protocol.DocumentSymbol{})
		return
	}

	symbols := convertDocumentSymbols(doc.Analysis.DocumentSymbols, doc.Analysis.Source)
	s.respondResult(msg.ID, symbols)
}

func convertDocumentSymbols(entries []engine.DocumentSymbolEntry, src *span.Source) []protocol.DocumentSymbol {
	symbols := make([]protocol.DocumentSymbol, 0, len(entries))
	for _, e := range entries {
		sym := protocol.DocumentSymbol{
			Name:           e.Name,
			Detail:         e.Detail,
			Kind:           protocol.SymbolKind(e.Kind),
			Range:          spanToRange(src, e.S),
			SelectionRange: spanToRange(src, e.S),
		}
		if len(e.Children) > 0 {
			sym.Children = convertDocumentSymbols(e.Children, src)
		}
		symbols = append(symbols, sym)
	}
	return symbols
}

// ---- Definition ----

func (s *Server) handleDefinition(msg *jsonrpc.Message) {
	var params protocol.DefinitionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidParams, err.Error()))
		return
	}

	doc, ok := s.docs.get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.Source == nil || doc.Analysis.Program == nil {
		s.respondResult(msg.ID, nil)
		return
	}

	offset := posToOffset(doc.Analysis.Source, params.Position)
	name := identifierAtOffset(doc.Analysis.Source.Text, int(offset))
	if name == "" {
		s.respondResult(msg.ID, nil)
		return
	}

	// Search pre-computed definitions.
	for _, def := range doc.Analysis.Definitions {
		if def.Name == name {
			uri := params.TextDocument.URI
			src := doc.Analysis.Source
			if def.FilePath != "" && def.Source != nil {
				uri = protocol.DocumentURI("file://" + def.FilePath)
				src = def.Source
			}
			s.respondResult(msg.ID, protocol.Location{
				URI:   uri,
				Range: spanToRange(src, def.S),
			})
			return
		}
	}

	s.respondResult(msg.ID, nil)
}

// identifierAtOffset extracts the identifier or operator at the given
// byte offset by scanning forwards and backwards. Uses rune-level
// decoding to match the scanner's identifier definition.
func identifierAtOffset(source string, offset int) string {
	if offset < 0 || offset >= len(source) {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(source[offset:])
	if r == utf8.RuneError {
		return ""
	}

	if isIdentRune(r) {
		start := offset
		for start > 0 {
			pr, size := utf8.DecodeLastRuneInString(source[:start])
			if !isIdentRune(pr) {
				break
			}
			start -= size
		}
		end := offset
		for end < len(source) {
			nr, size := utf8.DecodeRuneInString(source[end:])
			if !isIdentRune(nr) {
				break
			}
			end += size
		}
		return source[start:end]
	}

	if isOperatorRune(r) {
		start := offset
		for start > 0 {
			pr, size := utf8.DecodeLastRuneInString(source[:start])
			if !isOperatorRune(pr) {
				break
			}
			start -= size
		}
		end := offset
		for end < len(source) {
			nr, size := utf8.DecodeRuneInString(source[end:])
			if !isOperatorRune(nr) {
				break
			}
			end += size
		}
		return source[start:end]
	}

	return ""
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\''
}

func isOperatorRune(r rune) bool {
	return r != '_' && r != '\'' && r != '(' && r != ')' &&
		!unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) &&
		r > ' ' && r != ',' && r != ';' && r != '{' && r != '}' && r != '[' && r != ']'
}
