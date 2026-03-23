package types

import (
	"fmt"
	"strconv"
	"strings"
)

// WriteTypeKey writes a canonical, injective structural key for a type.
// The encoding is deterministic and collision-free: distinct types always
// produce distinct keys. This function is total over all Type variants.
//
// Used for:
//   - type family reduction cache keys
//   - instance dictionary binding names
//
// The key format uses unambiguous delimiters:
//   - TyCon: Name
//   - TyVar: 'Name
//   - TyMeta: ?ID
//   - TySkolem: #ID
//   - TyApp: (Fun Arg)
//   - TyArrow: (From->To)
//   - TyCBPV: {C Pre Post Result} or {T Pre Post Result}
//   - TyForall: {V Var Body}
//   - TyFamilyApp: [Name Arg1 Arg2 ...]
//   - TyEvidence: {E Constraints Body}
//   - TyEvidenceRow: capability = {R Label:Type ...}, constraint = {Q Class Args ...}
//   - TyError: !
func WriteTypeKey(b *strings.Builder, t Type) {
	switch ty := t.(type) {
	case *TyCon:
		b.WriteString(ty.Name)
	case *TyVar:
		b.WriteByte('\'')
		b.WriteString(ty.Name)
	case *TyMeta:
		b.WriteByte('?')
		var buf [20]byte
		b.Write(strconv.AppendInt(buf[:0], int64(ty.ID), 10))
	case *TyApp:
		b.WriteByte('(')
		WriteTypeKey(b, ty.Fun)
		b.WriteByte(' ')
		WriteTypeKey(b, ty.Arg)
		b.WriteByte(')')
	case *TyArrow:
		b.WriteByte('(')
		WriteTypeKey(b, ty.From)
		b.WriteString("->")
		WriteTypeKey(b, ty.To)
		b.WriteByte(')')
	case *TyCBPV:
		if ty.Tag == TagComp {
			b.WriteString("{C ")
		} else {
			b.WriteString("{T ")
		}
		WriteTypeKey(b, ty.Pre)
		b.WriteByte(' ')
		WriteTypeKey(b, ty.Post)
		b.WriteByte(' ')
		WriteTypeKey(b, ty.Result)
		b.WriteByte('}')
	case *TyForall:
		b.WriteString("{V ")
		b.WriteString(ty.Var)
		b.WriteByte(' ')
		WriteTypeKey(b, ty.Body)
		b.WriteByte('}')
	case *TyFamilyApp:
		b.WriteByte('[')
		b.WriteString(ty.Name)
		for _, a := range ty.Args {
			b.WriteByte(' ')
			WriteTypeKey(b, a)
		}
		b.WriteByte(']')
	case *TySkolem:
		b.WriteByte('#')
		var buf [20]byte
		b.Write(strconv.AppendInt(buf[:0], int64(ty.ID), 10))
	case *TyEvidence:
		b.WriteString("{E ")
		WriteTypeKey(b, ty.Constraints)
		b.WriteByte(' ')
		WriteTypeKey(b, ty.Body)
		b.WriteByte('}')
	case *TyEvidenceRow:
		writeEvidenceRowKey(b, ty)
	case *TyError:
		b.WriteByte('!')
	case nil:
		b.WriteString("_")
	default:
		// All Type variants are handled above. If this panics, a new
		// variant was added without updating WriteTypeKey.
		panic(fmt.Sprintf("WriteTypeKey: unhandled type %T", t))
	}
}

// TypeKey returns the canonical structural key for a type as a string.
func TypeKey(t Type) string {
	var b strings.Builder
	WriteTypeKey(&b, t)
	return b.String()
}

func writeEvidenceRowKey(b *strings.Builder, row *TyEvidenceRow) {
	switch entries := row.Entries.(type) {
	case *CapabilityEntries:
		b.WriteString("{R")
		for _, f := range entries.Fields {
			b.WriteByte(' ')
			b.WriteString(f.Label)
			b.WriteByte(':')
			WriteTypeKey(b, f.Type)
			for _, g := range f.Grades {
				b.WriteByte('@')
				WriteTypeKey(b, g)
			}
		}
		if row.Tail != nil {
			b.WriteString("|")
			WriteTypeKey(b, row.Tail)
		}
		b.WriteByte('}')
	case *ConstraintEntries:
		b.WriteString("{Q")
		for _, e := range entries.Entries {
			b.WriteByte(' ')
			b.WriteString(e.ClassName)
			for _, a := range e.Args {
				b.WriteByte(':')
				WriteTypeKey(b, a)
			}
		}
		if row.Tail != nil {
			b.WriteString("|")
			WriteTypeKey(b, row.Tail)
		}
		b.WriteByte('}')
	default:
		b.WriteString("{R}")
	}
}
