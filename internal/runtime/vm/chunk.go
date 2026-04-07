package vm

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Proto describes a compiled function or thunk body that can be instantiated
// as a VMClosure or VMThunkVal at runtime. Each ir.Lam, ir.Thunk, and ir.Fix
// compiles to a Proto. Satisfies eval.Bytecode.
type Proto struct {
	Code      []byte       // bytecode stream
	Constants []eval.Value // constant pool (literals, PrimVal stubs)
	Strings   []string     // string pool (constructor names, labels, keys)
	Protos    []*Proto     // nested prototypes (closures/thunks defined inside)

	NumLocals int      // total local variable slots required
	Captures  []int    // de Bruijn indices in enclosing scope to capture
	IsThunk   bool     // true for thunk bodies, false for closure bodies
	Params    []string // parameter names (nil for thunks, 1+ for closures)

	// FixSelfSlot is the local slot where the self-reference is stored
	// for Fix-derived prototypes. -1 if this is not a fix prototype.
	FixSelfSlot int

	// MatchDescs holds match descriptors referenced by MATCH_CON/MATCH_RECORD.
	MatchDescs []MatchDesc

	// RecordDescs holds record construction/update descriptors.
	RecordDescs []RecordDesc

	// MergeDescs holds merge descriptors (CapEnv label split info).
	MergeDescs []MergeDesc

	// Resolved primitive implementations. Indexed by Strings pool index.
	// Populated at link time (precompileVM); nil entries fall back to
	// registry lookup. Only entries that correspond to primitive names
	// (referenced by OpPrim/OpEffectPrim) are populated.
	ResolvedPrims []eval.PrimImpl

	// Debug information.
	Spans     []SpanEntry  // bytecode offset → source span
	Source    *span.Source // source text for error attribution
	BindNames []BindInfo   // maps OpBind slot → variable name (for observer)
}

// ResolvePrims populates ResolvedPrims by looking up each string in the
// registry. Called once at link time for each Proto (and its nested Protos).
//
// Also walks the Constants pool: any *eval.PrimVal stub stored as a
// constant (these are emitted by compilePrimOp as the value pushed by
// OpPrimPartial) gets its Impl field populated. This eliminates the
// per-call PrimRegistry.Lookup that the apply paths would otherwise
// incur every time a partial-application PrimVal flows through them.
func (p *Proto) ResolvePrims(reg *eval.PrimRegistry) {
	if len(p.Strings) > 0 {
		p.ResolvedPrims = make([]eval.PrimImpl, len(p.Strings))
		for i, name := range p.Strings {
			if impl, ok := reg.Lookup(name); ok {
				p.ResolvedPrims[i] = impl
			}
		}
	}
	for _, c := range p.Constants {
		if pv, ok := c.(*eval.PrimVal); ok && pv.Impl == nil {
			if impl, ok := reg.Lookup(pv.Name); ok {
				pv.Impl = impl
			}
		}
	}
	for _, child := range p.Protos {
		child.ResolvePrims(reg)
	}
}

// BindInfo records the variable name for an OpBind instruction.
type BindInfo struct {
	Slot int
	Name string
}

// BytecodeMarker satisfies the eval.Bytecode interface.
func (*Proto) BytecodeMarker() {}

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
	ArgSlots []int
	// ArgNames are the variable names for each arg (for observer).
	ArgNames []string
	// ArgGenerated marks which arg bindings are compiler-introduced.
	// When non-nil, ArgGenerated[i] corresponds to ArgNames[i].
	ArgGenerated []bool
	// Labels lists field labels (for record patterns).
	Labels []string
	// FieldSlots maps each field to a local slot (for record patterns).
	FieldSlots []int
}

// RecordDesc describes fields for record construction or update.
type RecordDesc struct {
	Labels []string // field labels in the order values appear on the stack
}

// MergeDesc describes CapEnv label splitting for parallel composition.
type MergeDesc struct {
	LeftLabels  []string // capability labels for left computation
	RightLabels []string // capability labels for right computation
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
