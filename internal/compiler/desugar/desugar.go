// Package desugar transforms surface AST nodes into simpler forms
// before type checking. This separates parsing (faithful representation
// of user syntax) from lowering (translation to checker-ready forms).
//
// Transformations:
//   - ExprIf → ExprCase over True/False
//   - ExprNegate → ExprApp(negate, e)
//   - ExprTuple → ExprRecord with _1, _2, ... labels
package desugar

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"
)

// Program desugars all expressions in an AST program.
func Program(prog *syn.Program) {
	for i := range prog.Decls {
		prog.Decls[i] = desugarDecl(prog.Decls[i])
	}
}

func desugarDecl(d syn.Decl) syn.Decl {
	switch d := d.(type) {
	case *syn.DeclValueDef:
		d.Expr = desugarExpr(d.Expr)
		return d
	case *syn.DeclImpl:
		d.Body = desugarExpr(d.Body)
		return d
	default:
		return d
	}
}

func desugarExpr(e syn.Expr) syn.Expr {
	if e == nil {
		return nil
	}
	switch e := e.(type) {
	// --- Surface nodes → lowered forms ---

	case *syn.ExprIf:
		cond := desugarExpr(e.Cond)
		then := desugarExpr(e.Then)
		els := desugarExpr(e.Else)
		return &syn.ExprCase{
			Scrutinee: cond,
			Alts: []syn.Alt{
				{Pattern: &syn.PatCon{Con: "True", S: e.S}, Body: then, S: e.S},
				{Pattern: &syn.PatCon{Con: "False", S: e.S}, Body: els, S: e.S},
			},
			S:         e.S,
			IfDesugar: true,
		}

	case *syn.ExprNegate:
		arg := desugarExpr(e.Expr)
		return &syn.ExprApp{
			Fun: &syn.ExprVar{Name: "negate", S: e.S},
			Arg: arg,
			S:   e.S,
		}

	case *syn.ExprTuple:
		fields := make([]syn.RecordField, len(e.Elems))
		for i, el := range e.Elems {
			fields[i] = syn.RecordField{
				Label: syn.TupleLabel(i + 1),
				Value: desugarExpr(el),
				S:     el.Span(),
			}
		}
		return &syn.ExprRecord{Fields: fields, S: e.S}

	// --- Recursive traversal ---

	case *syn.ExprApp:
		e.Fun = desugarExpr(e.Fun)
		e.Arg = desugarExpr(e.Arg)
		return e
	case *syn.ExprTyApp:
		e.Expr = desugarExpr(e.Expr)
		return e
	case *syn.ExprLam:
		e.Body = desugarExpr(e.Body)
		return e
	case *syn.ExprCase:
		e.Scrutinee = desugarExpr(e.Scrutinee)
		for i := range e.Alts {
			e.Alts[i].Body = desugarExpr(e.Alts[i].Body)
		}
		return e
	case *syn.ExprDo:
		for i := range e.Stmts {
			e.Stmts[i] = desugarStmt(e.Stmts[i])
		}
		return e
	case *syn.ExprBlock:
		for i := range e.Binds {
			e.Binds[i].Expr = desugarExpr(e.Binds[i].Expr)
		}
		e.Body = desugarExpr(e.Body)
		return e
	case *syn.ExprInfix:
		e.Left = desugarExpr(e.Left)
		e.Right = desugarExpr(e.Right)
		return e
	case *syn.ExprInfixSpine:
		for i := range e.Operands {
			e.Operands[i] = desugarExpr(e.Operands[i])
		}
		return e
	case *syn.ExprAnn:
		e.Expr = desugarExpr(e.Expr)
		return e
	case *syn.ExprParen:
		e.Inner = desugarExpr(e.Inner)
		return e
	case *syn.ExprList:
		for i := range e.Elems {
			e.Elems[i] = desugarExpr(e.Elems[i])
		}
		return e
	case *syn.ExprRecord:
		for i := range e.Fields {
			e.Fields[i].Value = desugarExpr(e.Fields[i].Value)
		}
		return e
	case *syn.ExprRecordUpdate:
		e.Record = desugarExpr(e.Record)
		for i := range e.Updates {
			e.Updates[i].Value = desugarExpr(e.Updates[i].Value)
		}
		return e
	case *syn.ExprProject:
		e.Record = desugarExpr(e.Record)
		return e
	case *syn.ExprSection:
		e.Arg = desugarExpr(e.Arg)
		return e
	case *syn.ExprEvidence:
		e.Dict = desugarExpr(e.Dict)
		e.Body = desugarExpr(e.Body)
		return e

	// Leaf nodes.
	case *syn.ExprVar, *syn.ExprCon, *syn.ExprQualVar, *syn.ExprQualCon,
		*syn.ExprIntLit, *syn.ExprStrLit, *syn.ExprRuneLit, *syn.ExprDoubleLit,
		*syn.ExprError:
		return e

	default:
		return e
	}
}

func desugarStmt(s syn.Stmt) syn.Stmt {
	switch s := s.(type) {
	case *syn.StmtBind:
		s.Comp = desugarExpr(s.Comp)
		return s
	case *syn.StmtPureBind:
		s.Expr = desugarExpr(s.Expr)
		return s
	case *syn.StmtExpr:
		s.Expr = desugarExpr(s.Expr)
		return s
	default:
		return s
	}
}
