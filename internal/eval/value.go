package eval

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/span"
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
	Env   *Env
	Param string
	Body  core.Core
	Name  string // top-level binding name; "" for anonymous lambdas
}

// ConVal is a fully-applied constructor value.
type ConVal struct {
	Con  string
	Args []Value
}

// ThunkVal is a suspended computation captured by `thunk`.
type ThunkVal struct {
	Env  *Env
	Comp core.Core
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
// leaveDepth records how many ev.limit.Leave() calls the trampoline must
// make before the next evalStep — this unwinds the Enter() that the
// bouncing frame performed (closure application, LetRec body, Force body).
// leaveObs records whether ev.obs.LeaveInternal() is needed (closure
// application of an internal-named function).
type bounceVal struct {
	env        *Env
	capEnv     CapEnv
	expr       core.Core
	leaveDepth int  // pending ev.limit.Leave() calls
	leaveObs   bool // pending ev.obs.LeaveInternal()
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
	if len(v.Args) == 0 {
		return v.Con
	}
	args := make([]string, len(v.Args))
	for i, a := range v.Args {
		args[i] = a.String()
	}
	return fmt.Sprintf("(%s %s)", v.Con, strings.Join(args, " "))
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
	parts := make([]string, 0, len(v.Fields))
	for k, val := range v.Fields {
		parts = append(parts, fmt.Sprintf("%s = %s", k, val))
	}
	return fmt.Sprintf("{ %s }", strings.Join(parts, ", "))
}

func (v *IndirectVal) String() string {
	if v.Ref == nil {
		return "IndirectVal(<uninitialized>)"
	}
	return (*v.Ref).String()
}
