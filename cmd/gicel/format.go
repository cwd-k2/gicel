package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cwd-k2/gicel"
)

// ANSI escape sequences.
const (
	cReset    = "\033[0m"
	cBold     = "\033[1m"
	cDim      = "\033[2m"
	cCyan     = "\033[36m"
	cBoldCyan = "\033[1;36m"
	cYellow   = "\033[33m"
	cGreen    = "\033[32m"
)

// Layout constants for column alignment.
//
// Line format:  " %2d  :%3d  %-6s │ %s"
// Positions:     0 12  3456  789...
//
//	  ↑         ↑
//	depth      kind
//
// The separator │ sits at byte offset 18; content starts at 20.
const (
	fmtPrefixLen = 20 // total width of fixed columns (up to and including "│ ")
	fmtRuleWidth = 56 // target total width for horizontal rules
)

// explainFormatter renders ExplainStep events as structured terminal output.
//
// Section labels (top-level bindings) are buffered: a label is only emitted
// when followed by at least one child event, suppressing noise from pure
// value bindings that produce no trace.
type explainFormatter struct {
	w            io.Writer
	color        bool
	verbose      bool
	sourceLines  []string
	pendingLabel *gicel.ExplainStep
	hadEvent     bool // child event seen since last section header
	prevIsEnter  bool // previous event was an enter (suppress separator)
}

func newExplainFormatter(w io.Writer, color, verbose bool, source string) *explainFormatter {
	f := &explainFormatter{w: w, color: color, verbose: verbose}
	if verbose {
		f.sourceLines = strings.Split(source, "\n")
	}
	return f
}

// useColor decides whether to enable color based on flag, env, and terminal.
func useColor(noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Emit processes a single ExplainStep using structured Detail data.
func (f *explainFormatter) Emit(step gicel.ExplainStep) {
	if step.Kind == gicel.ExplainResult {
		return
	}

	// Buffer section labels; discard empty ones.
	if step.Kind == gicel.ExplainLabel && step.Detail.LabelKind == "section" {
		f.flushPending(false)
		cp := step
		f.pendingLabel = &cp
		return
	}

	f.flushPending(true)

	switch step.Kind {
	case gicel.ExplainLabel:
		f.writeEnter(step)
		f.prevIsEnter = true
		return
	case gicel.ExplainBind:
		f.writeBind(step)
	case gicel.ExplainEffect:
		f.writeEffect(step)
	case gicel.ExplainMatch:
		f.writeMatch(step)
	}
	f.prevIsEnter = false
}

// Flush emits any remaining pending label (typically the entry point).
func (f *explainFormatter) Flush() {
	f.flushPending(true)
}

func (f *explainFormatter) flushPending(show bool) {
	if f.pendingLabel == nil {
		return
	}
	if show {
		f.writeSection(f.pendingLabel.Detail.Name)
	}
	f.pendingLabel = nil
	f.hadEvent = false
}

// ── section ─────────────────────────────────────
func (f *explainFormatter) writeSection(name string) {
	ruleLen := max(3, fmtRuleWidth-4-len(name))
	line := "── " + name + " " + strings.Repeat("─", ruleLen)
	if f.color {
		line = cBold + line + cReset
	}
	fmt.Fprintf(f.w, "\n%s\n", line)
}

// 0  :52  enter  │ name ───────────────────────
func (f *explainFormatter) writeEnter(step gicel.ExplainStep) {
	name := step.Detail.Name
	if step.Detail.Value != "" {
		argStr := step.Detail.Value
		if len(argStr) > 30 {
			argStr = argStr[:27] + "..."
		}
		name += "(" + argStr + ")"
	}

	if f.hadEvent && !f.prevIsEnter {
		f.writeSep()
	}
	f.hadEvent = true

	ruleLen := max(3, fmtRuleWidth-fmtPrefixLen-len(name)-1)
	var content string
	if f.color {
		content = cBoldCyan + name + cReset + " " + cDim + strings.Repeat("─", ruleLen) + cReset
	} else {
		content = name + " " + strings.Repeat("─", ruleLen)
	}

	f.writeLine(step.Depth, step.Line, "enter", cBoldCyan, content)

	if f.verbose && step.Line > 0 && step.Line <= len(f.sourceLines) {
		src := strings.TrimSpace(f.sourceLines[step.Line-1])
		if src != "" {
			pad := strings.Repeat(" ", fmtPrefixLen-2) + "│   "
			fmt.Fprintf(f.w, "%s\n", f.styled(cDim, pad+src))
		}
	}
}

// 1  :25  bind   │ price ← 50
func (f *explainFormatter) writeBind(step gicel.ExplainStep) {
	f.hadEvent = true
	d := step.Detail
	if d.Var == "" {
		f.writeRaw(step)
		return
	}
	op := "="
	if d.Monadic {
		op = "←"
	}
	content := f.styled(cCyan, d.Var+" "+op+" "+d.Value)
	f.writeLine(step.Depth, step.Line, "bind", cCyan, content)
}

// 1  :52  effect │ put 50
// 1  :25  effect │ get ⇒ 50
func (f *explainFormatter) writeEffect(step gicel.ExplainStep) {
	f.hadEvent = true
	d := step.Detail
	s := d.Op
	for _, a := range d.Args {
		if strings.Contains(a, " ") {
			s += " (" + a + ")"
		} else {
			s += " " + a
		}
	}
	if d.Result != "" && d.Result != "()" {
		s += " ⇒ " + d.Result
	}
	content := f.styled(cYellow, s)
	if len(d.CapDiff) > 0 {
		content += "  " + f.styled(cDim, fmtCapDiff(d.CapDiff))
	}
	f.writeLine(step.Depth, step.Line, "effect", cYellow, content)
}

// 1  :26  match  │ Circle 5 ▸ Circle r  r = 5
func (f *explainFormatter) writeMatch(step gicel.ExplainStep) {
	f.hadEvent = true
	d := step.Detail
	s := d.Scrutinee + " ▸ " + d.Pattern
	content := f.styled(cGreen, s)
	if len(d.Bindings) > 0 {
		content += "  " + f.styled(cDim, fmtBindings(d.Bindings))
	}
	f.writeLine(step.Depth, step.Line, "match", cGreen, content)
}

// writeLine emits one trace line with aligned fixed columns.
//
//	" %2d  :%3d  %-6s │ %s"
func (f *explainFormatter) writeLine(depth, line int, kind, kindColor, content string) {
	d := fmt.Sprintf("%2d", depth)
	var l string
	if line > 0 {
		l = fmt.Sprintf(":%3d", line)
	} else {
		l = "    "
	}
	k := fmt.Sprintf("%-6s", kind)
	sep := "│"

	if f.color {
		d = cDim + d + cReset
		l = cDim + l + cReset
		k = kindColor + k + cReset
		sep = cDim + sep + cReset
	}

	fmt.Fprintf(f.w, " %s  %s  %s %s %s\n", d, l, k, sep, content)
}

// writeSep emits a blank separator with only the │ marker.
func (f *explainFormatter) writeSep() {
	s := strings.Repeat(" ", fmtPrefixLen-2) + "│"
	fmt.Fprintln(f.w, f.styled(cDim, s))
}

func (f *explainFormatter) writeRaw(step gicel.ExplainStep) {
	fmt.Fprintf(f.w, "[kind=%d depth=%d]\n", step.Kind, step.Depth)
}

// styled wraps s in ANSI color codes when color is enabled.
func (f *explainFormatter) styled(code, s string) string {
	if f.color {
		return code + s + cReset
	}
	return s
}

// fmtCapDiff renders structured capability environment changes.
func fmtCapDiff(diff map[string][2]string) string {
	keys := make([]string, 0, len(diff))
	for k := range diff {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		pair := diff[k]
		switch {
		case pair[0] == "":
			parts[i] = fmt.Sprintf("[%s: _ → %s]", k, pair[1])
		case pair[1] == "":
			parts[i] = fmt.Sprintf("[%s: removed]", k)
		default:
			parts[i] = fmt.Sprintf("[%s: %s → %s]", k, pair[0], pair[1])
		}
	}
	return strings.Join(parts, " ")
}

// fmtBindings renders structured match bindings as "a = v1, b = v2".
func fmtBindings(bindings map[string]string) string {
	keys := make([]string, 0, len(bindings))
	for k := range bindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " = " + bindings[k]
	}
	return strings.Join(parts, ", ")
}
