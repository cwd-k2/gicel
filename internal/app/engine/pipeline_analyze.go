// Analysis pipeline — LSP-oriented partial compilation (lex → parse → check).
// Separated from pipeline.go which handles the full compilation path.
package engine

import (
	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// analyze runs lex → parse → check, returning partial results on error.
// Unlike compileMain, it does NOT run postCheck (optimize, annotate).
func (pc *pipelineCtx) analyze(source string) *AnalysisResult {
	result := &AnalysisResult{}

	ast, src, err := pc.lexAndParse("<input>", source, pc.store.Has("Core"))
	result.Source = src
	if err != nil {
		if ce, ok := err.(*CompileError); ok {
			result.Errors = ce.errs
		}
		result.Program = &ir.Program{}
		return result
	}

	cfg := pc.makeCheckConfig()
	cfg.Context = pc.ctx
	cfg.EntryPoint = pc.pipelineFlags.effectiveEntryPoint()
	cfg.DenyAssumptions = pc.pipelineFlags.denyAssumptions

	var builder *HoverIndexBuilder
	if pc.typeRecorder {
		builder = NewHoverIndexBuilder()
		varDocs := pc.collectVarDocs()
		for _, d := range ast.Decls {
			switch decl := d.(type) {
			case *syntax.DeclValueDef:
				if decl.S != (span.Span{}) {
					if doc := ExtractDocComment(source, decl.S.Start); doc != "" {
						varDocs[decl.Name] = doc
					}
				}
			case *syntax.DeclTypeAnn:
				if decl.S != (span.Span{}) {
					if doc := ExtractDocComment(source, decl.S.Start); doc != "" {
						varDocs[decl.Name] = doc
					}
				}
			}
		}
		cfg.HoverRecorder = &hoverAdapter{
			idx:       builder,
			fixityMap: pc.collectFixityMap(ast),
			varDocs:   varDocs,
			source:    source,
		}
	}

	prog, checkErrs := check.Check(ast, src, cfg)
	if builder != nil {
		populateHoverDecls(builder, ast, prog, source)
		result.HoverIndex = builder.Finalize()
	}

	result.Program = prog
	result.AST = ast
	result.Errors = checkErrs
	result.Complete = !checkErrs.HasErrors()

	// Flatten imported bindings for completion.
	if cfg.ImportedModules != nil {
		imported := make(map[string]types.Type)
		modules := make(map[string]string)
		for modName, exports := range cfg.ImportedModules {
			for name, ty := range exports.Values {
				imported[name] = ty
				modules[name] = modName
			}
			for name, ty := range exports.ConTypes {
				imported[name] = ty
				modules[name] = modName
			}
		}
		if len(imported) > 0 {
			result.ImportedBindings = imported
			result.ImportedModules = modules
		}
	}

	// Pre-compute LSP data so the server needs no lang/types or lang/syntax imports.
	result.CompletionEntries = buildCompletionEntries(result)
	result.DocumentSymbols = buildDocumentSymbolEntries(result)
	result.Definitions = buildDefinitionEntries(result, pc.store)

	return result
}

// populateHoverDecls records declaration-level hover entries from the
// checked program and the original AST, extracting doc comments from source.
func populateHoverDecls(idx *HoverIndexBuilder, ast *syntax.AstProgram, prog *ir.Program, source string) {
	doc := func(s span.Span) string {
		return ExtractDocComment(source, s.Start)
	}

	// Binding definitions (skip compiler-generated dict bindings etc.).
	for _, b := range prog.Bindings {
		if b.Type != nil && b.S != (span.Span{}) && !b.Generated.IsGenerated() {
			idx.RecordDecl(b.S, HoverBinding, b.Name, b.Type, doc(b.S))
		}
	}

	// Form declarations and constructors.
	for i := range prog.DataDecls {
		dd := &prog.DataDecls[i]
		if dd.S != (span.Span{}) {
			idx.RecordDecl(dd.S, HoverForm, dd.Name, ComputeFormKind(dd), doc(dd.S))
		}
		for j := range dd.Cons {
			con := &dd.Cons[j]
			if con.S != (span.Span{}) {
				idx.RecordDecl(con.S, HoverConstructor, con.Name, BuildConType(dd, con), "")
			}
		}
	}

	// Type annotations (match with binding types).
	bindingTypes := make(map[string]types.Type, len(prog.Bindings))
	for _, b := range prog.Bindings {
		if b.Type != nil {
			bindingTypes[b.Name] = b.Type
		}
	}
	for _, d := range ast.Decls {
		if ann, ok := d.(*syntax.DeclTypeAnn); ok {
			if ty, found := bindingTypes[ann.Name]; found && ann.S != (span.Span{}) {
				idx.RecordDecl(ann.S, HoverTypeAnn, ann.Name, ty, doc(ann.S))
			}
		}
	}

	// Import declarations.
	for _, imp := range ast.Imports {
		if imp.S != (span.Span{}) {
			label := imp.ModuleName
			if imp.Alias != "" {
				label += " as " + imp.Alias
			}
			idx.RecordDecl(imp.S, HoverImport, label, nil, "")
		}
	}
}

// collectVarDocs builds a name→doc map from all imported module bindings.
func (pc *pipelineCtx) collectVarDocs() map[string]string {
	docs := make(map[string]string)
	for _, name := range pc.store.order {
		mod, ok := pc.store.modules[name]
		if !ok || mod.source == nil {
			continue
		}
		for _, b := range mod.prog.Bindings {
			if b.Generated.IsGenerated() || b.S == (span.Span{}) {
				continue
			}
			if d := ExtractDocComment(mod.source.Text, b.S.Start); d != "" {
				docs[b.Name] = d
			}
		}
		// Form declarations (class methods have doc on the form, not individual fields).
		for i := range mod.prog.DataDecls {
			dd := &mod.prog.DataDecls[i]
			if dd.S == (span.Span{}) {
				continue
			}
			if d := ExtractDocComment(mod.source.Text, dd.S.Start); d != "" {
				docs[dd.Name] = d
			}
		}
	}
	return docs
}

// fixityToHover converts a syntax.Fixity to hover display information.
func fixityToHover(f syntax.Fixity) *OperatorFixity {
	return &OperatorFixity{Assoc: f.Assoc.String(), Prec: f.Prec}
}

// collectFixityMap gathers the merged fixity map for the given AST:
// transitive imports (from the module store) + local fixity declarations.
func (pc *pipelineCtx) collectFixityMap(ast *syntax.AstProgram) map[string]syntax.Fixity {
	importNames := make([]string, len(ast.Imports))
	for i, imp := range ast.Imports {
		importNames[i] = imp.ModuleName
	}
	result := pc.store.CollectFixityMap(importNames)
	for _, d := range ast.Decls {
		if fix, ok := d.(*syntax.DeclFixity); ok {
			result[fix.Op] = syntax.Fixity{Assoc: fix.Assoc, Prec: fix.Prec}
		}
	}
	return result
}
