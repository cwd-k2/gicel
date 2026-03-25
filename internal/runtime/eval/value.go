package eval

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// Value is a runtime value.
type Value interface {
	valueNode()
	String() string
}

// HostVal wraps an opaque Go value injected from the host.
type HostVal struct {
	Inner any
}

// Closure is a function value capturing its definition environment.
type Closure struct {
	Locals []Value
	Param  string
	Body   ir.Core
	Name   string       // top-level binding name; "" for anonymous lambdas
	Source *span.Source // source where the closure was created
}

// ConVal is a fully-applied constructor value.
type ConVal struct {
	Con  string
	Args []Value
}

// ThunkVal is a suspended computation captured by `thunk`.
type ThunkVal struct {
	Locals []Value
	Comp   ir.Core
	Source *span.Source // source where the thunk was created
}

// PrimVal is a partially or fully applied primitive operation.
// Non-effectful PrimVals are called when len(Args) == Arity.
// Effectful PrimVals (those returning Computation) are deferred until forced in Bind or top-level.
type PrimVal struct {
	Name      string
	Arity     int
	Effectful bool
	Args      []Value
	S         span.Span // source location from the originating PrimOp
}

// RecordVal is a record value { l1: v1, ..., ln: vn }.
type RecordVal struct {
	Fields map[string]Value
}

// IndirectVal is a forward-reference cell for mutually-recursive top-level bindings.
// It holds a pointer to the actual value, which is populated after the binding is evaluated.
type IndirectVal struct {
	Ref *Value
}

// bounceVal is an internal value used by the trampoline TCO mechanism.
// It signals that evaluation should continue with a new (env, capEnv, expr)
// without growing the Go call stack.
//
// leaveDepth records how many ev.budget.Leave() calls the trampoline must
// make before the next evalStep — this unwinds the Enter() that the
// bouncing frame performed (closure application, Force body).
// leaveObs records whether ev.obs.LeaveInternal() is needed (closure
// application of an internal-named function).
// forceSpan, when non-nil, defers a ForceEffectful call to the trampoline.
// This is used by Bind to avoid recursive Eval while still forcing the
// final result (e.g. a bare effectful PrimOp at the end of a do-block).
type bounceVal struct {
	locals     []Value
	capEnv     CapEnv
	expr       ir.Core
	leaveDepth int          // pending ev.budget.Leave() calls
	leaveObs   bool         // pending ev.obs.LeaveInternal()
	source     *span.Source // source context for the continuation (nil = no change)
	forceSpan  *span.Span   // pending ForceEffectful call site (nil = none)
}

func (*HostVal) valueNode()     {}
func (*Closure) valueNode()     {}
func (*ConVal) valueNode()      {}
func (*ThunkVal) valueNode()    {}
func (*PrimVal) valueNode()     {}
func (*RecordVal) valueNode()   {}
func (*IndirectVal) valueNode() {}
func (*bounceVal) valueNode()   {}

func (v *bounceVal) String() string { return "bounceVal(...)" }

func (v *HostVal) String() string {
	return fmt.Sprintf("HostVal(%v)", v.Inner)
}

func (v *Closure) String() string {
	return fmt.Sprintf("Closure(%s, ...)", v.Param)
}

func (v *ConVal) String() string {
	if s, ok := collectListElems(v, Value.String); ok {
		return s
	}
	if len(v.Args) == 0 {
		return v.Con
	}
	args := make([]string, len(v.Args))
	for i, a := range v.Args {
		args[i] = a.String()
	}
	return fmt.Sprintf("(%s %s)", v.Con, strings.Join(args, " "))
}

// List constructor names (Prelude convention).
const (
	ListCons = "Cons"
	ListNil  = "Nil"
)

// CollectList extracts a Cons/Nil chain into a slice of element values.
// Returns (nil, false) if v is not a well-formed list.
func CollectList(v *ConVal) ([]Value, bool) {
	if v.Con != ListCons && v.Con != ListNil {
		return nil, false
	}
	var elems []Value
	cur := Value(v)
	for {
		c, ok := cur.(*ConVal)
		if !ok {
			return nil, false
		}
		if c.Con == ListNil && len(c.Args) == 0 {
			return elems, true
		}
		if c.Con == ListCons && len(c.Args) == 2 {
			elems = append(elems, c.Args[0])
			cur = c.Args[1]
			continue
		}
		return nil, false
	}
}

// collectListElems formats a Cons/Nil chain as [e1, e2, ...],
// using fmtElem to render each element.
// Returns ("", false) if v is not a well-formed list.
func collectListElems(v *ConVal, fmtElem func(Value) string) (string, bool) {
	elems, ok := CollectList(v)
	if !ok {
		return "", false
	}
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = fmtElem(e)
	}
	return "[" + strings.Join(parts, ", ") + "]", true
}

func (v *ThunkVal) String() string {
	return "ThunkVal(...)"
}

func (v *PrimVal) String() string {
	if len(v.Args) == 0 {
		return fmt.Sprintf("PrimVal(%s)", v.Name)
	}
	args := make([]string, len(v.Args))
	for i, a := range v.Args {
		args[i] = a.String()
	}
	return fmt.Sprintf("PrimVal(%s %s)", v.Name, strings.Join(args, " "))
}

func (v *RecordVal) String() string {
	if len(v.Fields) == 0 {
		return "()"
	}
	keys := make([]string, 0, len(v.Fields))
	for k := range v.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s = %s", k, v.Fields[k])
	}
	return fmt.Sprintf("{ %s }", strings.Join(parts, ", "))
}

func (v *IndirectVal) String() string {
	if v.Ref == nil {
		return "IndirectVal(<uninitialized>)"
	}
	return (*v.Ref).String()
}
