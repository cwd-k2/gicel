package vm

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Proto describes a compiled function or thunk body that can be instantiated
// as a VMClosure or VMThunkVal at runtime. Each ir.Lam, ir.Thunk, and ir.Fix
// compiles to a Proto.
type Proto struct {
	Code      []byte       // bytecode stream
	Constants []eval.Value // constant pool (literals, PrimVal stubs)
	Strings   []string     // string pool (constructor names, labels, keys)
	Protos    []*Proto     // nested prototypes (closures/thunks defined inside)

	NumLocals int    // total local variable slots required
	Captures  []int  // de Bruijn indices in enclosing scope to capture
	IsThunk   bool   // true for thunk bodies, false for closure bodies
	ParamName string // parameter name (closures only; "" for thunks)

	// FixSelfSlot is the local slot where the self-reference is stored
	// for Fix-derived prototypes. -1 if this is not a fix prototype.
	FixSelfSlot int

	// MatchDescs holds match descriptors referenced by MATCH_CON/MATCH_RECORD.
	MatchDescs []MatchDesc

	// RecordDescs holds record construction/update descriptors.
	RecordDescs []RecordDesc

	// Debug information.
	Spans     []SpanEntry  // bytecode offset → source span
	Source    *span.Source // source text for error attribution
	BindNames []BindInfo   // maps OpBind slot → variable name (for observer)
}

// BindInfo records the variable name for an OpBind instruction.
type BindInfo struct {
	Slot int
	Name string
}

// SpanEntry maps a bytecode offset range to a source span.
type SpanEntry struct {
	Offset int
	Span   span.Span
}

// MatchDesc describes how to decompose a constructor or record pattern.
type MatchDesc struct {
	// ConName is the constructor name (for constructor patterns).
	ConName string
	// ArgSlots maps each constructor argument to a local slot.
	// For nested patterns, the argument is first stored in its slot,
	// then further match instructions test it.
	ArgSlots []int
	// Labels lists field labels (for record patterns).
	Labels []string
	// FieldSlots maps each field to a local slot (for record patterns).
	FieldSlots []int
}

// RecordDesc describes fields for record construction or update.
type RecordDesc struct {
	Labels []string // field labels in the order values appear on the stack
}

// SpanAt returns the source span for the given bytecode offset,
// using binary search on the sorted Spans slice.
func (p *Proto) SpanAt(offset int) span.Span {
	spans := p.Spans
	if len(spans) == 0 {
		return span.Span{}
	}
	lo, hi := 0, len(spans)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if spans[mid].Offset <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return spans[lo].Span
}
