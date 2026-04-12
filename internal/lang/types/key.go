package types

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// KeyWriter is the minimal writer interface accepted by WriteTypeKey.
// Both *strings.Builder and *bytes.Buffer satisfy it, so callers can
// build keys into either a one-shot growable buffer (for TypeKey-style
// callers) or a reusable scratch buffer (for engine fingerprinting).
type KeyWriter interface {
	io.Writer
	io.ByteWriter
	io.StringWriter
}

// WriteTypeKey writes a canonical, injective structural key for a type.
// The encoding is deterministic and collision-free: distinct types always
// produce distinct keys. This function is total over all Type variants.
//
// Used for:
//   - type family reduction cache keys
//   - instance dictionary binding names
//   - engine fingerprinting (via shared scratch buffer)
//
// The key format uses unambiguous delimiters:
//   - TyCon: Name
//   - TyVar: 'Name
//   - TyMeta: ?ID
//   - TySkolem: #ID
//   - TyApp: (Fun Arg)
//   - TyArrow: (From->To)
//   - TyCBPV: {C Pre Post Result} or {T Pre Post Result}
//   - TyForall: {V Var:Kind Body}
//   - TyFamilyApp: [Name Arg1 Arg2 ...]
//   - TyEvidence: {E Constraints Body}
//   - TyEvidenceRow: capability = {R Label:Type ...}, constraint = {Q Class Args ...}
//   - TyError: !
func WriteTypeKey(b KeyWriter, t Type) {
	switch ty := t.(type) {
	case *TyCon:
		b.WriteString(ty.Name)
		if ty.Level != nil && !IsValueLevel(ty.Level) {
			b.WriteByte('#')
			b.WriteString(ty.Level.LevelString())
		}
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
		if ty.Grade != nil {
			b.WriteString(" @")
			WriteTypeKey(b, ty.Grade)
		}
		b.WriteByte('}')
	case *TyForall:
		b.WriteString("{V ")
		b.WriteString(ty.Var)
		b.WriteByte(':')
		WriteTypeKey(b, ty.Kind)
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
		if ty.Kind != nil {
			b.WriteByte(':')
			WriteTypeKey(b, ty.Kind)
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

var builderPool = sync.Pool{
	New: func() any { return &strings.Builder{} },
}

// TypeKey returns the canonical structural key for a type as a string.
func TypeKey(t Type) string {
	b := builderPool.Get().(*strings.Builder)
	b.Reset()
	WriteTypeKey(b, t)
	s := b.String()
	builderPool.Put(b)
	return s
}

// TypeListKey serializes a prefix followed by type arguments into a canonical key.
// Each argument is preceded by the given separator byte.
func TypeListKey(prefix string, sep byte, args []Type) string {
	b := builderPool.Get().(*strings.Builder)
	b.Reset()
	b.Grow(len(prefix) + len(args)*16)
	b.WriteString(prefix)
	for _, a := range args {
		b.WriteByte(sep)
		WriteTypeKey(b, a)
	}
	s := b.String()
	builderPool.Put(b)
	return s
}

func writeEvidenceRowKey(b KeyWriter, row *TyEvidenceRow) {
	switch entries := row.Entries.(type) {
	case *CapabilityEntries:
		b.WriteString("{R")
		// Cap rows are maintained in sorted order by ExtendRow/ClosedRow/OpenRow,
		// so normalization is not needed here.
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
		// Constraint rows are maintained in sorted order by ExtendConstraint,
		// so normalization is not needed here.
		for _, e := range entries.Entries {
			b.WriteByte(' ')
			writeConstraintEntryKey(b, e)
		}
		if row.Tail != nil {
			b.WriteString("|")
			WriteTypeKey(b, row.Tail)
		}
		b.WriteByte('}')
	default:
		// Generic fallback for future fiber types.
		b.WriteString("{X")
		for _, child := range row.Entries.AllChildren() {
			b.WriteByte(' ')
			WriteTypeKey(b, child)
		}
		if row.Tail != nil {
			b.WriteString("|")
			WriteTypeKey(b, row.Tail)
		}
		b.WriteByte('}')
	}
}

// writeConstraintEntryKey writes a canonical key for a single constraint entry.
// The leading byte discriminates variants so keys of distinct variants can
// never collide:
//
//	C  ClassEntry          "C<name>:<arg>:<arg>..."
//	E  EqualityEntry       "E<lhs>:<rhs>"
//	V  VarEntry            "V<var>"
//	Q  QuantifiedConstraint "Q<head>"  (full quantifier encoding deferred)
func writeConstraintEntryKey(b KeyWriter, e ConstraintEntry) {
	switch e := e.(type) {
	case *ClassEntry:
		b.WriteByte('C')
		b.WriteString(e.ClassName)
		for _, a := range e.Args {
			b.WriteByte(':')
			WriteTypeKey(b, a)
		}
	case *EqualityEntry:
		b.WriteByte('E')
		WriteTypeKey(b, e.Lhs)
		b.WriteByte(':')
		WriteTypeKey(b, e.Rhs)
	case *VarEntry:
		b.WriteByte('V')
		WriteTypeKey(b, e.Var)
	case *QuantifiedConstraint:
		b.WriteByte('Q')
		if e.Head != nil {
			b.WriteString(e.Head.ClassName)
			for _, a := range e.Head.Args {
				b.WriteByte(':')
				WriteTypeKey(b, a)
			}
		}
	}
}
