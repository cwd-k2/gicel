package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/cwd-k2/gicel"
)

func runtimeErrorJSON(err error) map[string]any {
	out := map[string]any{"ok": false, "phase": "eval", "error": err.Error()}
	var re *gicel.RuntimeError
	if errors.As(err, &re) {
		out["message"] = re.Message
		if re.Line > 0 {
			out["line"] = re.Line
			out["col"] = re.Col
		}
	}
	return out
}

func compileErrorJSON(err error) map[string]any {
	out := map[string]any{"ok": false, "phase": "compile", "error": err.Error()}
	var ce *gicel.CompileError
	if errors.As(err, &ce) {
		diags := ce.Diagnostics()
		jdiags := make([]map[string]any, len(diags))
		for i, d := range diags {
			jd := map[string]any{
				"code":    fmt.Sprintf("E%04d", d.Code),
				"phase":   d.Phase,
				"line":    d.Line,
				"col":     d.Col,
				"message": d.Message,
			}
			if len(d.Hints) > 0 {
				jhints := make([]map[string]any, len(d.Hints))
				for j, h := range d.Hints {
					jhints[j] = map[string]any{
						"line":    h.Line,
						"col":     h.Col,
						"message": h.Message,
					}
				}
				jd["hints"] = jhints
			}
			jdiags[i] = jd
		}
		out["diagnostics"] = jdiags
	}
	return out
}

func outputJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing output: %v\n", err)
		os.Exit(1)
	}
}

// summarizeSteps produces a per-section breakdown of explain events.
func summarizeSteps(steps []gicel.ExplainStep) any {
	type sectionSummary struct {
		binds, effects, matches int
		ops                     []string
		result                  string
	}

	var sections []map[string]any
	var current *sectionSummary
	var currentName string
	opSet := map[string]bool{}

	flush := func() {
		if current == nil {
			return
		}
		if current.binds+current.effects+current.matches == 0 && current.result == "" {
			return
		}
		entry := map[string]any{
			"section": currentName,
			"binds":   current.binds,
			"effects": current.effects,
			"matches": current.matches,
			"ops":     current.ops,
		}
		if current.result != "" {
			entry["result"] = current.result
		}
		sections = append(sections, entry)
	}

	for _, s := range steps {
		switch s.Kind {
		case gicel.ExplainLabel:
			if s.Detail.LabelKind == "section" {
				flush()
				currentName = s.Detail.Name
				current = &sectionSummary{}
				opSet = map[string]bool{}
			}
		case gicel.ExplainBind:
			if current != nil {
				current.binds++
			}
		case gicel.ExplainEffect:
			if current != nil {
				current.effects++
				if op := s.Detail.Op; op != "" && !opSet[op] {
					opSet[op] = true
					current.ops = append(current.ops, op)
				}
			}
		case gicel.ExplainMatch:
			if current != nil {
				current.matches++
			}
		case gicel.ExplainResult:
			if current != nil {
				current.result = s.Detail.Value
			}
		}
	}
	flush()
	return sections
}

// formatCapEnv serializes a CapEnv as a map of structured values.
func formatCapEnv(ce gicel.CapEnv) map[string]any {
	labels := ce.Labels()
	if len(labels) == 0 {
		return nil
	}
	m := make(map[string]any, len(labels))
	for _, l := range labels {
		v, _ := ce.Get(l)
		// Omit empty buffer-mode console entries: when Console is in
		// JSON buffer mode but no output was produced, the capability
		// is an empty []string — omit it from the output to avoid
		// exposing transport-internal state.
		if ss, ok := v.([]string); ok && len(ss) == 0 {
			continue
		}
		if val, ok := v.(gicel.Value); ok {
			m[l] = formatValue(val)
		} else {
			m[l] = formatCapEntry(v)
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// formatCapEntry serializes a non-Value capability environment entry as a
// JSON-compatible type. Common Go types are handled explicitly to avoid
// Go-native fmt.Sprintf formatting (e.g., "[line1 line2]" for []string).
func formatCapEntry(v any) any {
	switch val := v.(type) {
	case []string:
		return val
	case string:
		return val
	case int64:
		return val
	case float64:
		return val
	case bool:
		return val
	case nil:
		return nil
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatValue(v gicel.Value) any {
	switch val := v.(type) {
	case *gicel.HostVal:
		// Sanitize non-finite floats: Go's json.Encoder rejects +Inf, -Inf, NaN.
		if f, ok := val.Inner.(float64); ok {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil
			}
		}
		// Data.Slice: recursively format each element.
		if items, ok := val.Inner.([]any); ok {
			result := make([]any, len(items))
			for i, e := range items {
				if ev, ok := e.(gicel.Value); ok {
					result[i] = formatValue(ev)
				} else {
					result[i] = e
				}
			}
			return result
		}
		// Opaque host types (Map, Set, Seq) have unexported fields that
		// json.Marshal serializes as {}. Use their String() if available.
		if s, ok := val.Inner.(fmt.Stringer); ok {
			return s.String()
		}
		return val.Inner
	case *gicel.ConVal:
		if elems, ok := gicel.CollectList(val); ok {
			result := make([]any, len(elems))
			for i, e := range elems {
				result[i] = formatValue(e)
			}
			return result
		}
		if b, ok := gicel.IsBool(val); ok {
			return b
		}
		// Skip leading evidence/dictionary arguments (existential constraints).
		visibleArgs := val.Args[val.DictArgCount:]
		args := make([]any, len(visibleArgs))
		for i, a := range visibleArgs {
			args[i] = formatValue(a)
		}
		return map[string]any{"con": val.Con, "args": args}
	case *gicel.RecordVal:
		if gicel.IsTuple(val) {
			return formatJSONTuple(val)
		}
		return formatJSONRecord(val)
	default:
		return gicel.PrettyValue(v)
	}
}

// formatJSONTuple converts a tuple RecordVal to a JSON array.
func formatJSONTuple(r *gicel.RecordVal) []any {
	n := r.Len()
	elems := make([]any, n)
	for i := range n {
		elems[i] = formatValue(r.MustGet(gicel.TupleLabel(i + 1)))
	}
	return elems
}

// formatJSONRecord converts a RecordVal to a JSON object with recursively formatted fields.
func formatJSONRecord(r *gicel.RecordVal) map[string]any {
	fields := r.RawFields()
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.Label] = formatValue(f.Value)
	}
	return m
}
