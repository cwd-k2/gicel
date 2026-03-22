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

// --- Legacy AST compatibility types ---
// These replicate the old syntax AST types that were removed during the unified
// syntax migration. They exist solely to preserve the checker's internal
// class/instance/type-family processing logic during the transition.
// They will be removed once the checker is fully adapted to the unified syntax.

// legacyDeclClass mirrors the removed syntax.DeclClass.
type legacyDeclClass struct {
	Name           string
	Supers         []syntax.TypeExpr
	TyParams       []syntax.TyBinder
	FunDeps        []legacyFunDep
	Methods        []legacyClassMethod
	AssocTypes     []legacyAssocTypeDecl
	AssocDataDecls []legacyAssocDataDecl
	S              span.Span
}

// legacyDeclInstance mirrors the removed syntax.DeclInstance.
type legacyDeclInstance struct {
	Context       []syntax.TypeExpr
	ClassName     string
	TypeArgs      []syntax.TypeExpr
	Methods       []legacyInstMethod
	AssocTypeDefs []legacyAssocTypeDef
	AssocDataDefs []legacyAssocDataDef
	S             span.Span
}

// legacyDeclTypeFamily mirrors the removed syntax.DeclTypeFamily.
type legacyDeclTypeFamily struct {
	Name       string
	Params     []syntax.TyBinder
	ResultKind syntax.KindExpr
	ResultName string
	Deps       []legacyFunDep
	Equations  []legacyTFEquation
	S          span.Span
}

type legacyFunDep struct {
	From string
	To   []string
}

type legacyAssocTypeDecl struct {
	Name       string
	Params     []syntax.TyBinder
	ResultKind syntax.KindExpr
	ResultName string
	Deps       []legacyFunDep
	S          span.Span
}

type legacyAssocDataDecl struct {
	Name       string
	Params     []syntax.TyBinder
	ResultKind syntax.KindExpr
	S          span.Span
}

type legacyTFEquation struct {
	Name     string
	Patterns []syntax.TypeExpr
	RHS      syntax.TypeExpr
	S        span.Span
}

type legacyAssocTypeDef struct {
	Name     string
	Patterns []syntax.TypeExpr
	RHS      syntax.TypeExpr
	S        span.Span
}

type legacyAssocDataDef struct {
	Name     string
	Patterns []syntax.TypeExpr
	Cons     []legacyDeclCon
	S        span.Span
}

type legacyDeclCon struct {
	Name   string
	Fields []syntax.TypeExpr
	S      span.Span
}

type legacyInstMethod struct {
	Name string
	Expr syntax.Expr
	S    span.Span
}

type legacyGADTConDecl struct {
	Name string
	Type syntax.TypeExpr
	S    span.Span
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
	// Build legacy method list from row fields.
	var methods []legacyClassMethod
	for _, f := range parts.Fields {
		methods = append(methods, legacyClassMethod{
			Name: f.Label,
			Type: f.Type,
			S:    f.S,
		})
	}

	// Build legacy associated type declarations from row type decls.
	var assocTypes []legacyAssocTypeDecl
	for _, td := range parts.TypeDecls {
		assocTypes = append(assocTypes, legacyAssocTypeDecl{
			Name:       td.Name,
			ResultKind: td.KindAnn,
			S:          td.S,
		})
	}

	legacy := &legacyDeclClass{
		Name:       d.Name,
		TyParams:   parts.Params,
		Supers:     parts.Supers,
		Methods:    methods,
		AssocTypes: assocTypes,
		S:          d.S,
	}
	ch.processClassDecl(legacy, prog)
}

// processImplHeader processes an impl declaration header, converting it to
// the instance processing pipeline.
func (ch *Checker) processImplHeader(impl *syntax.DeclImpl) (*InstanceInfo, map[string]syntax.Expr) {
	// Decompose the type annotation into class name and type args.
	// impl Eq Bool := { ... }  →  Ann = TyExprApp(TyExprCon("Eq"), TyExprCon("Bool"))
	// impl Monad Maybe := { ... }  →  Ann = TyExprApp(TyExprCon("Monad"), TyExprCon("Maybe"))

	// Peel off context constraints: impl (Eq a) => Ord a := { ... }
	ann := impl.Ann
	var context []syntax.TypeExpr
	for {
		if qual, ok := ann.(*syntax.TyExprQual); ok {
			context = append(context, desugarConstraints(qual.Constraint)...)
			ann = qual.Body
		} else {
			break
		}
	}

	// Unwind application to get class name and type args.
	className, typeArgs := unwindTypeApp(ann)
	if className == "" {
		return nil, nil
	}

	// Collect method expressions from the body (record expression).
	methods := extractImplMethods(impl.Body)

	legacy := &legacyDeclInstance{
		Context:   context,
		ClassName: className,
		TypeArgs:  typeArgs,
		Methods:   methods,
		S:         impl.S,
	}
	return ch.processInstanceHeaderLegacy(legacy)
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

// extractImplMethods extracts method definitions from an impl body expression.
// The body is expected to be a record expression: { m1 := e1; m2 := e2; ... }
func extractImplMethods(body syntax.Expr) []legacyInstMethod {
	rec, ok := body.(*syntax.ExprRecord)
	if !ok {
		return nil
	}
	var methods []legacyInstMethod
	for _, f := range rec.Fields {
		methods = append(methods, legacyInstMethod{
			Name: f.Label,
			Expr: f.Value,
			S:    f.S,
		})
	}
	return methods
}
