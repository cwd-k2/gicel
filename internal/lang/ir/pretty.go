package ir

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Pretty renders a Core term as readable pseudo-syntax.
func Pretty(c Core, ops *types.TypeOps) string {
	return prettyCore(c, 0, ops)
}

func prettyCore(c Core, indent int, ops *types.TypeOps) string {
	pad := strings.Repeat("  ", indent)
	switch n := c.(type) {
	case *Var:
		if n.Module != "" {
			return n.Module + "." + n.Name
		}
		return n.Name
	case *Lam:
		return "\\" + n.Param + " -> " + prettyCore(n.Body, indent, ops)
	case *App:
		return "(" + prettyCore(n.Fun, indent, ops) + " " + prettyCore(n.Arg, indent, ops) + ")"
	case *TyApp:
		return "(" + prettyCore(n.Expr, indent, ops) + " @" + ops.Pretty(n.TyArg) + ")"
	case *TyLam:
		return "/\\" + n.TyParam + " -> " + prettyCore(n.Body, indent, ops)
	case *Con:
		conName := n.Name
		if n.Module != "" {
			conName = n.Module + "." + n.Name
		}
		if len(n.Args) == 0 {
			return conName
		}
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = prettyCore(a, indent, ops)
		}
		return "(" + conName + " " + strings.Join(args, " ") + ")"
	case *Case:
		var b strings.Builder
		fmt.Fprintf(&b, "case %s of", prettyCore(n.Scrutinee, indent, ops))
		for _, alt := range n.Alts {
			fmt.Fprintf(&b, "\n%s  %s -> %s", pad, prettyPattern(alt.Pattern), prettyCore(alt.Body, indent+1, ops))
		}
		return b.String()
	case *Fix:
		return "fix " + n.Name + " in " + prettyCore(n.Body, indent+1, ops)
	case *Pure:
		return "(pure " + prettyCore(n.Expr, indent, ops) + ")"
	case *Bind:
		return "(bind " + prettyCore(n.Comp, indent, ops) + " " + n.Var + " " + prettyCore(n.Body, indent, ops) + ")"
	case *Thunk:
		return "(thunk " + prettyCore(n.Comp, indent, ops) + ")"
	case *Force:
		return "(force " + prettyCore(n.Expr, indent, ops) + ")"
	case *Merge:
		return "(merge " + prettyCore(n.Left, indent, ops) + " " + prettyCore(n.Right, indent, ops) + ")"
	case *PrimOp:
		if len(n.Args) == 0 {
			return "(prim " + n.Name + ")"
		}
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = prettyCore(a, indent, ops)
		}
		return "(prim " + n.Name + " " + strings.Join(args, " ") + ")"
	case *Lit:
		return fmt.Sprintf("(lit %v)", n.Value)
	case *RecordLit:
		if len(n.Fields) == 0 {
			return "{}"
		}
		fields := make([]string, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = f.Label + ": " + prettyCore(f.Value, indent, ops)
		}
		return "{ " + strings.Join(fields, ", ") + " }"
	case *RecordProj:
		return "(" + prettyCore(n.Record, indent, ops) + ".#" + n.Label + ")"
	case *RecordUpdate:
		updates := make([]string, len(n.Updates))
		for i, f := range n.Updates {
			updates[i] = f.Label + ": " + prettyCore(f.Value, indent, ops)
		}
		return "{ " + prettyCore(n.Record, indent, ops) + " | " + strings.Join(updates, ", ") + " }"
	case *VariantLit:
		return "(inject #" + n.Tag + " " + prettyCore(n.Value, indent, ops) + ")"
	case *Error:
		return "<error>"
	default:
		// Degraded output for unknown nodes — never panic in a formatter
		// that may be called from explain trace or diagnostic display.
		return fmt.Sprintf("<%T>", c)
	}
}

func prettyPattern(p Pattern) string {
	switch pat := p.(type) {
	case *PVar:
		return pat.Name
	case *PWild:
		return "_"
	case *PCon:
		if len(pat.Args) == 0 {
			return pat.Con
		}
		args := make([]string, len(pat.Args))
		for i, a := range pat.Args {
			args[i] = prettyPattern(a)
		}
		return "(" + pat.Con + " " + strings.Join(args, " ") + ")"
	case *PRecord:
		if len(pat.Fields) == 0 {
			return "{}"
		}
		fields := make([]string, len(pat.Fields))
		for i, f := range pat.Fields {
			fields[i] = f.Label + ": " + prettyPattern(f.Pattern)
		}
		return "{ " + strings.Join(fields, ", ") + " }"
	case *PLit:
		return fmt.Sprintf("%v", pat.Value)
	default:
		return "?"
	}
}

// PrettyProgram renders a full program.
func PrettyProgram(p *Program, ops *types.TypeOps) string {
	var b strings.Builder
	for _, d := range p.DataDecls {
		fmt.Fprintf(&b, "form %s", d.Name)
		for _, tp := range d.TyParams {
			fmt.Fprintf(&b, " %s", tp.Name)
		}
		for i, c := range d.Cons {
			if i == 0 {
				b.WriteString(" = ")
			} else {
				b.WriteString(" | ")
			}
			b.WriteString(c.Name)
			for _, f := range c.Fields {
				fmt.Fprintf(&b, " %s", ops.Pretty(f))
			}
		}
		b.WriteByte('\n')
	}
	for _, bind := range p.Bindings {
		fmt.Fprintf(&b, "%s = %s\n", bind.Name, Pretty(bind.Expr, ops))
	}
	return b.String()
}
