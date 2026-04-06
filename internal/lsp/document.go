package lsp

import (
	"sync"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

// Document tracks the state of an open text document.
type Document struct {
	URI      protocol.DocumentURI
	Text     string
	Version  int
	Analysis *engine.AnalysisResult // latest analysis result (may be nil)
}

// DocumentStore manages open documents with thread-safe access.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[protocol.DocumentURI]*Document
}

// NewDocumentStore creates an empty store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{docs: make(map[protocol.DocumentURI]*Document)}
}

// Open registers a new document.
func (ds *DocumentStore) Open(uri protocol.DocumentURI, text string, version int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.docs[uri] = &Document{URI: uri, Text: text, Version: version}
}

// Update replaces the document text.
func (ds *DocumentStore) Update(uri protocol.DocumentURI, text string, version int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if doc, ok := ds.docs[uri]; ok {
		doc.Text = text
		if version >= 0 {
			doc.Version = version
		}
	}
}

// Close removes a document.
func (ds *DocumentStore) Close(uri protocol.DocumentURI) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.docs, uri)
}

// Get returns a snapshot of the document for a URI.
func (ds *DocumentStore) Get(uri protocol.DocumentURI) (Document, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	doc, ok := ds.docs[uri]
	if !ok {
		return Document{}, false
	}
	return *doc, true
}

// SetAnalysis stores the latest analysis result for a document.
func (ds *DocumentStore) SetAnalysis(uri protocol.DocumentURI, ar *engine.AnalysisResult) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if doc, ok := ds.docs[uri]; ok {
		doc.Analysis = ar
	}
}
