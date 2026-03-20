package parse

import (
	syn "github.com/cwd-k2/gicel/internal/syntax"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// parseClassDecl parses: class [Constraint =>] ClassName params { method :: Type; ... }
func (p *Parser) parseClassDecl() *syn.DeclClass {
	start := p.peek().S.Start
	p.expect(syn.TokClass)

	// Parse the head: either "ClassName params" or "Constraint => ClassName params".
	// Strategy: parse type applications, check for => to detect superclass.
	var supers []syn.TypeExpr

	// Tuple-style superclass constraints: class (C1 a, C2 b) => ClassName params { ... }
	if p.peek().Kind == syn.TokLParen && !p.isClassKindedBinder() {
		p.advance() // consume (
		for {
			conStart := p.peek().S.Start
			conName := p.expectUpper()
			var conExpr syn.TypeExpr = &syn.TyExprCon{Name: conName, S: span.Span{Start: conStart, End: p.prevEnd()}}
			for p.peek().Kind == syn.TokLower {
				tok := p.peek()
				p.advance()
				arg := &syn.TyExprVar{Name: tok.Text, S: tok.S}
				conExpr = &syn.TyExprApp{Fun: conExpr, Arg: arg, S: span.Span{Start: conStart, End: tok.S.End}}
			}
			supers = append(supers, conExpr)
			if p.peek().Kind != syn.TokComma {
				break
			}
			p.advance() // consume ,
		}
		p.expect(syn.TokRParen)
		p.expect(syn.TokFatArrow)
		// Parse class name + params
		className := p.expectUpper()
		params := p.parseTyBinderList()
		funDeps := p.parseClassFunDeps()
		methods, assocTypes, assocDataDecls := p.parseClassBody()
		return &syn.DeclClass{
			Supers: supers, Name: className, TyParams: params, FunDeps: funDeps,
			Methods: methods, AssocTypes: assocTypes, AssocDataDecls: assocDataDecls,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	firstName := p.expectUpper()
	firstArgs := p.parseClassTyArgs()

	if p.peek().Kind == syn.TokFatArrow {
		// What we parsed is a superclass constraint.
		var superExpr syn.TypeExpr = &syn.TyExprCon{Name: firstName, S: span.Span{Start: start, End: p.prevEnd()}}
		for _, arg := range firstArgs {
			superExpr = &syn.TyExprApp{Fun: superExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
		}
		supers = append(supers, superExpr)
		p.advance() // consume =>

		// Support multiple superclass constraints: Super1 a => Super2 a => ... => ClassName params
		for {
			nextName := p.expectUpper()
			nextArgs := p.parseClassTyArgs()
			if p.peek().Kind == syn.TokFatArrow {
				// Another superclass constraint.
				var nextExpr syn.TypeExpr = &syn.TyExprCon{Name: nextName, S: span.Span{Start: start, End: p.prevEnd()}}
				for _, arg := range nextArgs {
					nextExpr = &syn.TyExprApp{Fun: nextExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
				}
				supers = append(supers, nextExpr)
				p.advance() // consume =>
				continue
			}
			// This is the actual class name.
			var params []syn.TyBinder
			for _, arg := range nextArgs {
				v, ok := arg.(*syn.TyExprVar)
				if !ok {
					p.addErrorCode(errs.ErrClassSyntax, "class type parameter must be a type variable")
					continue
				}
				params = append(params, syn.TyBinder{Name: v.Name, Kind: v.Kind, S: v.S})
			}
			funDeps := p.parseClassFunDeps()
			methods, assocTypes, assocDataDecls := p.parseClassBody()
			return &syn.DeclClass{
				Supers: supers, Name: nextName, TyParams: params, FunDeps: funDeps,
				Methods: methods, AssocTypes: assocTypes, AssocDataDecls: assocDataDecls,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
	}

	// No =>, so firstName is the class name, firstArgs are params.
	var params []syn.TyBinder
	for _, arg := range firstArgs {
		v, ok := arg.(*syn.TyExprVar)
		if !ok {
			p.addErrorCode(errs.ErrClassSyntax, "class type parameter must be a type variable")
			continue
		}
		params = append(params, syn.TyBinder{Name: v.Name, Kind: v.Kind, S: v.S})
	}
	funDeps := p.parseClassFunDeps()
	methods, assocTypes, assocDataDecls := p.parseClassBody()
	return &syn.DeclClass{
		Name: firstName, TyParams: params, FunDeps: funDeps,
		Methods: methods, AssocTypes: assocTypes, AssocDataDecls: assocDataDecls,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseClassTyArgs parses a sequence of class type arguments: bare lowercase vars or
// kinded binders (v: Kind). Returns them as syn.TyExprVar nodes.
func (p *Parser) parseClassTyArgs() []syn.TypeExpr {
	var args []syn.TypeExpr
	for p.peek().Kind == syn.TokLower || (p.peek().Kind == syn.TokLParen && p.isClassKindedBinder()) {
		if p.peek().Kind == syn.TokLParen {
			lp := p.peek().S.Start
			p.advance()
			name := p.expectLower()
			p.expect(syn.TokColon)
			kind := p.parseKindExpr()
			p.expect(syn.TokRParen)
			args = append(args, &syn.TyExprVar{
				Name: name,
				S:    span.Span{Start: lp, End: p.prevEnd()},
				Kind: kind,
			})
		} else {
			tok := p.peek()
			p.advance()
			args = append(args, &syn.TyExprVar{Name: tok.Text, S: tok.S})
		}
	}
	return args
}

// parseClassFunDeps parses optional functional dependencies: | a -> b, c -> d
func (p *Parser) parseClassFunDeps() []syn.FunDep {
	if p.peek().Kind != syn.TokPipe {
		return nil
	}
	p.advance() // consume |
	return p.parseFunDepList()
}

// parseFunDepList parses a comma-separated list of functional dependencies: a -> b, c -> d
func (p *Parser) parseFunDepList() []syn.FunDep {
	var deps []syn.FunDep
	for {
		from := p.expectLower()
		p.expect(syn.TokEqColon)
		var to []string
		for p.peek().Kind == syn.TokLower {
			to = append(to, p.expectLower())
		}
		if len(to) == 0 {
			p.addErrorCode(errs.ErrClassSyntax, "functional dependency requires at least one determined parameter after '=:'")
		}
		deps = append(deps, syn.FunDep{From: from, To: to})
		if p.peek().Kind == syn.TokComma {
			p.advance()
			continue
		}
		break
	}
	return deps
}

func (p *Parser) parseClassBody() ([]syn.ClassMethod, []syn.AssocTypeDecl, []syn.AssocDataDecl) {
	p.expect(syn.TokLBrace)
	var methods []syn.ClassMethod
	var assocTypes []syn.AssocTypeDecl
	var assocDataDecls []syn.AssocDataDecl
	p.parseBody("class declaration", func() {
		if p.peek().Kind == syn.TokType {
			atd := p.parseAssocTypeDecl()
			if atd != nil {
				assocTypes = append(assocTypes, *atd)
			}
		} else if p.peek().Kind == syn.TokData {
			add := p.parseAssocDataDecl()
			if add != nil {
				assocDataDecls = append(assocDataDecls, *add)
			}
		} else {
			mStart := p.peek().S.Start
			name := p.expectLower()
			p.expect(syn.TokColonColon)
			ty := p.parseType()
			methods = append(methods, syn.ClassMethod{Name: name, Type: ty, S: span.Span{Start: mStart, End: p.prevEnd()}})
		}
	})
	return methods, assocTypes, assocDataDecls
}

// parseAssocDataDecl parses an associated data family declaration in a class body:
//
//	data Name params :: Kind
func (p *Parser) parseAssocDataDecl() *syn.AssocDataDecl {
	start := p.peek().S.Start
	p.expect(syn.TokData)
	name := p.expectUpper()
	params := p.parseTyBinderList()
	p.expect(syn.TokColonColon)
	resultKind := p.parseKindExpr()
	return &syn.AssocDataDecl{
		Name:       name,
		Params:     params,
		ResultKind: resultKind,
		S:          span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseAssocTypeDecl parses an associated type declaration in a class body:
//
//	type Name params :: Kind
func (p *Parser) parseAssocTypeDecl() *syn.AssocTypeDecl {
	start := p.peek().S.Start
	p.expect(syn.TokType)
	name := p.expectUpper()
	params := p.parseTyBinderList()
	p.expect(syn.TokColonColon)
	resultKind, resultName, deps := p.parseResultKind()
	return &syn.AssocTypeDecl{
		Name:       name,
		Params:     params,
		ResultKind: resultKind,
		ResultName: resultName,
		Deps:       deps,
		S:          span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseInstanceDecl parses: instance [Constraint =>]* ClassName types { method := expr; ... }
// Supports curried constraints: instance Eq a => Eq b => Eq (Pair a b) { ... }
// Supports parenthesized constraints: instance (Eq a) => Eq (Maybe a) { ... }
func (p *Parser) parseInstanceDecl() *syn.DeclInstance {
	start := p.peek().S.Start
	p.expect(syn.TokInstance)

	var context []syn.TypeExpr

	// Loop: accumulate constraints until we find the actual class head.
	for {
		// Parenthesized constraint(s): (Eq a) => or (Eq a, Ord a) => ...
		if p.peek().Kind == syn.TokLParen {
			saved := p.pos
			savedDepth := p.depth
			savedErrLen := p.errors.Len()
			ty := p.parseTypeAtom() // parses (Eq a) or (Eq a, Ord a) as tuple
			if p.peek().Kind == syn.TokFatArrow {
				context = append(context, decomposeTupleConstraint(ty)...)
				p.advance() // consume =>
				continue
			}
			// Not a constraint — backtrack and discard phantom errors.
			p.pos = saved
			p.depth = savedDepth
			p.errors.Truncate(savedErrLen)
			break
		}

		if p.peek().Kind != syn.TokUpper {
			break
		}

		// Parse Upper + args, then check for =>
		nameStart := p.peek().S.Start
		firstName := p.peek().Text
		p.advance()

		var firstArgs []syn.TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != syn.TokLBrace && !p.atStmtBoundary() {
			firstArgs = append(firstArgs, p.parseTypeAtom())
		}

		if p.peek().Kind == syn.TokFatArrow {
			// This is a context constraint.
			var ctxExpr syn.TypeExpr = &syn.TyExprCon{Name: firstName, S: span.Span{Start: nameStart, End: p.prevEnd()}}
			for _, arg := range firstArgs {
				ctxExpr = &syn.TyExprApp{Fun: ctxExpr, Arg: arg, S: span.Span{Start: nameStart, End: arg.Span().End}}
			}
			context = append(context, ctxExpr)
			p.advance() // consume =>
			continue
		}

		// No =>, firstName IS the class name.
		methods, assocTypeDefs, assocDataDefs := p.parseInstBody()
		return &syn.DeclInstance{
			Context: context, ClassName: firstName, TypeArgs: firstArgs,
			Methods: methods, AssocTypeDefs: assocTypeDefs, AssocDataDefs: assocDataDefs,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Fallback: parse remaining class name + args
	className := p.expectUpper()
	var typeArgs []syn.TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != syn.TokLBrace && !p.atStmtBoundary() {
		typeArgs = append(typeArgs, p.parseTypeAtom())
	}
	methods, assocTypeDefs, assocDataDefs := p.parseInstBody()
	return &syn.DeclInstance{
		Context: context, ClassName: className, TypeArgs: typeArgs,
		Methods: methods, AssocTypeDefs: assocTypeDefs, AssocDataDefs: assocDataDefs,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseInstBody() ([]syn.InstMethod, []syn.AssocTypeDef, []syn.AssocDataDef) {
	p.expect(syn.TokLBrace)
	var methods []syn.InstMethod
	var assocTypeDefs []syn.AssocTypeDef
	var assocDataDefs []syn.AssocDataDef
	p.parseBody("instance declaration", func() {
		mStart := p.peek().S.Start
		if p.peek().Kind == syn.TokData {
			add := p.parseAssocDataDef()
			if add != nil {
				assocDataDefs = append(assocDataDefs, *add)
			}
		} else if p.peek().Kind == syn.TokType {
			atd := p.parseAssocTypeDef()
			if atd != nil {
				assocTypeDefs = append(assocTypeDefs, *atd)
			}
		} else if p.peek().Kind == syn.TokLower {
			name := p.expectLower()
			p.expect(syn.TokColonEq)
			expr := p.parseExpr()
			methods = append(methods, syn.InstMethod{Name: name, Expr: expr, S: span.Span{Start: mStart, End: p.prevEnd()}})
		}
	})
	return methods, assocTypeDefs, assocDataDefs
}

// parseAssocTypeDef parses an associated type definition in an instance body:
//
//	type Name patterns = syn.TypeExpr
func (p *Parser) parseAssocTypeDef() *syn.AssocTypeDef {
	start := p.peek().S.Start
	p.expect(syn.TokType)
	name := p.expectUpper()
	var patterns []syn.TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != syn.TokEqColon {
		patterns = append(patterns, p.parseTypeAtom())
	}
	p.expect(syn.TokEqColon)
	rhs := p.parseType()
	return &syn.AssocTypeDef{
		Name:     name,
		Patterns: patterns,
		RHS:      rhs,
		S:        span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseAssocDataDef parses an associated data family definition in an instance body:
//
//	data Name patterns =: Con fields | Con fields
func (p *Parser) parseAssocDataDef() *syn.AssocDataDef {
	start := p.peek().S.Start
	p.expect(syn.TokData)
	name := p.expectUpper()
	// Parse type patterns (same as type family equations).
	var patterns []syn.TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != syn.TokEqColon {
		patterns = append(patterns, p.parseTypeAtom())
	}
	p.expect(syn.TokEqColon)
	// Parse constructors: Con fields | Con fields | ...
	var cons []syn.DeclCon
	for {
		conStart := p.peek().S.Start
		conName := p.expectUpper()
		var fields []syn.TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != syn.TokPipe && p.peek().Kind != syn.TokSemicolon && p.peek().Kind != syn.TokRBrace {
			fields = append(fields, p.parseTypeAtom())
		}
		cons = append(cons, syn.DeclCon{Name: conName, Fields: fields, S: span.Span{Start: conStart, End: p.prevEnd()}})
		if p.peek().Kind == syn.TokPipe {
			p.advance()
			continue
		}
		break
	}
	return &syn.AssocDataDef{
		Name:     name,
		Patterns: patterns,
		Cons:     cons,
		S:        span.Span{Start: start, End: p.prevEnd()},
	}
}
