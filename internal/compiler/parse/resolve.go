package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// ResolveFixity walks the AST and resolves all ExprInfixSpine nodes
// into nested ExprInfix trees using the provided fixity map.
func ResolveFixity(prog *syn.AstProgram, fixity map[string]syn.Fixity, errors *diagnostic.Errors) {
	r := &resolver{fixity: fixity, errors: errors}
	for i, d := range prog.Decls {
		prog.Decls[i] = r.resolveDecl(d)
	}
}

// CollectModuleFixity extracts fixity declarations from parsed decls.
func CollectModuleFixity(decls []syn.Decl) map[string]syn.Fixity {
	fixity := make(map[string]syn.Fixity)
	for _, d := range decls {
		if f, ok := d.(*syn.DeclFixity); ok && f != nil {
			fixity[f.Op] = syn.Fixity{Assoc: f.Assoc, Prec: f.Prec}
		}
	}
	return fixity
}

type resolver struct {
	fixity map[string]syn.Fixity
	errors *diagnostic.Errors
}

func (r *resolver) lookupFixity(op string) syn.Fixity {
	if f, ok := r.fixity[op]; ok {
		return f
	}
	return syn.Fixity{Assoc: syn.AssocLeft, Prec: 9}
}

// resolveSpine converts a flat operator spine into a precedence-correct
// nested ExprInfix tree using the shunting-yard algorithm.
func (r *resolver) resolveSpine(e *syn.ExprInfixSpine) syn.Expr {
	// First resolve any nested spines inside operands.
	for i, op := range e.Operands {
		e.Operands[i] = r.resolveExpr(op)
	}

	type opEntry struct {
		name string
		fix  syn.Fixity
		span span.Span
	}

	output := []syn.Expr{e.Operands[0]}
	var opStack []opEntry

	for i, opName := range e.Ops {
		fix := r.lookupFixity(opName)
		rhs := e.Operands[i+1]

		// Pop operators with higher precedence (or equal + left-assoc)
		for len(opStack) > 0 {
			top := opStack[len(opStack)-1]
			pop := false
			if top.fix.Prec > fix.Prec {
				pop = true
			} else if top.fix.Prec == fix.Prec {
				if top.fix.Assoc == syn.AssocNone || fix.Assoc == syn.AssocNone {
					r.errors.Add(&diagnostic.Error{
						Code:    diagnostic.ErrInvalidOperator,
						Phase:   diagnostic.PhaseParse,
						Span:    e.OpSpans[i],
						Message: "cannot mix non-associative operators of equal precedence",
					})
					pop = true
				} else if fix.Assoc == syn.AssocLeft {
					pop = true
				}
				// AssocRight: don't pop (right-assoc chains)
			}
			if !pop {
				break
			}
			opStack = opStack[:len(opStack)-1]
			right := output[len(output)-1]
			left := output[len(output)-2]
			output = output[:len(output)-1]
			output[len(output)-1] = &syn.ExprInfix{
				Left: left, Op: top.name, OpSpan: top.span, Right: right,
				S: span.Span{Start: left.Span().Start, End: right.Span().End},
			}
		}

		opStack = append(opStack, opEntry{name: opName, fix: fix, span: e.OpSpans[i]})
		output = append(output, rhs)
	}

	// Drain remaining operators
	for len(opStack) > 0 {
		top := opStack[len(opStack)-1]
		opStack = opStack[:len(opStack)-1]
		right := output[len(output)-1]
		left := output[len(output)-2]
		output = output[:len(output)-1]
		output[len(output)-1] = &syn.ExprInfix{
			Left: left, Op: top.name, OpSpan: top.span, Right: right,
			S: span.Span{Start: left.Span().Start, End: right.Span().End},
		}
	}

	return output[0]
}

// resolveExpr recursively resolves spines in an expression tree.
func (r *resolver) resolveExpr(e syn.Expr) syn.Expr {
	if e == nil {
		return nil
	}
	switch e := e.(type) {
	case *syn.ExprInfixSpine:
		return r.resolveSpine(e)
	case *syn.ExprInfix:
		e.Left = r.resolveExpr(e.Left)
		e.Right = r.resolveExpr(e.Right)
		return e
	case *syn.ExprApp:
		e.Fun = r.resolveExpr(e.Fun)
		e.Arg = r.resolveExpr(e.Arg)
		return e
	case *syn.ExprTyApp:
		e.Expr = r.resolveExpr(e.Expr)
		return e
	case *syn.ExprLam:
		e.Body = r.resolveExpr(e.Body)
		return e
	case *syn.ExprCase:
		e.Scrutinee = r.resolveExpr(e.Scrutinee)
		for i := range e.Alts {
			e.Alts[i].Body = r.resolveExpr(e.Alts[i].Body)
		}
		return e
	case *syn.ExprDo:
		for i := range e.Stmts {
			e.Stmts[i] = r.resolveStmt(e.Stmts[i])
		}
		return e
	case *syn.ExprBlock:
		for i := range e.Binds {
			e.Binds[i].Expr = r.resolveExpr(e.Binds[i].Expr)
		}
		e.Body = r.resolveExpr(e.Body)
		return e
	case *syn.ExprAnn:
		e.Expr = r.resolveExpr(e.Expr)
		return e
	case *syn.ExprParen:
		e.Inner = r.resolveExpr(e.Inner)
		return e
	case *syn.ExprList:
		for i := range e.Elems {
			e.Elems[i] = r.resolveExpr(e.Elems[i])
		}
		return e
	case *syn.ExprRecord:
		for i := range e.Fields {
			e.Fields[i].Value = r.resolveExpr(e.Fields[i].Value)
		}
		return e
	case *syn.ExprRecordUpdate:
		e.Record = r.resolveExpr(e.Record)
		for i := range e.Updates {
			e.Updates[i].Value = r.resolveExpr(e.Updates[i].Value)
		}
		return e
	case *syn.ExprProject:
		e.Record = r.resolveExpr(e.Record)
		return e
	case *syn.ExprSection:
		e.Arg = r.resolveExpr(e.Arg)
		return e
	case *syn.ExprEvidence:
		e.Dict = r.resolveExpr(e.Dict)
		e.Body = r.resolveExpr(e.Body)
		return e
	case *syn.ExprIf:
		e.Cond = r.resolveExpr(e.Cond)
		e.Then = r.resolveExpr(e.Then)
		e.Else = r.resolveExpr(e.Else)
		return e
	case *syn.ExprNegate:
		e.Expr = r.resolveExpr(e.Expr)
		return e
	case *syn.ExprTuple:
		for i := range e.Elems {
			e.Elems[i] = r.resolveExpr(e.Elems[i])
		}
		return e
	// Leaf nodes: no sub-expressions to resolve.
	case *syn.ExprVar, *syn.ExprCon, *syn.ExprQualVar, *syn.ExprQualCon,
		*syn.ExprIntLit, *syn.ExprDoubleLit, *syn.ExprStrLit, *syn.ExprRuneLit,
		*syn.ExprError:
		return e
	}
	return e
}

func (r *resolver) resolveStmt(s syn.Stmt) syn.Stmt {
	switch s := s.(type) {
	case *syn.StmtBind:
		s.Comp = r.resolveExpr(s.Comp)
		return s
	case *syn.StmtPureBind:
		s.Expr = r.resolveExpr(s.Expr)
		return s
	case *syn.StmtExpr:
		s.Expr = r.resolveExpr(s.Expr)
		return s
	}
	return s
}

// resolveDecl dispatches resolution on declaration bodies.
func (r *resolver) resolveDecl(d syn.Decl) syn.Decl {
	switch d := d.(type) {
	case *syn.DeclValueDef:
		if d.Expr != nil {
			d.Expr = r.resolveExpr(d.Expr)
		}
		return d
	case *syn.DeclImpl:
		d.Body = r.resolveExpr(d.Body)
		return d
	// No Expr sub-trees to resolve:
	case *syn.DeclTypeAnn, *syn.DeclForm, *syn.DeclTypeAlias, *syn.DeclFixity:
		return d
	}
	return d
}
