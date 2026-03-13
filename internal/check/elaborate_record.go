package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
)

// inferRecord infers the type of a record literal { l1 = e1, ..., ln = en }.
// Type: Record { l1 : T1, ..., ln : Tn }
func (ch *Checker) inferRecord(e *syntax.ExprRecord) (types.Type, core.Core) {
	fields := make([]types.RowField, len(e.Fields))
	coreFields := make([]core.RecordField, len(e.Fields))
	seen := make(map[string]bool, len(e.Fields))
	for i, f := range e.Fields {
		if seen[f.Label] {
			ch.addCodedError(errs.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record literal", f.Label))
		}
		seen[f.Label] = true
		ty, coreVal := ch.infer(f.Value)
		fields[i] = types.RowField{Label: f.Label, Type: ty, S: f.S}
		coreFields[i] = core.RecordField{Label: f.Label, Value: coreVal}
	}
	row := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{Fields: fields},
		S:       e.S,
	}
	recTy := &types.TyApp{Fun: &types.TyCon{Name: "Record"}, Arg: row, S: e.S}
	return ch.unifier.Zonk(recTy), &core.RecordLit{Fields: coreFields, S: e.S}
}

// inferProject infers the type of a record projection r!#label.
func (ch *Checker) inferProject(e *syntax.ExprProject) (types.Type, core.Core) {
	recTy, recCore := ch.infer(e.Record)
	fieldTy := ch.matchRecordField(recTy, e.Label, e.S)
	return fieldTy, &core.RecordProj{Record: recCore, Label: e.Label, S: e.S}
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
				ch.addCodedError(errs.ErrRowMismatch, s, fmt.Sprintf("record has no field %s: %s", label, err.Error()))
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
	expectedRecTy := &types.TyApp{Fun: &types.TyCon{Name: "Record"}, Arg: expectedRow, S: s}
	if err := ch.unifier.Unify(ty, expectedRecTy); err != nil {
		ch.addCodedError(errs.ErrRowMismatch, s, fmt.Sprintf("expected record with field %s, got %s", label, types.Pretty(ty)))
		return ch.freshMeta(types.KType{})
	}
	return ch.unifier.Zonk(fieldMeta)
}

// inferRecordUpdate infers the type of a record update { r | l1 = e1, ..., ln = en }.
func (ch *Checker) inferRecordUpdate(e *syntax.ExprRecordUpdate) (types.Type, core.Core) {
	recTy, recCore := ch.infer(e.Record)
	coreUpdates := make([]core.RecordField, len(e.Updates))
	seen := make(map[string]bool, len(e.Updates))
	for i, upd := range e.Updates {
		if seen[upd.Label] {
			ch.addCodedError(errs.ErrDuplicateLabel, upd.S,
				fmt.Sprintf("duplicate label %q in record update", upd.Label))
		}
		seen[upd.Label] = true
		// Infer the update value type, then check it matches the existing field.
		fieldTy := ch.matchRecordField(recTy, upd.Label, upd.S)
		updCore := ch.check(upd.Value, fieldTy)
		coreUpdates[i] = core.RecordField{Label: upd.Label, Value: updCore}
	}
	return recTy, &core.RecordUpdate{Record: recCore, Updates: coreUpdates, S: e.S}
}

// checkRecordPattern checks a record pattern { l1 = p1, ..., ln = pn } against a scrutinee type.
func (ch *Checker) checkRecordPattern(p *syntax.PatRecord, scrutTy types.Type) (core.Pattern, map[string]types.Type, map[int]string, bool) {
	bindings := make(map[string]types.Type)
	coreFields := make([]core.PRecordField, len(p.Fields))
	seen := make(map[string]bool, len(p.Fields))
	for i, f := range p.Fields {
		if seen[f.Label] {
			ch.addCodedError(errs.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record pattern", f.Label))
		}
		seen[f.Label] = true
		fieldTy := ch.matchRecordField(scrutTy, f.Label, f.S)
		corePat, fieldBindings, _, _ := ch.checkPattern(f.Pattern, fieldTy)
		coreFields[i] = core.PRecordField{Label: f.Label, Pattern: corePat}
		for k, v := range fieldBindings {
			bindings[k] = v
		}
	}
	return &core.PRecord{Fields: coreFields, S: p.S}, bindings, nil, false
}
