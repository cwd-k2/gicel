package lsp

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lsp/protocol"
)

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
		if e.Span != (span.Span{}) && src != nil {
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
	if line < 0 || line >= len(src.Lines) {
		if line < 0 {
			return 0
		}
		return span.Pos(len(src.Text))
	}
	offset := int(src.Lines[line]) + pos.Character
	if offset > len(src.Text) {
		offset = len(src.Text)
	}
	return span.Pos(offset)
}
