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
	docs      *DocumentStore
	logger    *log.Logger

	// Engine factory — called per diagnose to create a fresh Engine.
	engineSetup func() *engine.Engine

	// Debounce state.
	mu             sync.Mutex
	debounceTimers map[protocol.DocumentURI]*time.Timer
	debounceDelay  time.Duration

	// Lifecycle state.
	initialized       bool
	shutdownRequested bool
	exitCode          int           // 0 if shutdown received, 1 otherwise
	exitCh            chan struct{} // closed on exit notification
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
		docs:           NewDocumentStore(),
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
func (s *Server) Run(ctx context.Context) error {
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
			// Recoverable: malformed JSON body — log and continue.
			var decErr *jsonrpc.DecodeError
			if errors.As(err, &decErr) {
				s.logger.Printf("malformed message (skipping): %v", decErr)
				continue
			}
			// Non-recoverable: transport-level error (EOF, I/O).
			select {
			case <-s.exitCh:
				return nil
			default:
			}
			return err
		}
		s.dispatch(msg)
	}
}

func (s *Server) dispatch(msg *jsonrpc.Message) {
	if msg.IsRequest() {
		s.handleRequest(msg)
	} else if msg.IsNotification() {
		s.handleNotification(msg)
	}
}

const codeServerNotInitialized = -32002

func (s *Server) handleRequest(msg *jsonrpc.Message) {
	// LSP spec: after shutdown, only exit is valid.
	if s.shutdownRequested {
		s.respond(jsonrpc.NewError(msg.ID, jsonrpc.CodeInvalidRequest,
			"server is shutting down"))
		return
	}
	// LSP spec: before initialize, only initialize is valid.
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

func (s *Server) handleNotification(msg *jsonrpc.Message) {
	switch msg.Method {
	case "initialized":
		// no-op
	case "exit":
		close(s.exitCh)
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/didSave":
		s.handleDidSave(msg)
	}
}

func (s *Server) respond(msg jsonrpc.Message) {
	if err := s.transport.Write(msg); err != nil {
		s.logger.Printf("write error: %v", err)
	}
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
	resp, _ := jsonrpc.NewResponse(msg.ID, result)
	s.respond(resp)
}

func (s *Server) handleShutdown(msg *jsonrpc.Message) {
	s.shutdownRequested = true
	s.exitCode = 0 // clean shutdown
	resp, _ := jsonrpc.NewResponse(msg.ID, nil)
	s.respond(resp)
}

// ---- Document Sync ----

func (s *Server) handleDidOpen(msg *jsonrpc.Message) {
	var params protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	s.docs.Open(params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version)
	s.scheduleDiagnose(params.TextDocument.URI)
}

func (s *Server) handleDidChange(msg *jsonrpc.Message) {
	var params protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	if len(params.ContentChanges) > 0 {
		s.docs.Update(params.TextDocument.URI, params.ContentChanges[0].Text, params.TextDocument.Version)
	}
	s.scheduleDiagnose(params.TextDocument.URI)
}

func (s *Server) handleDidClose(msg *jsonrpc.Message) {
	var params protocol.DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	s.docs.Close(params.TextDocument.URI)
	// Clear diagnostics for closed document.
	s.notify("textDocument/publishDiagnostics", protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
}

func (s *Server) handleDidSave(msg *jsonrpc.Message) {
	var params protocol.DidSaveTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}
	if params.Text != nil {
		s.docs.Update(params.TextDocument.URI, *params.Text, -1)
	}
	s.scheduleDiagnose(params.TextDocument.URI)
}

// ---- Diagnostics ----

func (s *Server) scheduleDiagnose(uri protocol.DocumentURI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timer, ok := s.debounceTimers[uri]; ok {
		timer.Stop()
	}
	s.debounceTimers[uri] = time.AfterFunc(s.debounceDelay, func() {
		s.diagnose(uri)
	})
}

func (s *Server) diagnose(uri protocol.DocumentURI) {
	doc, ok := s.docs.Get(uri)
	if !ok {
		return
	}

	eng := s.engineSetup()
	eng.EnableTypeIndex()

	// Recursively resolve header directives (--module, --recursion).
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ar := eng.Analyze(ctx, doc.Text)
	s.docs.SetAnalysis(uri, ar)

	diags := convertDiagnostics(ar)
	if diags == nil {
		diags = []protocol.Diagnostic{} // empty array, not null
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

	doc, ok := s.docs.Get(params.TextDocument.URI)
	if !ok || doc.Analysis == nil || doc.Analysis.TypeIndex == nil || doc.Analysis.Source == nil {
		resp, _ := jsonrpc.NewResponse(msg.ID, nil)
		s.respond(resp)
		return
	}

	offset := posToOffset(doc.Analysis.Source, params.Position)
	ty := doc.Analysis.TypeIndex.TypeAt(offset)
	if ty == nil {
		resp, _ := jsonrpc.NewResponse(msg.ID, nil)
		s.respond(resp)
		return
	}

	hover := protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: "```gicel\n" + types.Pretty(ty) + "\n```",
		},
	}
	resp, _ := jsonrpc.NewResponse(msg.ID, hover)
	s.respond(resp)
}
