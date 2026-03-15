package eval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/span"
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
	Seq    int           `json:"seq"`
	Depth  int           `json:"depth"`
	Kind   ExplainKind   `json:"kind"`
	Line   int           `json:"line,omitempty"`
	Col    int           `json:"col,omitempty"`
	Detail ExplainDetail `json:"detail,omitempty"`
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
		step.Line, step.Col = o.source.Location(s.Start)
	}
	o.hook(step)
}

// Section emits a section label (not subject to suppression).
func (o *ExplainObserver) Section(name string) {
	o.seq++
	o.hook(ExplainStep{Seq: o.seq, Kind: ExplainLabel, Detail: LabelDetail(name, "section")})
}

// Result emits the final result (not subject to suppression).
func (o *ExplainObserver) Result(value string) {
	o.seq++
	o.hook(ExplainStep{Seq: o.seq, Kind: ExplainResult, Detail: ResultDetail(value)})
}

// MarkInternal registers a closure name as stdlib-internal.
// Safe to call on a nil observer.
func (o *ExplainObserver) MarkInternal(name string) {
	if o != nil {
		o.internals[name] = true
	}
}

// IsInternal reports whether the named closure is stdlib-internal.
func (o *ExplainObserver) IsInternal(name string) bool {
	return o != nil && o.internals[name]
}

// EnterInternal increments the suppression counter.
func (o *ExplainObserver) EnterInternal() { o.suppress++ }

// LeaveInternal decrements the suppression counter.
func (o *ExplainObserver) LeaveInternal() { o.suppress-- }

// SetAll disables suppression (ExplainAll mode).
func (o *ExplainObserver) SetAll(v bool) { o.all = v }

// PrettyValue formats a runtime value in source-level terms.
// No "HostVal(...)", no "{ _1 = ..., _2 = ... }" — uses tuples and bare values.
func PrettyValue(v Value) string {
	switch val := v.(type) {
	case *HostVal:
		return prettyHost(val.Inner)
	case *ConVal:
		if len(val.Args) == 0 {
			return val.Con
		}
		args := make([]string, len(val.Args))
		for i, a := range val.Args {
			s := PrettyValue(a)
			// Parenthesize constructor arguments that contain spaces.
			if strings.Contains(s, " ") {
				s = "(" + s + ")"
			}
			args[i] = s
		}
		return val.Con + " " + strings.Join(args, " ")
	case *RecordVal:
		if isTuple(val) {
			return prettyTuple(val)
		}
		return prettyRecord(val)
	case *Closure:
		return "<function>"
	case *ThunkVal:
		return "<thunk>"
	case *PrimVal:
		return "<primitive:" + val.Name + ">"
	case *IndirectVal:
		if val.Ref == nil {
			return "<uninitialized>"
		}
		return PrettyValue(*val.Ref)
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

// isTuple checks if a RecordVal is tuple sugar (fields _1, _2, ..., _n).
func isTuple(r *RecordVal) bool {
	n := len(r.Fields)
	if n == 0 {
		return true // () is the unit tuple
	}
	for i := 1; i <= n; i++ {
		if _, ok := r.Fields["_"+strconv.Itoa(i)]; !ok {
			return false
		}
	}
	return true
}

func prettyTuple(r *RecordVal) string {
	n := len(r.Fields)
	if n == 0 {
		return "()"
	}
	parts := make([]string, n)
	for i := range n {
		parts[i] = PrettyValue(r.Fields["_"+strconv.Itoa(i+1)])
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func prettyRecord(r *RecordVal) string {
	keys := make([]string, 0, len(r.Fields))
	for k := range r.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " = " + PrettyValue(r.Fields[k])
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// FormatPattern renders a Core pattern in source-level terms.
func FormatPattern(p core.Pattern) string {
	switch pat := p.(type) {
	case *core.PVar:
		return pat.Name
	case *core.PWild:
		return "_"
	case *core.PCon:
		if len(pat.Args) == 0 {
			return pat.Con
		}
		args := make([]string, len(pat.Args))
		for i, a := range pat.Args {
			s := FormatPattern(a)
			if strings.Contains(s, " ") {
				s = "(" + s + ")"
			}
			args[i] = s
		}
		return pat.Con + " " + strings.Join(args, " ")
	case *core.PRecord:
		if len(pat.Fields) == 0 {
			return "()"
		}
		// Check for tuple pattern.
		if isTuplePattern(pat) {
			parts := make([]string, len(pat.Fields))
			for i, f := range pat.Fields {
				parts[i] = FormatPattern(f.Pattern)
			}
			return "(" + strings.Join(parts, ", ") + ")"
		}
		parts := make([]string, len(pat.Fields))
		for i, f := range pat.Fields {
			parts[i] = f.Label + " = " + FormatPattern(f.Pattern)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	}
	return "?"
}

// isInternalPattern reports whether a pattern involves compiler-generated names.
func isInternalPattern(p core.Pattern) bool {
	switch pat := p.(type) {
	case *core.PCon:
		return isCompilerGenerated(pat.Con)
	case *core.PVar:
		return isCompilerGenerated(pat.Name)
	}
	return false
}

func isTuplePattern(p *core.PRecord) bool {
	for i, f := range p.Fields {
		if f.Label != "_"+strconv.Itoa(i+1) {
			return false
		}
	}
	return true
}

// BindDetail builds an ExplainDetail for a bind event.
func BindDetail(varName, value string, monadic bool) ExplainDetail {
	return ExplainDetail{Var: varName, Value: value, Monadic: monadic}
}

// MatchDetail builds an ExplainDetail for a match event.
func MatchDetail(scrutinee, pattern string, bindings map[string]Value) ExplainDetail {
	d := ExplainDetail{Scrutinee: scrutinee, Pattern: pattern}
	if len(bindings) > 0 {
		d.Bindings = make(map[string]string, len(bindings))
		for k, v := range bindings {
			d.Bindings[k] = PrettyValue(v)
		}
	}
	return d
}

// EffectDetail builds an ExplainDetail for an effect event.
func EffectDetail(name string, args []Value, result Value, oldCap, newCap CapEnv) ExplainDetail {
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

// LabelDetail builds an ExplainDetail for a label event.
func LabelDetail(name, labelKind string) ExplainDetail {
	return ExplainDetail{Name: name, LabelKind: labelKind}
}

// ResultDetail builds an ExplainDetail for a result event.
func ResultDetail(value string) ExplainDetail {
	return ExplainDetail{Name: "result", Value: value}
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
