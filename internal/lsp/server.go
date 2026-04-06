// Package lsp implements the GICEL Language Server Protocol server.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/app/header"
	"github.com/cwd-k2/gicel/internal/lang/types"
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

	debounceDelay time.Duration
	cancel        context.CancelFunc // cancels pending diagnose goroutines

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
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.SyncFull,
				Save:      &protocol.SaveOptions{IncludeText: true},
			},
			HoverProvider: true,
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
	s.docs.close(params.TextDocument.URI)
	s.notify("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
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
	if timer, ok := s.debounceTimers[uri]; ok {
		timer.Stop()
	}
	s.debounceTimers[uri] = time.AfterFunc(s.debounceDelay, func() {
		s.diagnose(ctx, uri)
	})
}

// drainTimers stops all pending debounce timers.
func (s *Server) drainTimers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for uri, timer := range s.debounceTimers {
		timer.Stop()
		delete(s.debounceTimers, uri)
	}
}

func (s *Server) diagnose(ctx context.Context, uri protocol.DocumentURI) {
	if ctx.Err() != nil {
		return // server shutting down
	}

	doc, ok := s.docs.get(uri)
	if !ok {
		return
	}

	eng := s.engineSetup()
	if eng == nil {
		s.logger.Printf("engine factory returned nil")
		return
	}
	eng.EnableTypeIndex()

	docPath := protocol.URIToPath(uri)
	res, err := header.Resolve(doc.Text, docPath)
	if err != nil {
		s.logger.Printf("header resolve: %v", err)
	} else {
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
	if !ok || doc.Analysis == nil || doc.Analysis.TypeIndex == nil || doc.Analysis.Source == nil {
		s.respondResult(msg.ID, nil)
		return
	}

	offset := posToOffset(doc.Analysis.Source, params.Position)
	ty := doc.Analysis.TypeIndex.TypeAt(offset)
	if ty == nil {
		s.respondResult(msg.ID, nil)
		return
	}

	s.respondResult(msg.ID, protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: "```gicel\n" + types.Pretty(ty) + "\n```",
		},
	})
}
