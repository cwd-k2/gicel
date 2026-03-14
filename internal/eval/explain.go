package eval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
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
type ExplainStep struct {
	Depth   int         `json:"depth"`
	Kind    ExplainKind `json:"kind"`
	Message string      `json:"message"`
	Line    int         `json:"line,omitempty"`
	Col     int         `json:"col,omitempty"`
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
	if int(k) < len(explainKindNames) {
		return []byte(`"` + explainKindNames[k] + `"`), nil
	}
	return []byte(`"unknown"`), nil
}

// ExplainHook receives semantic evaluation events.
type ExplainHook func(ExplainStep)

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

// isInternalPattern returns true if the pattern involves compiler-generated
// names (type class dictionaries, elaboration artifacts) — not user-visible.
func isInternalPattern(p core.Pattern) bool {
	switch pat := p.(type) {
	case *core.PCon:
		return strings.Contains(pat.Con, "$")
	case *core.PVar:
		return strings.HasPrefix(pat.Name, "$")
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

// FormatBindings renders match bindings as "a = v1, b = v2".
func FormatBindings(bindings map[string]Value) string {
	if len(bindings) == 0 {
		return ""
	}
	keys := make([]string, 0, len(bindings))
	for k := range bindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " = " + PrettyValue(bindings[k])
	}
	return strings.Join(parts, ", ")
}

// FormatEffect describes an effectful primitive execution with CapEnv diff.
func FormatEffect(name string, args []Value, result Value, oldCap, newCap CapEnv) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		s := PrettyValue(a)
		if strings.Contains(s, " ") {
			s = "(" + s + ")"
		}
		b.WriteByte(' ')
		b.WriteString(s)
	}
	b.WriteString(" → ")
	b.WriteString(PrettyValue(result))

	if diff := capEnvDiff(oldCap, newCap); diff != "" {
		b.WriteString("    ")
		b.WriteString(diff)
	}
	return b.String()
}

// capEnvDiff computes a human-readable description of CapEnv changes.
func capEnvDiff(old, new CapEnv) string {
	// Collect all labels from both.
	seen := make(map[string]bool)
	for _, l := range old.Labels() {
		seen[l] = true
	}
	for _, l := range new.Labels() {
		seen[l] = true
	}

	labels := make([]string, 0, len(seen))
	for l := range seen {
		labels = append(labels, l)
	}
	sort.Strings(labels)

	var diffs []string
	for _, l := range labels {
		ov, oldOK := old.Get(l)
		nv, newOK := new.Get(l)
		if !oldOK && newOK {
			diffs = append(diffs, fmt.Sprintf("[%s: _ → %s]", l, fmtCapVal(nv)))
		} else if oldOK && !newOK {
			diffs = append(diffs, fmt.Sprintf("[%s: removed]", l))
		} else if oldOK && newOK && fmtCapVal(ov) != fmtCapVal(nv) {
			diffs = append(diffs, fmt.Sprintf("[%s: %s → %s]", l, fmtCapVal(ov), fmtCapVal(nv)))
		}
	}
	return strings.Join(diffs, " ")
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
