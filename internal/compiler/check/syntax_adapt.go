package check

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
)

// --- Unified syntax adaptation layer ---
//
// The unified syntax represents data declarations as TypeExpr bodies:
//   data Name := \params. [constraints =>] { fields };
//
// The checker's internal processing expects decomposed components.
// These functions bridge the gap.

// dataBodyParts holds the decomposed components of a data declaration body.
type dataBodyParts struct {
	Params    []syntax.TyBinder
	Supers    []syntax.TypeExpr
	Fields    []syntax.TyRowField
	TypeDecls []syntax.TyRowTypeDecl
	InnerBody syntax.TypeExpr // the raw body (for non-record data)
}

// decomposeDataBody decomposes a data declaration body TypeExpr into
// parameters, superclass constraints, row fields, and associated type declarations.
//
//	\(a: Type). Eq a => { eq: a -> a -> Bool; }
//	→ Params: [a:Type], Supers: [Eq a], Fields: [{eq: a->a->Bool}]
func decomposeDataBody(body syntax.TypeExpr) dataBodyParts {
	var parts dataBodyParts
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
	// Check for tuple constraint: Record { _1: C1, _2: C2 }
	if app, ok := c.(*syntax.TyExprApp); ok {
		if con, ok := app.Fun.(*syntax.TyExprCon); ok && con.Name == "Record" {
			if row, ok := app.Arg.(*syntax.TyExprRow); ok && len(row.Fields) > 0 {
				cs := make([]syntax.TypeExpr, len(row.Fields))
				for i, f := range row.Fields {
					cs[i] = f.Type
				}
				return cs
			}
		}
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

// isClassLikeData determines whether a data declaration represents a type class
// (all fields are lowercase methods) vs a regular data type (at least one uppercase constructor).
func isClassLikeData(parts dataBodyParts) bool {
	if len(parts.Fields) == 0 {
		return false
	}
	for _, f := range parts.Fields {
		if len(f.Label) > 0 && f.Label[0] >= 'A' && f.Label[0] <= 'Z' {
			return false // has an uppercase field = constructor = not a class
		}
	}
	return true // all lowercase fields = methods = class-like
}

// --- Compatibility wrappers ---
// These convert from the unified syntax AST to the checker's internal processing.

// legacyClassDecl builds a legacy DeclClass-equivalent from a data declaration.
// This is used during the transition period while the checker internals still
// expect the old class processing pipeline.
type legacyClassDecl struct {
	Name       string
	TyParams   []syntax.TyBinder
	Supers     []syntax.TypeExpr
	Methods    []legacyClassMethod
	AssocTypes []syntax.TyRowTypeDecl
	S          span.Span
}

type legacyClassMethod struct {
	Name string
	Type syntax.TypeExpr
	S    span.Span
}

// legacyInstanceDecl builds a legacy DeclInstance-equivalent from an impl declaration.
type legacyInstanceDecl struct {
	Context   []syntax.TypeExpr
	ClassName string
	TypeArgs  []syntax.TypeExpr
	Methods   map[string]syntax.Expr
	S         span.Span
}

// processClassFromData processes a class-like data declaration by converting it
// to the legacy class processing pipeline.
func (ch *Checker) processClassFromData(d *syntax.DeclData, parts dataBodyParts, prog *ir.Program) {
	// Convert parts → legacy class decl and delegate to processClassDecl-equivalent logic.
	// For now, this is a stub — full implementation requires threading through the
	// existing class processing code.
	// TODO: Implement full class-from-data processing.
	_ = d
	_ = parts
	_ = prog
}

// processImplHeader processes an impl declaration header, converting it to
// the instance processing pipeline.
func (ch *Checker) processImplHeader(impl *syntax.DeclImpl) (*InstanceInfo, map[string]syntax.Expr) {
	// TODO: Implement full impl header processing.
	// This needs to:
	// 1. Resolve the type annotation to determine class name and type args
	// 2. Register the instance in the registry
	// 3. Collect method expressions from the body
	_ = impl
	return nil, nil
}
