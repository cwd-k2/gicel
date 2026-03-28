package eval

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// ExplainKind classifies semantic evaluation events.
type ExplainKind int

const (
	ExplainBind   ExplainKind = iota // do-block: x ← value; or let: y = value
	ExplainMatch                     // case: scrutinee matched pattern
	ExplainEffect                    // effectful primitive executed
	ExplainLabel                     // section: which top-level binding is being evaluated
	ExplainResult                    // entry point yielded final value
)

// ExplainStep is a single semantic event during evaluation.
// Detail carries all event data; consumers format as needed.
type ExplainStep struct {
	Seq        int           `json:"seq"`
	Depth      int           `json:"depth"`
	Kind       ExplainKind   `json:"kind"`
	SourceName string        `json:"source,omitempty"`
	Line       int           `json:"line,omitempty"`
	Col        int           `json:"col,omitempty"`
	Detail     ExplainDetail `json:"detail,omitempty"`
}

// ExplainDetail carries kind-specific structured data.
// Exactly one group of fields is populated, corresponding to the step's Kind.
type ExplainDetail struct {
	// Label/Result: section name or entry point.
	Name string `json:"name,omitempty"`
	// Label: "enter" for function call, "section" for top-level binding.
	LabelKind string `json:"labelKind,omitempty"`

	// Bind: variable name and value.
	Var     string `json:"var,omitempty"`
	Value   string `json:"value,omitempty"`
	Monadic bool   `json:"monadic,omitempty"` // true for ← (do-bind), false for = (let)

	// Effect: primitive operation details.
	Op      string               `json:"op,omitempty"`
	Args    []string             `json:"args,omitempty"`
	Result  string               `json:"result,omitempty"`
	CapDiff map[string][2]string `json:"capDiff,omitempty"` // label → [old, new]

	// Match: pattern matching details.
	Scrutinee string            `json:"scrutinee,omitempty"`
	Pattern   string            `json:"pattern,omitempty"`
	Bindings  map[string]string `json:"bindings,omitempty"`
}

var explainKindNames = [...]string{
	ExplainBind:   "bind",
	ExplainMatch:  "match",
	ExplainEffect: "effect",
	ExplainLabel:  "label",
	ExplainResult: "result",
}

// MarshalJSON encodes ExplainKind as a string.
func (k ExplainKind) MarshalJSON() ([]byte, error) {
	if k >= 0 && int(k) < len(explainKindNames) {
		return []byte(`"` + explainKindNames[k] + `"`), nil
	}
	return []byte(`"unknown"`), nil
}

// ExplainHook receives semantic evaluation events.
type ExplainHook func(ExplainStep)

// ExplainObserver instruments evaluation with semantic trace events.
// A nil *ExplainObserver is safe to call — all methods are no-ops.
type ExplainObserver struct {
	hook      ExplainHook
	source    *span.Source
	seq       int
	suppress  int
	all       bool            // ignore suppression (ExplainAll mode)
	internals map[string]bool // stdlib/module-internal closure names
}

// NewExplainObserver creates an observer with the given hook and source.
func NewExplainObserver(hook ExplainHook, source *span.Source) *ExplainObserver {
	return &ExplainObserver{hook: hook, source: source, internals: make(map[string]bool)}
}

// Active reports whether trace events should be emitted.
// A nil observer is never active.
func (o *ExplainObserver) Active() bool {
	return o != nil && (o.suppress == 0 || o.all)
}

// Emit sends a trace event through the hook, assigning seq and resolving location.
// Suppressed events are silently dropped.
func (o *ExplainObserver) Emit(depth int, kind ExplainKind, detail ExplainDetail, s span.Span) {
	if !o.Active() {
		return
	}
	o.seq++
	step := ExplainStep{Seq: o.seq, Depth: depth, Kind: kind, Detail: detail}
	if o.source != nil && s.Start > 0 && int(s.Start) < len(o.source.Text) {
		step.SourceName = o.source.Name
		step.Line, step.Col = o.source.Location(s.Start)
	}
	o.hook(step)
}

// Section emits a section label (not subject to suppression).
// Safe to call on a nil observer.
func (o *ExplainObserver) Section(name string) {
	if o == nil {
		return
	}
	o.seq++
	o.hook(ExplainStep{Seq: o.seq, Kind: ExplainLabel, Detail: labelDetail(name, "section")})
}

// Result emits the final result (not subject to suppression).
// Safe to call on a nil observer.
func (o *ExplainObserver) Result(value string) {
	if o == nil {
		return
	}
	o.seq++
	o.hook(ExplainStep{Seq: o.seq, Kind: ExplainResult, Detail: resultDetail(value)})
}

// MarkInternal registers a closure name as stdlib-internal.
// Safe to call on a nil observer.
func (o *ExplainObserver) MarkInternal(name string) {
	if o != nil {
		o.internals[name] = true
	}
}

// IsInternal reports whether the named closure is stdlib-internal.
// Safe to call on a nil observer.
func (o *ExplainObserver) IsInternal(name string) bool {
	return o != nil && o.internals[name]
}

// EnterInternal increments the suppression counter.
// Safe to call on a nil observer.
func (o *ExplainObserver) EnterInternal() {
	if o == nil {
		return
	}
	o.suppress++
}

// LeaveInternal decrements the suppression counter.
// Safe to call on a nil observer.
func (o *ExplainObserver) LeaveInternal() {
	if o == nil {
		return
	}
	o.suppress--
}

// SetSource updates the current source context.
// Called by the evaluator when crossing module boundaries.
// Safe to call on a nil observer.
func (o *ExplainObserver) SetSource(src *span.Source) {
	if o != nil {
		o.source = src
	}
}

// SetAll disables suppression (ExplainAll mode).
func (o *ExplainObserver) SetAll(v bool) { o.all = v }

// maxPrettyDepth is the maximum recursion depth for PrettyValue.
const maxPrettyDepth = 256

// PrettyValue formats a runtime value in source-level terms.
// No "HostVal(...)", no "{ _1: ..., _2: ... }" — uses tuples and bare values.
func PrettyValue(v Value) string {
	return prettyValueDepth(v, 0)
}

func prettyValueDepth(v Value, depth int) string {
	if depth > maxPrettyDepth {
		return "..."
	}
	switch val := v.(type) {
	case *HostVal:
		return prettyHost(val.Inner)
	case *ConVal:
		if s, ok := collectListElems(val, func(v Value) string { return prettyValueDepth(v, depth+1) }); ok {
			return s
		}
		if len(val.Args) == 0 {
			return val.Con
		}
		args := make([]string, len(val.Args))
		for i, a := range val.Args {
			s := prettyValueDepth(a, depth+1)
			// Parenthesize constructor arguments that contain spaces.
			if strings.Contains(s, " ") {
				s = "(" + s + ")"
			}
			args[i] = s
		}
		return val.Con + " " + strings.Join(args, " ")
	case *RecordVal:
		if IsTuple(val) {
			return prettyTupleDepth(val, depth)
		}
		return prettyRecordDepth(val, depth)
	case *Closure:
		return "<function>"
	case *VMClosure:
		return "<function>"
	case *ThunkVal:
		return "<thunk>"
	case *VMThunkVal:
		return "<thunk>"
	case *PrimVal:
		return "<function>"
	case *IndirectVal:
		if val.Ref == nil {
			return "<uninitialized>"
		}
		return prettyValueDepth(*val.Ref, depth+1)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func prettyHost(v any) string {
	if v == nil {
		return "()"
	}
	switch val := v.(type) {
	case string:
		return strconv.Quote(val)
	case rune:
		return strconv.QuoteRune(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// IsTuple checks if a RecordVal is tuple sugar (fields _1, _2, ..., _n).
func IsTuple(r *RecordVal) bool {
	n := r.Len()
	if n == 0 {
		return true // () is the unit tuple
	}
	for i := 1; i <= n; i++ {
		if _, ok := r.Get(ir.TupleLabel(i)); !ok {
			return false
		}
	}
	return true
}

func prettyTupleDepth(r *RecordVal, depth int) string {
	n := r.Len()
	if n == 0 {
		return "()"
	}
	parts := make([]string, n)
	for i := range n {
		parts[i] = prettyValueDepth(r.MustGet(ir.TupleLabel(i+1)), depth+1)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func prettyRecordDepth(r *RecordVal, depth int) string {
	fields := r.RawFields()
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = f.Label + ": " + prettyValueDepth(f.Value, depth+1)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// EffectDetail constructs an ExplainDetail for an effect event, including CapEnv diff.
func EffectDetail(name string, args []Value, result Value, oldCap, newCap CapEnv) ExplainDetail {
	return effectDetail(name, args, result, oldCap, newCap)
}

func effectDetail(name string, args []Value, result Value, oldCap, newCap CapEnv) ExplainDetail {
	d := ExplainDetail{
		Op:     name,
		Result: PrettyValue(result),
	}
	if len(args) > 0 {
		d.Args = make([]string, len(args))
		for i, a := range args {
			d.Args[i] = PrettyValue(a)
		}
	}
	d.CapDiff = capEnvDiffStructured(oldCap, newCap)
	return d
}

func labelDetail(name, labelKind string) ExplainDetail {
	return ExplainDetail{Name: name, LabelKind: labelKind}
}

func resultDetail(value string) ExplainDetail {
	return ExplainDetail{Value: value}
}

// capEnvDiffStructured computes structured CapEnv changes as [old, new] pairs.
func capEnvDiffStructured(old, new CapEnv) map[string][2]string {
	oldLabels := old.Labels()
	newLabels := new.Labels()
	if len(oldLabels) == 0 && len(newLabels) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	for _, l := range oldLabels {
		seen[l] = true
	}
	for _, l := range newLabels {
		seen[l] = true
	}

	var diffs map[string][2]string
	for l := range seen {
		ov, oldOK := old.Get(l)
		nv, newOK := new.Get(l)
		oldStr := ""
		newStr := ""
		if oldOK {
			oldStr = fmtCapVal(ov)
		}
		if newOK {
			newStr = fmtCapVal(nv)
		}
		if oldStr != newStr {
			if diffs == nil {
				diffs = make(map[string][2]string)
			}
			diffs[l] = [2]string{oldStr, newStr}
		}
	}
	return diffs
}

func fmtCapVal(v any) string {
	if v == nil {
		return "_"
	}
	if val, ok := v.(Value); ok {
		return PrettyValue(val)
	}
	return fmt.Sprintf("%v", v)
}
