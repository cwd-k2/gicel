package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
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

// isClassLikeData determines whether a data declaration represents a type class
// vs a regular data type.
//
// The distinction follows GICEL's lexical category rule: data constructors start
// with uppercase, type class methods start with lowercase. This is a language
// invariant enforced by the parser (TokUpper vs TokLower), not a naming convention.
//
// Additionally, the body must be a record row ({ fields }), not a raw type expression
// or pipe-ADT sugar. A non-row body (e.g., data Void := \a. a) is never class-like.
func isClassLikeData(parts dataBodyParts) bool {
	// Must have a record body and at least one field.
	if _, isRow := parts.InnerBody.(*syntax.TyExprRow); !isRow {
		return false
	}
	if len(parts.Fields) == 0 {
		return false
	}
	for _, f := range parts.Fields {
		if len(f.Label) > 0 && f.Label[0] >= 'A' && f.Label[0] <= 'Z' {
			return false // uppercase field = constructor = data type
		}
	}
	return true
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

// --- Compatibility wrappers ---
// These convert from the unified syntax AST to the checker's internal processing.

type legacyClassMethod struct {
	Name string
	Type syntax.TypeExpr
	S    span.Span
}

// processClassFromData processes a class-like data declaration by converting it
// to the legacy class processing pipeline.
func (ch *Checker) processClassFromData(d *syntax.DeclData, parts dataBodyParts, prog *ir.Program) {
	// Build legacy method list from row fields.
	var methods []legacyClassMethod
	for _, f := range parts.Fields {
		if f.Default != nil {
			ch.addCodedError(diagnostic.ErrBadClass, f.S,
				"default method implementations are not yet supported in unified syntax")
		}
		methods = append(methods, legacyClassMethod{
			Name: f.Label,
			Type: f.Type,
			S:    f.S,
		})
	}

	// Build legacy associated type declarations from row type decls.
	// In unified syntax, associated types don't repeat class params.
	// Add the class params to make them compatible with the legacy format.
	var assocTypes []legacyAssocTypeDecl
	for _, td := range parts.TypeDecls {
		assocTypes = append(assocTypes, legacyAssocTypeDecl{
			Name:       td.Name,
			Params:     parts.Params, // inherit class params
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

	// Extract methods and associated type definitions from the body.
	bodyParts := extractImplBody(impl.Body)

	// Build legacy associated type/data definitions with patterns from type args.
	var assocTypeDefs []legacyAssocTypeDef
	var assocDataDefs []legacyAssocDataDef
	for _, td := range bodyParts.TypeDefs {
		if td.IsData && td.TypeDef != nil {
			// Associated data family definition: data Elem := ListElem a | Empty
			// The TypeDef contains the first constructor type (ListElem a).
			// Register as a data family equation (type family with mangled name).
			assocDataDefs = append(assocDataDefs, legacyAssocDataDef{
				Name:     td.Name,
				Patterns: typeArgs,
				Cons:     parseImplDataCons(td.TypeDef),
				S:        td.S,
			})
		} else if td.TypeDef != nil {
			// Type family equation (non-data associated type).
			assocTypeDefs = append(assocTypeDefs, legacyAssocTypeDef{
				Name:     td.Name,
				Patterns: typeArgs,
				RHS:      td.TypeDef,
				S:        td.S,
			})
		}
	}

	legacy := &legacyDeclInstance{
		Context:       context,
		ClassName:     className,
		TypeArgs:      typeArgs,
		Methods:       bodyParts.Methods,
		AssocTypeDefs: assocTypeDefs,
		AssocDataDefs: assocDataDefs,
		S:             impl.S,
	}
	return ch.processInstanceHeaderLegacy(legacy)
}

// parseImplDataCons extracts constructor declarations from a data family definition RHS.
// The RHS is the first constructor type (from parseType), e.g., "ListElem a".
// For ADT shorthand with |, only the first constructor is captured by the parser.
func parseImplDataCons(rhs syntax.TypeExpr) []legacyDeclCon {
	// The RHS is a type expression representing a constructor application.
	// e.g., "ListElem a" → TyExprApp(TyExprCon("ListElem"), TyExprVar("a"))
	// Extract the constructor name and field types.
	// Unwind TyExprApp chain to get head constructor and args.
	var args []syntax.TypeExpr
	t := rhs
	for {
		if app, ok := t.(*syntax.TyExprApp); ok {
			args = append([]syntax.TypeExpr{app.Arg}, args...)
			t = app.Fun
		} else {
			break
		}
	}
	if con, ok := t.(*syntax.TyExprCon); ok {
		fields := make([]syntax.TypeExpr, len(args))
		copy(fields, args)
		return []legacyDeclCon{{Name: con.Name, Fields: fields}}
	}
	return nil
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
	Methods  []legacyInstMethod
	TypeDefs []syntax.ImplField // associated type/data definitions from impl body
}

// extractImplBody extracts method definitions and associated type definitions
// from an impl body expression.
func extractImplBody(body syntax.Expr) implBodyParts {
	var parts implBodyParts
	switch b := body.(type) {
	case *syntax.ExprRecord:
		for _, f := range b.Fields {
			parts.Methods = append(parts.Methods, legacyInstMethod{
				Name: f.Label,
				Expr: f.Value,
				S:    f.S,
			})
		}
	case *syntax.ExprBlock:
		for _, bind := range b.Binds {
			parts.Methods = append(parts.Methods, legacyInstMethod{
				Name: bind.Var,
				Expr: bind.Expr,
				S:    bind.S,
			})
		}
		parts.TypeDefs = append(parts.TypeDefs, b.TypeDefs...)
	}
	return parts
}
