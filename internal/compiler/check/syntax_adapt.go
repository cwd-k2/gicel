package check

import (
	"github.com/cwd-k2/gicel/internal/lang/syntax"
)

// --- Unified syntax adaptation layer ---
//
// The unified syntax represents form declarations as TypeExpr bodies:
//   form Name := \params. [constraints =>] { fields };
//
// The checker's internal processing expects decomposed components.
// These functions bridge the gap.

// formBodyParts holds the decomposed components of a form declaration body.
type formBodyParts struct {
	Params    []syntax.TyBinder
	Supers    []syntax.TypeExpr
	Fields    []syntax.TyRowField
	TypeDecls []syntax.TyRowTypeDecl
	InnerBody syntax.TypeExpr // the raw body (for non-record forms)
}

// decomposeFormBody decomposes a form declaration body TypeExpr into
// parameters, superclass constraints, row fields, and associated type declarations.
//
//	\(a: Type). Eq a => { eq: a -> a -> Bool; }
//	→ Params: [a:Type], Supers: [Eq a], Fields: [{eq: a->a->Bool}]
func decomposeFormBody(body syntax.TypeExpr) formBodyParts {
	var parts formBodyParts
	t := body

	// Peel off forall binders: \params. body
	if fa, ok := t.(*syntax.TyExprForall); ok {
		parts.Params = fa.Binders
		t = fa.Body
	}

	// Peel off constraints: C1 => C2 => ... => body
	for {
		if qual, ok := t.(*syntax.TyExprQual); ok {
			supers := desugarConstraints(qual.Constraint)
			parts.Supers = append(parts.Supers, supers...)
			t = qual.Body
		} else {
			break
		}
	}

	// Expect a row type: { fields }
	if row, ok := t.(*syntax.TyExprRow); ok {
		parts.Fields = row.Fields
		parts.TypeDecls = row.TypeDecls
	}

	parts.InnerBody = t
	return parts
}

// desugarConstraints unpacks a constraint that may be a tuple.
// (Eq a, Ord a) → [Eq a, Ord a]; Eq a → [Eq a].
func desugarConstraints(c syntax.TypeExpr) []syntax.TypeExpr {
	if cs := syntax.DesugarConstraintTuple(c); cs != nil {
		return cs
	}
	return []syntax.TypeExpr{c}
}

// typeAliasParts holds the decomposed components of a type alias body.
type typeAliasParts struct {
	Params []syntax.TyBinder
	Body   syntax.TypeExpr
}

// decomposeTypeAliasBody extracts parameters from lambda prefixes.
//
//	\a b. T → Params: [a, b], Body: T
func decomposeTypeAliasBody(body syntax.TypeExpr) typeAliasParts {
	var parts typeAliasParts
	t := body

	if fa, ok := t.(*syntax.TyExprForall); ok {
		parts.Params = fa.Binders
		t = fa.Body
	}

	parts.Body = t
	return parts
}

// isTypeFamilyBody checks if a type alias body contains a case expression,
// indicating it's a closed type family.
func isTypeFamilyBody(body syntax.TypeExpr) bool {
	t := body
	// Peel off forall
	if fa, ok := t.(*syntax.TyExprForall); ok {
		t = fa.Body
	}
	_, ok := t.(*syntax.TyExprCase)
	return ok
}

// isClassLikeForm determines whether a form declaration represents a type class
// vs a regular algebraic data type.
//
// The distinction follows GICEL's lexical category rule: constructors start
// with uppercase, type class methods start with lowercase. This is a language
// invariant enforced by the parser (TokUpper vs TokLower), not a naming convention.
//
// Additionally, the body must be a record row ({ fields }), not a raw type expression
// or pipe-ADT sugar. A non-row body (e.g., form Void := \a. a) is never class-like.
func isClassLikeForm(parts formBodyParts) bool {
	// Must have a record body.
	if _, isRow := parts.InnerBody.(*syntax.TyExprRow); !isRow {
		return false
	}
	// Must have at least one field or associated type declaration.
	// A class with only associated types (no methods) is valid.
	if len(parts.Fields) == 0 && len(parts.TypeDecls) == 0 {
		return false
	}
	for _, f := range parts.Fields {
		if len(f.Label) > 0 && f.Label[0] >= 'A' && f.Label[0] <= 'Z' {
			return false // uppercase field = constructor = algebraic type
		}
	}
	return true
}

// unwindTypeApp decomposes a type expression into a head name and arguments.
//
//	Eq Bool → ("Eq", [Bool])
//	Monad (List a) → ("Monad", [List a])
//	Eq → ("Eq", [])
func unwindTypeApp(t syntax.TypeExpr) (string, []syntax.TypeExpr) {
	var args []syntax.TypeExpr
	for {
		if app, ok := t.(*syntax.TyExprApp); ok {
			args = append([]syntax.TypeExpr{app.Arg}, args...)
			t = app.Fun
		} else if paren, ok := t.(*syntax.TyExprParen); ok {
			t = paren.Inner
		} else {
			break
		}
	}
	switch head := t.(type) {
	case *syntax.TyExprCon:
		return head.Name, args
	case *syntax.TyExprQualCon:
		return head.Qualifier + "." + head.Name, args
	}
	return "", nil
}

// implBodyParts holds decomposed impl body contents.
type implBodyParts struct {
	Methods  map[string]syntax.Expr // method name → unevaluated expression
	TypeDefs []syntax.ImplField     // associated type/data definitions from impl body
}

// extractImplBody extracts method definitions and associated type definitions
// from an impl body expression.
func extractImplBody(body syntax.Expr) implBodyParts {
	parts := implBodyParts{Methods: make(map[string]syntax.Expr)}
	switch b := body.(type) {
	case *syntax.ExprRecord:
		for _, f := range b.Fields {
			parts.Methods[f.Label] = f.Value
		}
	case *syntax.ExprBlock:
		for _, bind := range b.Binds {
			if name, ok := syntax.PatVarName(bind.Pat); ok {
				parts.Methods[name] = bind.Expr
			}
		}
		parts.TypeDefs = append(parts.TypeDefs, b.TypeDefs...)
	}
	return parts
}
