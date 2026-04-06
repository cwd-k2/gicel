package lsp

import (
	"sync"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

// document tracks the state of an open text document.
// Analysis is a pointer to an immutable AnalysisResult — once set,
// the pointee is never mutated. Readers may hold a reference while
// a new analysis replaces it in the store.
type document struct {
	URI      protocol.DocumentURI
	Text     string
	Version  int
	Analysis *engine.AnalysisResult
}

// documentStore manages open documents with thread-safe access.
type documentStore struct {
	mu   sync.RWMutex
	docs map[protocol.DocumentURI]*document
}

func newDocumentStore() *documentStore {
	return &documentStore{docs: make(map[protocol.DocumentURI]*document)}
}

func (ds *documentStore) open(uri protocol.DocumentURI, text string, version int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.docs[uri] = &document{URI: uri, Text: text, Version: version}
}

func (ds *documentStore) update(uri protocol.DocumentURI, text string, version int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if doc, ok := ds.docs[uri]; ok {
		doc.Text = text
		if version >= 0 {
			doc.Version = version
		}
	}
}

func (ds *documentStore) close(uri protocol.DocumentURI) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.docs, uri)
}

// get returns a snapshot of the document. The returned document's Analysis
// pointer refers to an immutable AnalysisResult that remains valid even
// if the store replaces it with a newer analysis.
func (ds *documentStore) get(uri protocol.DocumentURI) (document, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	doc, ok := ds.docs[uri]
	if !ok {
		return document{}, false
	}
	return *doc, true
}

func (ds *documentStore) setAnalysis(uri protocol.DocumentURI, ar *engine.AnalysisResult) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if doc, ok := ds.docs[uri]; ok {
		doc.Analysis = ar
	}
}
