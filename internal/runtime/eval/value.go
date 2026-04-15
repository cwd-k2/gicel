package eval

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// cmpRecordField compares RecordFields by label for deterministic ordering.
func cmpRecordField(a, b RecordField) int {
	return cmp.Compare(a.Label, b.Label)
}

// Value is a runtime value.
type Value interface {
	valueNode()
	String() string
}

// HostVal wraps an opaque Go value injected from the host.
type HostVal struct {
	Inner any
}

// Closure is a test-only function placeholder. Tests use it to inject a
// Value-implementing stub that can be discriminated via type switch; the
// Value interface's unexported valueNode() method prevents external test
// packages from defining such a stub themselves. Production code never
// creates or consumes Closure — the bytecode VM uses VMClosure.
type Closure struct {
	Param string
}

// ConVal is a fully-applied constructor value.
// Con is the bare constructor name (no module qualification). This is safe
// because the type checker guarantees that all constructor references are
// resolved to their canonical names and that no two constructors with the
// same bare name from different modules can appear in the same scope.
// See also B-3 (caseOfKnownCtor module check, now fixed).
type ConVal struct {
	Con          string
	Args         []Value
	IsDict       bool // true for type class dictionary constructors (structural, not name-derived)
	DictArgCount int  // number of leading evidence/dictionary arguments (hidden from display)
}


// PrimVal is a partially or fully applied primitive operation.
// Non-effectful PrimVals are called when len(Args) == Arity.
// Effectful PrimVals (those returning Computation) are deferred until forced in Bind or top-level.
//
// Impl is an optional cached pointer to the resolved PrimImpl. When non-nil
// the apply paths skip the per-call PrimRegistry.Lookup(Name) hash lookup.
// Populated at link time by Proto.ResolvePrims for stub PrimVals living in
// the Constants pool, and propagated to derived PrimVals (partial / over-
// app intermediates) by every apply site that constructs a fresh PrimVal.
type PrimVal struct {
	Name        string
	Arity       int
	IsEffectful bool
	Args        []Value
	S           span.Span // source location from the originating PrimOp
	Impl        PrimImpl  // resolved impl, or nil to fall back to registry lookup
}

// VariantVal is a variant (labeled coproduct) value — one tag selected from a row.
type VariantVal struct {
	Tag   string // selected label
	Value Value  // payload value
}

// UnitVal is the unit value () — an empty record.
var UnitVal Value = &RecordVal{}

// RecordField is a single field in a record value.
type RecordField struct {
	Label string
	Value Value
}

// RecordVal is a record value { l1: v1, ..., ln: vn }.
// Fields are stored as a label-sorted slice. Sorted order is enforced
// by NewRecord/NewRecordFromMap/Update constructors, enabling O(log n)
// binary search in Get.
type RecordVal struct {
	fields []RecordField
}

// Get returns the value for the given label, or (nil, false).
// Uses binary search on the sorted field slice.
func (r *RecordVal) Get(label string) (Value, bool) {
	lo, hi := 0, len(r.fields)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if r.fields[mid].Label < label {
			lo = mid + 1
		} else if r.fields[mid].Label > label {
			hi = mid
		} else {
			return r.fields[mid].Value, true
		}
	}
	return nil, false
}

// MustGet returns the value for the given label, panicking if absent.
func (r *RecordVal) MustGet(label string) Value {
	v, ok := r.Get(label)
	if !ok {
		panic("RecordVal.MustGet: missing label " + label)
	}
	return v
}

// Len returns the number of fields.
func (r *RecordVal) Len() int { return len(r.fields) }

// RawFields returns the underlying sorted field slice.
func (r *RecordVal) RawFields() []RecordField { return r.fields }

// Fields returns a map view for backward compatibility.
// Allocates a new map on each call — prefer Get/RawFields in new code.
func (r *RecordVal) AsMap() map[string]Value {
	m := make(map[string]Value, len(r.fields))
	for _, f := range r.fields {
		m[f.Label] = f.Value
	}
	return m
}

// NewRecord creates a RecordVal from fields, sorting by label.
func NewRecord(fields []RecordField) *RecordVal {
	slices.SortFunc(fields, cmpRecordField)
	return &RecordVal{fields: fields}
}

// NewRecordFromMap creates a RecordVal from a map (sorts labels).
func NewRecordFromMap(m map[string]Value) *RecordVal {
	fields := make([]RecordField, 0, len(m))
	for k, v := range m {
		fields = append(fields, RecordField{Label: k, Value: v})
	}
	slices.SortFunc(fields, cmpRecordField)
	return &RecordVal{fields: fields}
}

// RecordUpdate returns a new RecordVal with updated fields.
// Updates are applied on top of the existing fields.
func (r *RecordVal) Update(updates []RecordField) *RecordVal {
	// Build update map for O(1) lookup.
	umap := make(map[string]Value, len(updates))
	for _, u := range updates {
		umap[u.Label] = u.Value
	}
	// Merge: keep existing fields (possibly overwritten) + new fields.
	result := make([]RecordField, 0, len(r.fields)+len(updates))
	for _, f := range r.fields {
		if v, ok := umap[f.Label]; ok {
			result = append(result, RecordField{Label: f.Label, Value: v})
			delete(umap, f.Label)
		} else {
			result = append(result, f)
		}
	}
	// Remaining updates are new fields — insert in sorted position.
	for k, v := range umap {
		result = append(result, RecordField{Label: k, Value: v})
	}
	slices.SortFunc(result, cmpRecordField)
	return &RecordVal{fields: result}
}

// Bytecode is the interface satisfied by compiled bytecode prototypes.
//
// Defined in the eval package to break a circular import between eval
// and vm. The sole implementor is *vm.Proto, created exclusively within
// the vm package. All type assertions from Bytecode to *Proto are routed
// through vm.protoOf() — a single-point cast that consolidates the
// contract and simplifies future substitution.
//
// This is a marker interface by design: adding behavioral methods would
// re-introduce the circular dependency. The safety of the assertion is
// guaranteed by the vm package's exclusive ownership of Proto creation.
type Bytecode interface {
	BytecodeMarker()
}

// VMClosure is a function value in the bytecode VM.
type VMClosure struct {
	Captured []Value
	Proto    Bytecode     // *vm.Proto (satisfies eval.Bytecode)
	Name     string       // top-level binding name; "" for anonymous lambdas
	Source   *span.Source // source where the closure was created
}

// VMThunkVal is a suspended computation in the bytecode VM.
type VMThunkVal struct {
	Captured  []Value
	Proto     Bytecode     // *vm.Proto (satisfies eval.Bytecode)
	Source    *span.Source // source where the thunk was created
	IsAutoForce bool       // true for rec self-referential thunks
}

// PAPVal is a partial application: a multi-parameter closure that has received
// fewer arguments than its arity. Created when applying a single argument to a
// closure whose Proto has len(Params) > 1, or by adding an argument to an
// existing PAPVal that is not yet saturated.
type PAPVal struct {
	Fun   *VMClosure // original closure (carries Proto with full arity)
	Args  []Value    // arguments applied so far
	Arity int        // total arity (cached from Proto)
}

// IndirectVal is a forward-reference cell for mutually-recursive top-level bindings.
// It holds a pointer to the actual value, which is populated after the binding is evaluated.
type IndirectVal struct {
	Ref *Value
}

func (*HostVal) valueNode()     {}
func (*Closure) valueNode()     {}
func (*ConVal) valueNode()      {}
func (*PrimVal) valueNode()     {}
func (*RecordVal) valueNode()   {}
func (*VariantVal) valueNode()  {}
func (*VMClosure) valueNode()   {}
func (*VMThunkVal) valueNode()  {}
func (*PAPVal) valueNode()      {}
func (*IndirectVal) valueNode() {}

func (v *HostVal) String() string {
	return fmt.Sprintf("HostVal(%v)", v.Inner)
}

func (v *Closure) String() string {
	return "Closure(" + v.Param + ", ...)"
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
	return "(" + v.Con + " " + strings.Join(args, " ") + ")"
}

// Prelude constructor names.
const (
	ListCons  = "Cons"
	ListNil   = "Nil"
	BoolTrue  = "True"
	BoolFalse = "False"

	MaybeJust    = "Just"
	MaybeNothing = "Nothing"
	ResultOk     = "Ok"
	ResultErr    = "Err"
	OrderLT      = "LT"
	OrderEQ      = "EQ"
	OrderGT      = "GT"
)

// Interned values — immutable singleton instances.
// Structurally equal immutable values may share representation.
var (
	TrueVal  Value = &ConVal{Con: BoolTrue}
	FalseVal Value = &ConVal{Con: BoolFalse}
	LTVal    Value = &ConVal{Con: OrderLT}
	EQVal    Value = &ConVal{Con: OrderEQ}
	GTVal    Value = &ConVal{Con: OrderGT}
)

// BoolVal returns the interned Bool value for b.
func BoolVal(b bool) Value {
	if b {
		return TrueVal
	}
	return FalseVal
}

// smallInts caches HostVal for integers -128..127.
var smallInts [256]*HostVal

func init() {
	for i := range smallInts {
		n := int64(i) - 128
		smallInts[i] = &HostVal{Inner: n}
	}
}

// IntVal returns a HostVal for n, using a cached instance for small values.
func IntVal(n int64) Value {
	if n >= -128 && n <= 127 {
		return smallInts[n+128]
	}
	return &HostVal{Inner: n}
}

// IsBool checks if a ConVal is a Prelude Bool (True or False, nullary).
func IsBool(v *ConVal) (val bool, ok bool) {
	if len(v.Args) != 0 {
		return false, false
	}
	switch v.Con {
	case BoolTrue:
		return true, true
	case BoolFalse:
		return false, true
	default:
		return false, false
	}
}

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

func (v *PrimVal) String() string {
	return "<function>"
}

func (v *RecordVal) String() string {
	if len(v.fields) == 0 {
		return "()"
	}
	// Fields are already sorted, so no need to sort keys.
	parts := make([]string, len(v.fields))
	for i, f := range v.fields {
		parts[i] = f.Label + " = " + f.Value.String()
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func (v *VariantVal) String() string {
	// Tag already carries the "#" prefix from label erasure (e.g. "#a").
	return "@" + v.Tag + " " + v.Value.String()
}

func (v *VMClosure) String() string {
	return "VMClosure(" + v.Name + ", ...)"
}

func (v *PAPVal) String() string {
	return fmt.Sprintf("PAPVal(%s, %d/%d)", v.Fun.Name, len(v.Args), v.Arity)
}

func (v *VMThunkVal) String() string {
	return "VMThunkVal(...)"
}

func (v *IndirectVal) String() string {
	if v.Ref == nil {
		return "IndirectVal(<uninitialized>)"
	}
	return (*v.Ref).String()
}
