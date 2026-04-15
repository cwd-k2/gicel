package lsp

import (
	"context"

	"github.com/cwd-k2/gicel/internal/app/engine"
)

// AnalysisEngine abstracts the compilation/analysis pipeline for LSP consumers.
// *engine.Engine satisfies this interface implicitly.
//
// The engine.AnalysisResult import is retained because it is a pure data
// structure with no mutable behavior. Full decoupling (moving AnalysisResult
// to a shared package) is warranted when a second consumer appears.
type AnalysisEngine interface {
	EnableHoverIndex()
	EnableRecursion()
	RegisterModule(name, source string) error
	Analyze(ctx context.Context, source string) *engine.AnalysisResult
}
