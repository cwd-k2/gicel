package engine

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// CompletionKind classifies completion entries (mirrors LSP CompletionItemKind).
type CompletionKind int

const (
	CompletionFunction    CompletionKind = 3
	CompletionConstructor CompletionKind = 4
	CompletionVariable    CompletionKind = 6
	CompletionStruct      CompletionKind = 22
)

// CompletionEntry is a pre-computed completion item for LSP.
// Type strings are pre-formatted by the engine so the LSP layer
// needs no direct dependency on lang/types.
type CompletionEntry struct {
	Label         string
	Detail        string // short classification (e.g., "function", "constructor")
	Documentation string // rich type signature as markdown code block
	Kind          CompletionKind
}

// SymbolKind classifies document symbols (mirrors LSP SymbolKind).
type SymbolKind int

const (
	SymbolFunction    SymbolKind = 12
	SymbolStruct      SymbolKind = 23
	SymbolConstructor SymbolKind = 9 // constructor
	SymbolClass       SymbolKind = 5
)

// DocumentSymbolEntry is a pre-computed document symbol for LSP.
type DocumentSymbolEntry struct {
	Name     string
	Detail   string
	Kind     SymbolKind
	S        span.Span
	Children []DocumentSymbolEntry
}

// DefinitionEntry maps a declared name to its source span.
type DefinitionEntry struct {
	Name string
	S    span.Span
}

// completionDoc formats a type signature as a markdown code block for LSP documentation.
func completionDoc(name, sig string) string {
	return "```gicel\n" + name + " :: " + sig + "\n```"
}

// buildCompletionEntries produces completion data from a compiled program.
func buildCompletionEntries(ar *AnalysisResult) []CompletionEntry {
	prog := ar.Program
	var items []CompletionEntry

	for _, b := range prog.Bindings {
		if b.Generated.IsGenerated() {
			continue
		}
		sig := types.PrettyDisplay(b.Type)
		items = append(items, CompletionEntry{
			Label:         b.Name,
			Kind:          CompletionFunction,
			Detail:        sig,
			Documentation: completionDoc(b.Name, sig),
		})
	}

	for i := range prog.DataDecls {
		dd := &prog.DataDecls[i]
		kind := types.PrettyTypeAsKind(ComputeFormKind(dd))
		items = append(items, CompletionEntry{
			Label:         dd.Name,
			Kind:          CompletionStruct,
			Detail:        kind,
			Documentation: completionDoc("form "+dd.Name, kind),
		})
		for j := range dd.Cons {
			con := &dd.Cons[j]
			sig := types.PrettyDisplay(BuildConType(dd, con))
			items = append(items, CompletionEntry{
				Label:         con.Name,
				Kind:          CompletionConstructor,
				Detail:        sig,
				Documentation: completionDoc(con.Name, sig),
			})
		}
	}

	for name, ty := range ar.ImportedBindings {
		sig := types.PrettyDisplay(ty)
		qualName := name
		if mod := ar.ImportedModules[name]; mod != "" {
			qualName = mod + "." + name
		}
		items = append(items, CompletionEntry{
			Label:         name,
			Kind:          CompletionVariable,
			Detail:        sig,
			Documentation: completionDoc(qualName, sig),
		})
	}

	return items
}

// buildDocumentSymbolEntries produces document symbol data from analysis results.
func buildDocumentSymbolEntries(ar *AnalysisResult) []DocumentSymbolEntry {
	prog := ar.Program
	var symbols []DocumentSymbolEntry

	for _, b := range prog.Bindings {
		if b.Generated.IsGenerated() || b.S == (span.Span{}) {
			continue
		}
		symbols = append(symbols, DocumentSymbolEntry{
			Name:   b.Name,
			Detail: types.PrettyDisplay(b.Type),
			Kind:   SymbolFunction,
			S:      b.S,
		})
	}

	for i := range prog.DataDecls {
		dd := &prog.DataDecls[i]
		if dd.S == (span.Span{}) {
			continue
		}
		sym := DocumentSymbolEntry{
			Name:   dd.Name,
			Detail: types.PrettyTypeAsKind(ComputeFormKind(dd)),
			Kind:   SymbolStruct,
			S:      dd.S,
		}
		for j := range dd.Cons {
			con := &dd.Cons[j]
			if con.S == (span.Span{}) {
				continue
			}
			sym.Children = append(sym.Children, DocumentSymbolEntry{
				Name:   con.Name,
				Detail: types.PrettyDisplay(BuildConType(dd, con)),
				Kind:   SymbolConstructor,
				S:      con.S,
			})
		}
		symbols = append(symbols, sym)
	}

	if ar.AST != nil {
		for _, d := range ar.AST.Decls {
			switch decl := d.(type) {
			case *syntax.DeclTypeAlias:
				if decl.S != (span.Span{}) {
					symbols = append(symbols, DocumentSymbolEntry{
						Name: decl.Name,
						Kind: SymbolStruct,
						S:    decl.S,
					})
				}
			case *syntax.DeclImpl:
				if decl.S != (span.Span{}) {
					name := decl.Name
					if name == "" {
						name = "impl"
					}
					symbols = append(symbols, DocumentSymbolEntry{
						Name: name,
						Kind: SymbolClass,
						S:    decl.S,
					})
				}
			}
		}
	}

	return symbols
}

// buildDefinitionEntries collects named declarations with their source spans.
func buildDefinitionEntries(ar *AnalysisResult) []DefinitionEntry {
	prog := ar.Program
	var entries []DefinitionEntry

	for _, b := range prog.Bindings {
		if b.S != (span.Span{}) {
			entries = append(entries, DefinitionEntry{Name: b.Name, S: b.S})
		}
	}

	for i := range prog.DataDecls {
		dd := &prog.DataDecls[i]
		if dd.S != (span.Span{}) {
			entries = append(entries, DefinitionEntry{Name: dd.Name, S: dd.S})
		}
		for j := range dd.Cons {
			con := &dd.Cons[j]
			if con.S != (span.Span{}) {
				entries = append(entries, DefinitionEntry{Name: con.Name, S: con.S})
			}
		}
	}

	if ar.AST != nil {
		for _, d := range ar.AST.Decls {
			if alias, ok := d.(*syntax.DeclTypeAlias); ok {
				if alias.S != (span.Span{}) {
					entries = append(entries, DefinitionEntry{Name: alias.Name, S: alias.S})
				}
			}
		}
	}

	return entries
}
