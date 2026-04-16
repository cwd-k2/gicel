package lsp

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

// Compile-time verification that engine.SymbolKind constants match the
// corresponding LSP protocol values. A mismatch is a build error, not a
// runtime surprise.
var _ = [1]struct{}{}[int(engine.SymbolFunction)-int(protocol.SKFunction)]
var _ = [1]struct{}{}[int(engine.SymbolConstructor)-int(protocol.SKConstructor)]
var _ = [1]struct{}{}[int(engine.SymbolClass)-int(protocol.SKClass)]
var _ = [1]struct{}{}[int(engine.SymbolStruct)-int(protocol.SKStruct)]

// convertDiagnostics transforms an AnalysisResult's errors into LSP diagnostics.
func convertDiagnostics(ar *engine.AnalysisResult) []protocol.Diagnostic {
	if ar.Errors == nil || !ar.Errors.HasErrors() {
		return nil
	}
	src := ar.Source
	diags := make([]protocol.Diagnostic, 0, len(ar.Errors.Errs))
	for _, e := range ar.Errors.Errs {
		d := protocol.Diagnostic{
			Severity: protocol.SeverityError,
			Source:   "gicel",
			Code:     fmt.Sprintf("E%04d", e.Code),
			Message:  e.Message,
		}
		if !e.Span.IsZero() && src != nil {
			d.Range = spanToRange(src, e.Span)
		}
		diags = append(diags, d)
	}
	return diags
}

// spanToRange converts a GICEL span (byte offsets) to an LSP Range (0-based line/character).
func spanToRange(src *span.Source, sp span.Span) protocol.Range {
	startLine, startCol := src.Location(sp.Start)
	endLine, endCol := src.Location(sp.End)
	return protocol.Range{
		Start: protocol.Position{Line: startLine - 1, Character: startCol - 1},
		End:   protocol.Position{Line: endLine - 1, Character: endCol - 1},
	}
}

// posToOffset converts an LSP Position (0-based) to a byte offset in the source.
func posToOffset(src *span.Source, pos protocol.Position) span.Pos {
	line := pos.Line
	if line < 0 {
		return 0
	}
	if line >= len(src.Lines) {
		return span.Pos(len(src.Text))
	}
	lineStart := int(src.Lines[line])
	char := max(pos.Character, 0)
	offset := min(lineStart+char, len(src.Text))
	return span.Pos(offset)
}

// convertDocumentSymbols transforms pre-computed symbol entries into LSP DocumentSymbols.
func convertDocumentSymbols(entries []engine.DocumentSymbolEntry, src *span.Source) []protocol.DocumentSymbol {
	symbols := make([]protocol.DocumentSymbol, 0, len(entries))
	for _, e := range entries {
		nameS := e.NameS
		if nameS.IsZero() {
			nameS = e.S
		}
		sym := protocol.DocumentSymbol{
			Name:           e.Name,
			Detail:         e.Detail,
			Kind:           protocol.SymbolKind(e.Kind),
			Range:          spanToRange(src, e.S),
			SelectionRange: spanToRange(src, nameS),
		}
		if len(e.Children) > 0 {
			sym.Children = convertDocumentSymbols(e.Children, src)
		}
		symbols = append(symbols, sym)
	}
	return symbols
}
