package engine

import (
	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// hoverAdapter implements check.HoverRecorder by delegating to a
// HoverIndex with additional pipeline context (fixity, docs, source).
type hoverAdapter struct {
	idx       *HoverIndexBuilder
	fixityMap map[string]syntax.Fixity
	varDocs   map[string]string
	source    string
}

func (a *hoverAdapter) RecordType(sp span.Span, ty types.Type) {
	a.idx.Record(sp, ty)
}

func (a *hoverAdapter) RecordOperator(sp span.Span, name, module string, ty types.Type) {
	var fix *OperatorFixity
	if f, ok := a.fixityMap[name]; ok {
		fix = fixityToHover(f)
	}
	a.idx.RecordOperator(sp, name, module, ty, fix)
}

func (a *hoverAdapter) RecordVarDoc(sp span.Span, name, module string) {
	a.idx.AttachVarInfo(sp, name, module, a.varDocs[name])
}

func (a *hoverAdapter) RecordDecl(sp span.Span, kind check.DeclKind, name string, ty types.Type) {
	doc := ExtractDocComment(a.source, sp.Start)
	switch kind {
	case check.DeclAlias:
		a.idx.RecordDecl(sp, HoverTypeAlias, name, ty, doc)
	case check.DeclImpl:
		a.idx.RecordDecl(sp, HoverImpl, name, ty, doc)
	default:
		panic("hoverAdapter.RecordDecl: unknown DeclKind")
	}
}

func (a *hoverAdapter) Rezonk(zonk func(types.Type) types.Type) {
	a.idx.RezonkAll(zonk)
}
