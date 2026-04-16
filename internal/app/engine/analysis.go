package engine

import (
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
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
	Module        string // source module (non-empty for imported bindings)
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
	NameS    span.Span // name-only span for selectionRange (zero → falls back to S)
	Children []DocumentSymbolEntry
}

// DefinitionEntry maps a declared name to its source span.
type DefinitionEntry struct {
	Name     string
	S        span.Span
	FilePath string       // non-empty for cross-module definitions
	Source   *span.Source // non-nil for cross-module (needed for spanToRange)
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
		if dd.Generated.IsGenerated() {
			continue
		}
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
		mod := ar.ImportedModules[name]
		qualName := name
		if mod != "" {
			qualName = mod + "." + name
		}
		items = append(items, CompletionEntry{
			Label:         name,
			Kind:          CompletionVariable,
			Detail:        sig,
			Documentation: completionDoc(qualName, sig),
			Module:        mod,
		})
	}

	return items
}

// buildDocumentSymbolEntries produces document symbol data from analysis results.
func buildDocumentSymbolEntries(ar *AnalysisResult) []DocumentSymbolEntry {
	prog := ar.Program
	var symbols []DocumentSymbolEntry

	for _, b := range prog.Bindings {
		if b.Generated.IsGenerated() || b.S.IsZero() {
			continue
		}
		symbols = append(symbols, DocumentSymbolEntry{
			Name:   b.Name,
			Detail: types.PrettyDisplay(b.Type),
			Kind:   SymbolFunction,
			S:      b.S,
			NameS:  nameSpan(ar.Source, b.S, b.Name),
		})
	}

	for i := range prog.DataDecls {
		dd := &prog.DataDecls[i]
		if dd.S.IsZero() {
			continue
		}
		if dd.Generated == ir.GenDict {
			// Show class dictionary as the original class name.
			name := strings.TrimSuffix(dd.Name, ir.DictSuffix)
			symbols = append(symbols, DocumentSymbolEntry{
				Name:  name,
				Kind:  SymbolClass,
				S:     dd.S,
				NameS: nameSpan(ar.Source, dd.S, name),
			})
			continue
		}
		sym := DocumentSymbolEntry{
			Name:   dd.Name,
			Detail: types.PrettyTypeAsKind(ComputeFormKind(dd)),
			Kind:   SymbolStruct,
			S:      dd.S,
			NameS:  nameSpan(ar.Source, dd.S, dd.Name),
		}
		for j := range dd.Cons {
			con := &dd.Cons[j]
			if con.S.IsZero() {
				continue
			}
			sym.Children = append(sym.Children, DocumentSymbolEntry{
				Name:   con.Name,
				Detail: types.PrettyDisplay(BuildConType(dd, con)),
				Kind:   SymbolConstructor,
				S:      con.S,
				NameS:  nameSpan(ar.Source, con.S, con.Name),
			})
		}
		symbols = append(symbols, sym)
	}

	if ar.AST != nil {
		for _, d := range ar.AST.Decls {
			switch decl := d.(type) {
			case *syntax.DeclTypeAlias:
				if !decl.S.IsZero() {
					symbols = append(symbols, DocumentSymbolEntry{
						Name:  decl.Name,
						Kind:  SymbolStruct,
						S:     decl.S,
						NameS: nameSpan(ar.Source, decl.S, decl.Name),
					})
				}
			case *syntax.DeclImpl:
				if !decl.S.IsZero() {
					name := decl.Name
					if name == "" {
						if ann := decl.Ann.Span(); !ann.IsZero() && ar.Source != nil &&
							int(ann.End) <= len(ar.Source.Text) {
							name = "impl " + ar.Source.Text[ann.Start:ann.End]
						} else {
							name = "impl"
						}
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

// nameSpan locates the identifier name within the declaration span and returns
// a span covering only the name. Falls back to the full span if the name cannot
// be found (e.g., nil source or name not present in the span text).
func nameSpan(src *span.Source, declSpan span.Span, name string) span.Span {
	if src == nil || declSpan.IsZero() || name == "" {
		return declSpan
	}
	text := src.Text
	start := int(declSpan.Start)
	end := int(declSpan.End)
	if end > len(text) {
		end = len(text)
	}
	if start >= end {
		return declSpan
	}
	idx := strings.Index(text[start:end], name)
	if idx < 0 {
		return declSpan
	}
	ns := span.Pos(start + idx)
	ne := ns + span.Pos(len(name))
	return span.Span{Start: ns, End: ne}
}

// buildDefinitionEntries collects named declarations with their source spans.
// Includes cross-module definitions for imported bindings when file paths
// are available in the module store.
func buildDefinitionEntries(ar *AnalysisResult, store *ModuleStore) []DefinitionEntry {
	prog := ar.Program
	var entries []DefinitionEntry

	// Same-file definitions.
	for _, b := range prog.Bindings {
		if !b.S.IsZero() {
			entries = append(entries, DefinitionEntry{Name: b.Name, S: b.S})
		}
	}

	for i := range prog.DataDecls {
		dd := &prog.DataDecls[i]
		if dd.Generated.IsGenerated() || dd.S.IsZero() {
			continue
		}
		entries = append(entries, DefinitionEntry{Name: dd.Name, S: dd.S})
		for j := range dd.Cons {
			con := &dd.Cons[j]
			if !con.S.IsZero() {
				entries = append(entries, DefinitionEntry{Name: con.Name, S: con.S})
			}
		}
	}

	if ar.AST != nil {
		for _, d := range ar.AST.Decls {
			if alias, ok := d.(*syntax.DeclTypeAlias); ok {
				if !alias.S.IsZero() {
					entries = append(entries, DefinitionEntry{Name: alias.Name, S: alias.S})
				}
			}
		}
	}

	// Cross-module definitions: for each imported binding, look up the
	// source module and find the binding's span within that module.
	if store != nil {
		sameFile := make(map[string]bool, len(entries))
		for _, e := range entries {
			sameFile[e.Name] = true
		}
		for name, modName := range ar.ImportedModules {
			if sameFile[name] {
				continue // same-file definition takes precedence
			}
			mod, ok := store.modules[modName]
			if !ok || mod.filePath == "" || mod.prog == nil {
				continue
			}
			for _, b := range mod.prog.Bindings {
				if b.Name == name && !b.S.IsZero() {
					entries = append(entries, DefinitionEntry{
						Name:     name,
						S:        b.S,
						FilePath: mod.filePath,
						Source:   mod.source,
					})
					break
				}
			}
		}
	}

	return entries
}
