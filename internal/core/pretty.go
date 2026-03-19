package core

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/types"
)

// Pretty renders a Core term as readable pseudo-syntax.
func Pretty(c Core) string {
	return prettyCore(c, 0)
}

func prettyCore(c Core, indent int) string {
	pad := strings.Repeat("  ", indent)
	switch n := c.(type) {
	case *Var:
		if n.Module != "" {
			return n.Module + "." + n.Name
		}
		return n.Name
	case *Lam:
		return fmt.Sprintf("\\%s -> %s", n.Param, prettyCore(n.Body, indent))
	case *App:
		return fmt.Sprintf("(%s %s)", prettyCore(n.Fun, indent), prettyCore(n.Arg, indent))
	case *TyApp:
		return fmt.Sprintf("(%s @%s)", prettyCore(n.Expr, indent), types.Pretty(n.TyArg))
	case *TyLam:
		return fmt.Sprintf("/\\%s -> %s", n.TyParam, prettyCore(n.Body, indent))
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
			args[i] = prettyCore(a, indent)
		}
		return fmt.Sprintf("(%s %s)", conName, strings.Join(args, " "))
	case *Case:
		var b strings.Builder
		fmt.Fprintf(&b, "case %s of", prettyCore(n.Scrutinee, indent))
		for _, alt := range n.Alts {
			fmt.Fprintf(&b, "\n%s  %s -> %s", pad, prettyPattern(alt.Pattern), prettyCore(alt.Body, indent+1))
		}
		return b.String()
	case *Fix:
		return fmt.Sprintf("fix %s in %s", n.Name, prettyCore(n.Body, indent+1))
	case *Pure:
		return fmt.Sprintf("(pure %s)", prettyCore(n.Expr, indent))
	case *Bind:
		return fmt.Sprintf("(bind %s %s %s)", prettyCore(n.Comp, indent), n.Var, prettyCore(n.Body, indent))
	case *Thunk:
		return fmt.Sprintf("(thunk %s)", prettyCore(n.Comp, indent))
	case *Force:
		return fmt.Sprintf("(force %s)", prettyCore(n.Expr, indent))
	case *PrimOp:
		if len(n.Args) == 0 {
			return fmt.Sprintf("(prim %s)", n.Name)
		}
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = prettyCore(a, indent)
		}
		return fmt.Sprintf("(prim %s %s)", n.Name, strings.Join(args, " "))
	case *Lit:
		return fmt.Sprintf("(lit %v)", n.Value)
	case *RecordLit:
		if len(n.Fields) == 0 {
			return "{}"
		}
		fields := make([]string, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = fmt.Sprintf("%s: %s", f.Label, prettyCore(f.Value, indent))
		}
		return fmt.Sprintf("{ %s }", strings.Join(fields, ", "))
	case *RecordProj:
		return fmt.Sprintf("(%s.#%s)", prettyCore(n.Record, indent), n.Label)
	case *RecordUpdate:
		updates := make([]string, len(n.Updates))
		for i, f := range n.Updates {
			updates[i] = fmt.Sprintf("%s: %s", f.Label, prettyCore(f.Value, indent))
		}
		return fmt.Sprintf("{ %s | %s }", prettyCore(n.Record, indent), strings.Join(updates, ", "))
	default:
		return "<?>"
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
		return fmt.Sprintf("(%s %s)", pat.Con, strings.Join(args, " "))
	case *PRecord:
		if len(pat.Fields) == 0 {
			return "{}"
		}
		fields := make([]string, len(pat.Fields))
		for i, f := range pat.Fields {
			fields[i] = fmt.Sprintf("%s: %s", f.Label, prettyPattern(f.Pattern))
		}
		return fmt.Sprintf("{ %s }", strings.Join(fields, ", "))
	case *PLit:
		return fmt.Sprintf("%v", pat.Value)
	default:
		return "?"
	}
}

// PrettyProgram renders a full program.
func PrettyProgram(p *Program) string {
	var b strings.Builder
	for _, d := range p.DataDecls {
		fmt.Fprintf(&b, "data %s", d.Name)
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
				fmt.Fprintf(&b, " %s", types.Pretty(f))
			}
		}
		b.WriteByte('\n')
	}
	for _, bind := range p.Bindings {
		fmt.Fprintf(&b, "%s = %s\n", bind.Name, Pretty(bind.Expr))
	}
	return b.String()
}
