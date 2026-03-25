package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// inferRecord infers the type of a record literal { l1: e1, ..., ln: en }.
// Type: Record { l1: T1, ..., ln: Tn }
func (ch *Checker) inferRecord(e *syntax.ExprRecord) (types.Type, ir.Core) {
	seen := make(map[string]bool, len(e.Fields))
	var fields []types.RowField
	var coreFields []ir.RecordField
	for _, f := range e.Fields {
		if seen[f.Label] {
			ch.addCodedError(diagnostic.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record literal", f.Label))
			continue
		}
		seen[f.Label] = true
		ty, coreVal := ch.infer(f.Value)
		fields = append(fields, types.RowField{Label: f.Label, Type: ty, S: f.S})
		coreFields = append(coreFields, ir.RecordField{Label: f.Label, Value: coreVal})
	}
	row := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{Fields: fields},
		S:       e.S,
	}
	recTy := &types.TyApp{Fun: types.Con("Record"), Arg: row, S: e.S}
	return ch.unifier.Zonk(recTy), &ir.RecordLit{Fields: coreFields, S: e.S}
}

// inferProject infers the type of a record projection r.#label.
func (ch *Checker) inferProject(e *syntax.ExprProject) (types.Type, ir.Core) {
	recTy, recCore := ch.infer(e.Record)
	fieldTy := ch.matchRecordField(recTy, e.Label, e.S)
	projCore := &ir.RecordProj{Record: recCore, Label: e.Label, S: e.S}
	return ch.instantiate(fieldTy, projCore)
}

// matchRecordField extracts a field's type from a Record row type.
// If the type is not a Record or the field doesn't exist, reports an error and returns a meta.
func (ch *Checker) matchRecordField(ty types.Type, label string, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	// Decompose TyApp(TyCon("Record"), row).
	if app, ok := ty.(*types.TyApp); ok {
		if con, ok := app.Fun.(*types.TyCon); ok && con.Name == "Record" {
			row := ch.unifier.Zonk(app.Arg)
			// Try to find the label in the row.
			if evRow, ok := row.(*types.TyEvidenceRow); ok {
				if cap, ok := evRow.Entries.(*types.CapabilityEntries); ok {
					for _, f := range cap.Fields {
						if f.Label == label {
							return f.Type
						}
					}
				}
			}
			// Row might be a meta or open row — unify to extract the field.
			fieldMeta := ch.freshMeta(types.KType{})
			tailMeta := ch.freshMeta(types.KRow{})
			expectedRow := &types.TyEvidenceRow{
				Entries: &types.CapabilityEntries{
					Fields: []types.RowField{{Label: label, Type: fieldMeta}},
				},
				Tail: tailMeta,
				S:    s,
			}
			if err := ch.unifier.Unify(row, expectedRow); err != nil {
				ch.addCodedError(diagnostic.ErrRowMismatch, s, fmt.Sprintf("record has no field %s: %s", label, err.Error()))
				return ch.freshMeta(types.KType{})
			}
			return ch.unifier.Zonk(fieldMeta)
		}
	}
	// Type might be a meta — try to unify with Record { label : ?m | ?tail }.
	fieldMeta := ch.freshMeta(types.KType{})
	tailMeta := ch.freshMeta(types.KRow{})
	expectedRow := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{{Label: label, Type: fieldMeta}},
		},
		Tail: tailMeta,
		S:    s,
	}
	expectedRecTy := &types.TyApp{Fun: types.Con("Record"), Arg: expectedRow, S: s}
	if err := ch.unifier.Unify(ty, expectedRecTy); err != nil {
		ch.addCodedError(diagnostic.ErrRowMismatch, s, fmt.Sprintf("expected record with field %s, got %s", label, types.Pretty(ty)))
		return ch.freshMeta(types.KType{})
	}
	return ch.unifier.Zonk(fieldMeta)
}

// inferRecordUpdate infers the type of a record update { r | l1: e1, ..., ln: en }.
func (ch *Checker) inferRecordUpdate(e *syntax.ExprRecordUpdate) (types.Type, ir.Core) {
	recTy, recCore := ch.infer(e.Record)
	coreUpdates := make([]ir.RecordField, len(e.Updates))
	seen := make(map[string]bool, len(e.Updates))
	for i, upd := range e.Updates {
		if seen[upd.Label] {
			ch.addCodedError(diagnostic.ErrDuplicateLabel, upd.S,
				fmt.Sprintf("duplicate label %q in record update", upd.Label))
		}
		seen[upd.Label] = true
		// Infer the update value type, then check it matches the existing field.
		fieldTy := ch.matchRecordField(recTy, upd.Label, upd.S)
		updCore := ch.check(upd.Value, fieldTy)
		coreUpdates[i] = ir.RecordField{Label: upd.Label, Value: updCore}
	}
	return recTy, &ir.RecordUpdate{Record: recCore, Updates: coreUpdates, S: e.S}
}

// checkRecord checks a record literal against an expected record type,
// propagating expected field types to enable higher-rank fields.
func (ch *Checker) checkRecord(e *syntax.ExprRecord, expected types.Type) ir.Core {
	// Try to extract expected field types from the expected record type.
	expectedFields := ch.extractRecordFieldTypes(expected)
	if expectedFields == nil {
		// Can't decompose — fall back to infer + subsCheck.
		inferredTy, coreExpr := ch.inferRecord(e)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	seen := make(map[string]bool, len(e.Fields))
	var coreFields []ir.RecordField
	var rowFields []types.RowField
	for _, f := range e.Fields {
		if seen[f.Label] {
			ch.addCodedError(diagnostic.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record literal", f.Label))
			continue
		}
		seen[f.Label] = true
		if fieldTy, ok := expectedFields[f.Label]; ok {
			coreVal := ch.check(f.Value, fieldTy)
			coreFields = append(coreFields, ir.RecordField{Label: f.Label, Value: coreVal})
			rowFields = append(rowFields, types.RowField{Label: f.Label, Type: fieldTy, S: f.S})
		} else {
			ty, coreVal := ch.infer(f.Value)
			coreFields = append(coreFields, ir.RecordField{Label: f.Label, Value: coreVal})
			rowFields = append(rowFields, types.RowField{Label: f.Label, Type: ty, S: f.S})
		}
	}
	row := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{Fields: rowFields},
		S:       e.S,
	}
	recTy := &types.TyApp{Fun: types.Con("Record"), Arg: row, S: e.S}
	coreExpr := &ir.RecordLit{Fields: coreFields, S: e.S}
	return ch.subsCheck(ch.unifier.Zonk(recTy), expected, coreExpr, e.S)
}

// extractRecordFieldTypes decomposes a Record type into a map of field types.
// Returns nil if the type is not a decomposable Record.
func (ch *Checker) extractRecordFieldTypes(ty types.Type) map[string]types.Type {
	ty = ch.unifier.Zonk(ty)
	app, ok := ty.(*types.TyApp)
	if !ok {
		return nil
	}
	con, ok := app.Fun.(*types.TyCon)
	if !ok || con.Name != "Record" {
		return nil
	}
	row := ch.unifier.Zonk(app.Arg)
	evRow, ok := row.(*types.TyEvidenceRow)
	if !ok {
		return nil
	}
	cap, ok := evRow.Entries.(*types.CapabilityEntries)
	if !ok {
		return nil
	}
	result := make(map[string]types.Type, len(cap.Fields))
	for _, f := range cap.Fields {
		result[f.Label] = f.Type
	}
	return result
}

// checkRecordPattern checks a record pattern { l1: p1, ..., ln: pn } against a scrutinee type.
func (ch *Checker) checkRecordPattern(p *syntax.PatRecord, scrutTy types.Type) patternResult {
	bindings := make(map[string]types.Type)
	coreFields := make([]ir.PRecordField, len(p.Fields))
	seen := make(map[string]bool, len(p.Fields))
	for i, f := range p.Fields {
		if seen[f.Label] {
			ch.addCodedError(diagnostic.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record pattern", f.Label))
		}
		seen[f.Label] = true
		fieldTy := ch.matchRecordField(scrutTy, f.Label, f.S)
		child := ch.checkPattern(f.Pattern, fieldTy)
		coreFields[i] = ir.PRecordField{Label: f.Label, Pattern: child.Pattern}
		for k, v := range child.Bindings {
			bindings[k] = v
		}
	}
	return patternResult{
		Pattern:  &ir.PRecord{Fields: coreFields, S: p.S},
		Bindings: bindings,
	}
}
