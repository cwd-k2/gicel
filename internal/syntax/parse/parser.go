package parse

import (
	"strconv"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// Fixity holds operator precedence and associativity.
type Fixity struct {
	Assoc Assoc
	Prec  int
}

// Parser is a Pratt parser for the surface language.
type Parser struct {
	tokens      []Token
	pos         int
	fixity      map[string]Fixity
	errors      *errs.Errors
	depth       int  // paren/brace nesting depth
	noBraceAtom bool // when true, { is not an atom start (inside case scrutinee)

	// Safety harness: prevent resource exhaustion on malformed input.
	recurseDepth    int // current recursion depth
	maxRecurseDepth int // limit (default 256)
	steps           int // advance() call count
	maxSteps        int // limit (default len(tokens) * 4)
	halted          bool
}

// NewParser creates a parser from a token stream.
func NewParser(tokens []Token, errors *errs.Errors) *Parser {
	maxSteps := len(tokens) * 4
	if maxSteps < 100 {
		maxSteps = 100
	}
	return &Parser{
		tokens:          tokens,
		pos:             0,
		fixity:          make(map[string]Fixity),
		errors:          errors,
		maxRecurseDepth: 256,
		maxSteps:        maxSteps,
	}
}

// AddFixity seeds the parser with additional fixity declarations (e.g. from imported modules).
func (p *Parser) AddFixity(fixity map[string]Fixity) {
	for op, f := range fixity {
		if _, exists := p.fixity[op]; !exists {
			p.fixity[op] = f
		}
	}
}

// ParseProgram parses a complete program.
func (p *Parser) ParseProgram() *AstProgram {
	// First pass: collect fixity declarations.
	p.collectFixity()
	p.pos = 0
	p.steps = 0
	p.halted = false

	// Parse imports first.
	var imports []DeclImport
	p.skipSemicolons()
	for p.peek().Kind == TokImport {
		imports = append(imports, p.parseImportDecl())
		p.skipSemicolons()
	}

	var decls []Decl
	for p.peek().Kind != TokEOF {
		d := p.parseDecl()
		if d != nil {
			decls = append(decls, d)
		}
		p.skipSemicolons()
	}
	return &AstProgram{Imports: imports, Decls: decls}
}

// ParseExpr parses a single expression.
func (p *Parser) ParseExpr() Expr {
	return p.parseExpr()
}

// ParseType parses a type expression.
func (p *Parser) ParseType() TypeExpr {
	return p.parseType()
}

// --- Declarations ---

func (p *Parser) collectFixity() {
	saved := p.pos
	for p.peek().Kind != TokEOF {
		if p.isFixityKeyword() {
			d := p.parseFixityDecl()
			if d != nil {
				p.fixity[d.Op] = Fixity{Assoc: d.Assoc, Prec: d.Prec}
			}
		} else {
			p.advance()
		}
	}
	p.pos = saved
}

func (p *Parser) parseDecl() Decl {
	switch {
	case p.peek().Kind == TokData:
		return p.parseDataDecl()
	case p.peek().Kind == TokType:
		return p.parseTypeDecl()
	case p.peek().Kind == TokClass:
		return p.parseClassDecl()
	case p.peek().Kind == TokInstance:
		return p.parseInstanceDecl()
	case p.isFixityKeyword():
		d := p.parseFixityDecl()
		return d
	case p.peek().Kind == TokLParen && p.isOperatorDeclStart():
		return p.parseOperatorDecl()
	case p.peek().Kind == TokLower:
		return p.parseNamedDecl()
	default:
		p.addError("expected declaration")
		p.advance()
		return nil
	}
}

func (p *Parser) parseImportDecl() DeclImport {
	start := p.peek().S.Start
	p.expect(TokImport)
	modTok := p.peek()
	modName := p.expectUpper()
	// Support dotted module names: Std.Num, Std.Str, etc.
	// Only adjacent dots (no whitespace) form module paths.
	for p.peek().Kind == TokDot && tokensAdjacent(modTok, p.peek()) {
		p.advance()
		part := p.expectUpper()
		modName = modName + "." + part
		modTok = p.tokens[p.pos-1] // update for next adjacency check
	}

	imp := DeclImport{ModuleName: modName}

	// Check for qualified import: import M as N
	if p.peek().Kind == TokLower && p.peek().Text == "as" {
		p.advance() // consume "as"
		imp.Alias = p.expectUpper()
		imp.S = span.Span{Start: start, End: p.prevEnd()}
		return imp
	}

	// Check for selective import: import M (name, T(..), C(A,B))
	if p.peek().Kind == TokLParen {
		imp.Names = p.parseImportList()
	}

	imp.S = span.Span{Start: start, End: p.prevEnd()}
	return imp
}

// parseImportList parses the parenthesized import name list: (name, T(..), (op), C(A,B))
func (p *Parser) parseImportList() []ImportName {
	p.expect(TokLParen)
	names := []ImportName{} // non-nil empty slice distinguishes import M () from import M
	for p.peek().Kind != TokRParen && p.peek().Kind != TokEOF {
		names = append(names, p.parseImportName())
		if p.peek().Kind == TokComma {
			p.advance()
		} else {
			break
		}
	}
	p.expect(TokRParen)
	return names
}

// parseImportName parses one entry in an import list.
//
//	lower            → value binding
//	(op)             → operator
//	Upper            → type/class bare
//	Upper (..)       → type/class with all subs
//	Upper (A, B)     → type/class with specific subs
func (p *Parser) parseImportName() ImportName {
	// Operator: (op)
	if p.peek().Kind == TokLParen {
		p.advance()
		var opName string
		if p.peek().Kind == TokOp {
			opName = p.peek().Text
			p.advance()
		} else if p.peek().Kind == TokDot {
			opName = "."
			p.advance()
		} else {
			p.addError("expected operator in import list")
			opName = "<error>"
		}
		p.expect(TokRParen)
		return ImportName{Name: opName}
	}

	// Value binding: lower
	if p.peek().Kind == TokLower {
		name := p.peek().Text
		p.advance()
		return ImportName{Name: name}
	}

	// Type/class: Upper [(..) | (A, B)]
	name := p.expectUpper()
	in := ImportName{Name: name}

	if p.peek().Kind == TokLParen {
		p.advance()
		in.HasSub = true
		if p.peek().Kind == TokDot && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TokDot {
			// (..)
			p.advance() // first .
			p.advance() // second .
			in.AllSubs = true
		} else if p.peek().Kind != TokRParen {
			// Explicit sub-list: (A, B, methodName, ...)
			for {
				var sub string
				if p.peek().Kind == TokUpper {
					sub = p.peek().Text
					p.advance()
				} else if p.peek().Kind == TokLower {
					sub = p.peek().Text
					p.advance()
				} else {
					p.addError("expected name in import sub-list")
					break
				}
				in.SubList = append(in.SubList, sub)
				if p.peek().Kind == TokComma {
					p.advance()
				} else {
					break
				}
			}
		}
		p.expect(TokRParen)
	}

	return in
}

func (p *Parser) parseDataDecl() *DeclData {
	start := p.peek().S.Start
	p.expect(TokData)
	name := p.expectUpper()
	var params []TyBinder
	for p.peek().Kind == TokLower || p.peek().Kind == TokLParen {
		if p.peek().Kind == TokLParen {
			// Kinded param: (name: Kind)
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			params = append(params, TyBinder{
				Name: pName,
				Kind: kind,
				S:    span.Span{Start: lp, End: p.prevEnd()},
			})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, TyBinder{Name: pName, S: pS})
		}
	}
	p.expect(TokColonEq)

	// GADT mode: `:= { ConName :: Type; ... }`
	if p.peek().Kind == TokLBrace {
		gadtCons := p.parseGADTCons()
		return &DeclData{
			Name:     name,
			Params:   params,
			GADTCons: gadtCons,
			S:        span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// ADT mode: `= Con1 fields | Con2 fields`
	var cons []DeclCon
	cons = append(cons, p.parseConDecl())
	for p.peek().Kind == TokPipe {
		p.advance()
		cons = append(cons, p.parseConDecl())
	}
	return &DeclData{
		Name:   name,
		Params: params,
		Cons:   cons,
		S:      span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseGADTCons parses GADT constructor declarations inside braces.
// Format: { ConName :: Type ; ConName :: Type ; ... }
func (p *Parser) parseGADTCons() []GADTConDecl {
	p.expect(TokLBrace)
	var cons []GADTConDecl
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		before := p.pos
		cStart := p.peek().S.Start
		cName := p.expectUpper()
		p.expect(TokColonColon)
		cType := p.parseType()
		cons = append(cons, GADTConDecl{
			Name: cName,
			Type: cType,
			S:    span.Span{Start: cStart, End: p.prevEnd()},
		})
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in GADT declaration")
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return cons
}

func (p *Parser) parseConDecl() DeclCon {
	start := p.peek().S.Start
	name := p.expectUpper()
	var fields []TypeExpr
	for p.isTypeAtomStart() && !p.atDeclBoundary() {
		fields = append(fields, p.parseTypeAtom())
	}
	return DeclCon{Name: name, Fields: fields, S: span.Span{Start: start, End: p.prevEnd()}}
}

// parseTypeDecl parses either a type alias or a type family declaration.
//
//	type Name params = TypeExpr              → type alias
//	type Name params :: Kind = { equations } → type family
func (p *Parser) parseTypeDecl() Decl {
	start := p.peek().S.Start
	p.expect(TokType)
	name := p.expectUpper()
	params := p.parseTyBinderList()

	if p.peek().Kind == TokColonColon {
		// Type family declaration.
		return p.parseTypeFamilyBody(name, params, start)
	}

	// Type alias.
	p.expect(TokColonEq)
	body := p.parseType()
	return &DeclTypeAlias{
		Name:   name,
		Params: params,
		Body:   body,
		S:      span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTyBinderList parses a sequence of type binders: bare or kinded.
func (p *Parser) parseTyBinderList() []TyBinder {
	var params []TyBinder
	for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
		if p.peek().Kind == TokLParen {
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			params = append(params, TyBinder{Name: pName, Kind: kind, S: span.Span{Start: lp, End: p.prevEnd()}})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, TyBinder{Name: pName, S: pS})
		}
	}
	return params
}

// parseTypeFamilyBody parses the body of a type family declaration after `::`.
//
//	:: Kind = { equations }
//	:: (r: Kind) | deps = { equations }
func (p *Parser) parseTypeFamilyBody(name string, params []TyBinder, start span.Pos) *DeclTypeFamily {
	p.expect(TokColonColon)
	resultKind, resultName, deps := p.parseResultKind()

	p.expect(TokColonEq)
	equations := p.parseTypeFamilyEquations(name)

	return &DeclTypeFamily{
		Name:       name,
		Params:     params,
		ResultKind: resultKind,
		ResultName: resultName,
		Deps:       deps,
		Equations:  equations,
		S:          span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseResultKind parses either a plain kind or an injective result kind.
//
//	Type                            → (KindExprType, "", nil)
//	(r: Type) | r -> a             → (KindExprType, "r", [{From:"r", To:["a"]}])
func (p *Parser) parseResultKind() (KindExpr, string, []FunDep) {
	// Check for injective form: (name: Kind) | deps
	if p.peek().Kind == TokLParen && p.isInjectiveResult() {
		p.advance() // consume (
		resultName := p.expectLower()
		p.expect(TokColon)
		kind := p.parseKindExpr()
		p.expect(TokRParen)

		// Parse | deps
		p.expect(TokPipe)
		deps := p.parseFunDepList()
		return kind, resultName, deps
	}

	// Plain kind expression.
	kind := p.parseKindExpr()
	return kind, "", nil
}

// isInjectiveResult checks if the tokens at current position form
// (lower: Kind) | ..., which indicates an injective result kind.
func (p *Parser) isInjectiveResult() bool {
	if p.pos+3 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == TokLower && p.tokens[p.pos+2].Kind == TokColon
}

// parseTypeFamilyEquations parses the equation block { Name Pat* = RHS; ... }.
func (p *Parser) parseTypeFamilyEquations(familyName string) []TFEquation {
	p.expect(TokLBrace)
	var equations []TFEquation
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		before := p.pos
		eqStart := p.peek().S.Start
		eqName := p.expectUpper()
		// Parse LHS type patterns.
		var patterns []TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != TokEqColon {
			patterns = append(patterns, p.parseTypeAtom())
		}
		p.expect(TokEqColon)
		rhs := p.parseType()
		equations = append(equations, TFEquation{
			Name:     eqName,
			Patterns: patterns,
			RHS:      rhs,
			S:        span.Span{Start: eqStart, End: p.prevEnd()},
		})
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in type family declaration")
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return equations
}

func (p *Parser) parseFixityDecl() *DeclFixity {
	start := p.peek().S.Start
	var assoc Assoc
	switch p.peek().Kind {
	case TokInfixl:
		assoc = AssocLeft
	case TokInfixr:
		assoc = AssocRight
	case TokInfixn:
		assoc = AssocNone
	}
	p.advance()
	prec := 9
	if p.peek().Kind == TokIntLit {
		n, err := strconv.Atoi(p.peek().Text)
		if err == nil {
			prec = n
		}
		p.advance()
	}
	var op string
	if p.peek().Kind == TokOp {
		op = p.peek().Text
		p.advance()
	} else if p.peek().Kind == TokDot {
		op = "."
		p.advance()
	} else {
		p.addError("expected operator in fixity declaration")
		return nil
	}
	return &DeclFixity{
		Assoc: assoc, Prec: prec, Op: op,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseClassDecl parses: class [Constraint =>] ClassName params { method :: Type; ... }
func (p *Parser) parseClassDecl() *DeclClass {
	start := p.peek().S.Start
	p.expect(TokClass)

	// Parse the head: either "ClassName params" or "Constraint => ClassName params".
	// Strategy: parse type applications, check for => to detect superclass.
	var supers []TypeExpr

	// Tuple-style superclass constraints: class (C1 a, C2 b) => ClassName params { ... }
	if p.peek().Kind == TokLParen && !p.isClassKindedBinder() {
		p.advance() // consume (
		for {
			conStart := p.peek().S.Start
			conName := p.expectUpper()
			var conExpr TypeExpr = &TyExprCon{Name: conName, S: span.Span{Start: conStart, End: p.prevEnd()}}
			for p.peek().Kind == TokLower {
				tok := p.peek()
				p.advance()
				arg := &TyExprVar{Name: tok.Text, S: tok.S}
				conExpr = &TyExprApp{Fun: conExpr, Arg: arg, S: span.Span{Start: conStart, End: tok.S.End}}
			}
			supers = append(supers, conExpr)
			if p.peek().Kind != TokComma {
				break
			}
			p.advance() // consume ,
		}
		p.expect(TokRParen)
		p.expect(TokFatArrow)
		// Parse class name + params
		className := p.expectUpper()
		var params []TyBinder
		for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
			if p.peek().Kind == TokLParen {
				lp := p.peek().S.Start
				p.advance()
				name := p.expectLower()
				p.expect(TokColon)
				kind := p.parseKindExpr()
				p.expect(TokRParen)
				params = append(params, TyBinder{Name: name, Kind: kind, S: span.Span{Start: lp, End: p.prevEnd()}})
			} else {
				tok := p.peek()
				p.advance()
				params = append(params, TyBinder{Name: tok.Text, S: tok.S})
			}
		}
		funDeps := p.parseClassFunDeps()
		methods, assocTypes, assocDataDecls := p.parseClassBody()
		return &DeclClass{
			Supers: supers, Name: className, TyParams: params, FunDeps: funDeps,
			Methods: methods, AssocTypes: assocTypes, AssocDataDecls: assocDataDecls,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	firstName := p.expectUpper()
	var firstArgs []TypeExpr
	// Parse class params: either bare lowercase vars or kinded binders (v: Kind).
	for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
		if p.peek().Kind == TokLParen {
			// Kinded class param: (m: Kind)
			lp := p.peek().S.Start
			p.advance()
			name := p.expectLower()
			p.expect(TokColon)
			kind := p.parseKindExpr()
			p.expect(TokRParen)
			firstArgs = append(firstArgs, &TyExprVar{
				Name: name,
				S:    span.Span{Start: lp, End: p.prevEnd()},
				Kind: kind,
			})
		} else {
			tok := p.peek()
			p.advance()
			firstArgs = append(firstArgs, &TyExprVar{Name: tok.Text, S: tok.S})
		}
	}

	if p.peek().Kind == TokFatArrow {
		// What we parsed is a superclass constraint.
		var superExpr TypeExpr = &TyExprCon{Name: firstName, S: span.Span{Start: start, End: p.prevEnd()}}
		for _, arg := range firstArgs {
			superExpr = &TyExprApp{Fun: superExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
		}
		supers = append(supers, superExpr)
		p.advance() // consume =>

		// Support multiple superclass constraints: Super1 a => Super2 a => ... => ClassName params
		for {
			nextName := p.expectUpper()
			var nextArgs []TypeExpr
			for p.peek().Kind == TokLower || (p.peek().Kind == TokLParen && p.isClassKindedBinder()) {
				if p.peek().Kind == TokLParen {
					lp := p.peek().S.Start
					p.advance()
					name := p.expectLower()
					p.expect(TokColon)
					kind := p.parseKindExpr()
					p.expect(TokRParen)
					nextArgs = append(nextArgs, &TyExprVar{
						Name: name,
						S:    span.Span{Start: lp, End: p.prevEnd()},
						Kind: kind,
					})
				} else {
					tok := p.peek()
					p.advance()
					nextArgs = append(nextArgs, &TyExprVar{Name: tok.Text, S: tok.S})
				}
			}
			if p.peek().Kind == TokFatArrow {
				// Another superclass constraint.
				var nextExpr TypeExpr = &TyExprCon{Name: nextName, S: span.Span{Start: start, End: p.prevEnd()}}
				for _, arg := range nextArgs {
					nextExpr = &TyExprApp{Fun: nextExpr, Arg: arg, S: span.Span{Start: start, End: arg.Span().End}}
				}
				supers = append(supers, nextExpr)
				p.advance() // consume =>
				continue
			}
			// This is the actual class name.
			var params []TyBinder
			for _, arg := range nextArgs {
				v := arg.(*TyExprVar)
				params = append(params, TyBinder{Name: v.Name, Kind: v.Kind, S: v.S})
			}
			funDeps := p.parseClassFunDeps()
			methods, assocTypes, assocDataDecls := p.parseClassBody()
			return &DeclClass{
				Supers: supers, Name: nextName, TyParams: params, FunDeps: funDeps,
				Methods: methods, AssocTypes: assocTypes, AssocDataDecls: assocDataDecls,
				S: span.Span{Start: start, End: p.prevEnd()},
			}
		}
	}

	// No =>, so firstName is the class name, firstArgs are params.
	var params []TyBinder
	for _, arg := range firstArgs {
		v := arg.(*TyExprVar)
		params = append(params, TyBinder{Name: v.Name, Kind: v.Kind, S: v.S})
	}
	funDeps := p.parseClassFunDeps()
	methods, assocTypes, assocDataDecls := p.parseClassBody()
	return &DeclClass{
		Name: firstName, TyParams: params, FunDeps: funDeps,
		Methods: methods, AssocTypes: assocTypes, AssocDataDecls: assocDataDecls,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseClassFunDeps parses optional functional dependencies: | a -> b, c -> d
func (p *Parser) parseClassFunDeps() []FunDep {
	if p.peek().Kind != TokPipe {
		return nil
	}
	p.advance() // consume |
	return p.parseFunDepList()
}

// parseFunDepList parses a comma-separated list of functional dependencies: a -> b, c -> d
func (p *Parser) parseFunDepList() []FunDep {
	var deps []FunDep
	for {
		from := p.expectLower()
		p.expect(TokEqColon)
		var to []string
		for p.peek().Kind == TokLower {
			to = append(to, p.expectLower())
		}
		if len(to) == 0 {
			p.addError("functional dependency requires at least one determined parameter after '=:'")
		}
		deps = append(deps, FunDep{From: from, To: to})
		if p.peek().Kind == TokComma {
			p.advance()
			continue
		}
		break
	}
	return deps
}

func (p *Parser) parseClassBody() ([]ClassMethod, []AssocTypeDecl, []AssocDataDecl) {
	p.expect(TokLBrace)
	var methods []ClassMethod
	var assocTypes []AssocTypeDecl
	var assocDataDecls []AssocDataDecl
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		before := p.pos
		if p.peek().Kind == TokType {
			atd := p.parseAssocTypeDecl()
			if atd != nil {
				assocTypes = append(assocTypes, *atd)
			}
		} else if p.peek().Kind == TokData {
			add := p.parseAssocDataDecl()
			if add != nil {
				assocDataDecls = append(assocDataDecls, *add)
			}
		} else {
			mStart := p.peek().S.Start
			name := p.expectLower()
			p.expect(TokColonColon)
			ty := p.parseType()
			methods = append(methods, ClassMethod{Name: name, Type: ty, S: span.Span{Start: mStart, End: p.prevEnd()}})
		}
		if p.peek().Kind == TokSemicolon {
			p.advance()
		} else if p.pos == before {
			p.addError("unexpected token in class declaration")
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return methods, assocTypes, assocDataDecls
}

// parseAssocDataDecl parses an associated data family declaration in a class body:
//
//	data Name params :: Kind
func (p *Parser) parseAssocDataDecl() *AssocDataDecl {
	start := p.peek().S.Start
	p.expect(TokData)
	name := p.expectUpper()
	params := p.parseTyBinderList()
	p.expect(TokColonColon)
	resultKind := p.parseKindExpr()
	return &AssocDataDecl{
		Name:       name,
		Params:     params,
		ResultKind: resultKind,
		S:          span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseAssocTypeDecl parses an associated type declaration in a class body:
//
//	type Name params :: Kind
func (p *Parser) parseAssocTypeDecl() *AssocTypeDecl {
	start := p.peek().S.Start
	p.expect(TokType)
	name := p.expectUpper()
	params := p.parseTyBinderList()
	p.expect(TokColonColon)
	resultKind, resultName, deps := p.parseResultKind()
	return &AssocTypeDecl{
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
func (p *Parser) parseInstanceDecl() *DeclInstance {
	start := p.peek().S.Start
	p.expect(TokInstance)

	var context []TypeExpr

	// Loop: accumulate constraints until we find the actual class head.
	for {
		// Parenthesized constraint(s): (Eq a) => or (Eq a, Ord a) => ...
		if p.peek().Kind == TokLParen {
			saved := p.pos
			savedDepth := p.depth
			savedErrLen := p.errors.Len()
			ty := p.parseTypeAtom() // parses (Eq a) or (Eq a, Ord a) as tuple
			if p.peek().Kind == TokFatArrow {
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

		if p.peek().Kind != TokUpper {
			break
		}

		// Parse Upper + args, then check for =>
		nameStart := p.peek().S.Start
		firstName := p.peek().Text
		p.advance()

		var firstArgs []TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != TokLBrace && !p.atDeclBoundary() {
			firstArgs = append(firstArgs, p.parseTypeAtom())
		}

		if p.peek().Kind == TokFatArrow {
			// This is a context constraint.
			var ctxExpr TypeExpr = &TyExprCon{Name: firstName, S: span.Span{Start: nameStart, End: p.prevEnd()}}
			for _, arg := range firstArgs {
				ctxExpr = &TyExprApp{Fun: ctxExpr, Arg: arg, S: span.Span{Start: nameStart, End: arg.Span().End}}
			}
			context = append(context, ctxExpr)
			p.advance() // consume =>
			continue
		}

		// No =>, firstName IS the class name.
		methods, assocTypeDefs, assocDataDefs := p.parseInstBody()
		return &DeclInstance{
			Context: context, ClassName: firstName, TypeArgs: firstArgs,
			Methods: methods, AssocTypeDefs: assocTypeDefs, AssocDataDefs: assocDataDefs,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Fallback: parse remaining class name + args
	className := p.expectUpper()
	var typeArgs []TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != TokLBrace && !p.atDeclBoundary() {
		typeArgs = append(typeArgs, p.parseTypeAtom())
	}
	methods, assocTypeDefs, assocDataDefs := p.parseInstBody()
	return &DeclInstance{
		Context: context, ClassName: className, TypeArgs: typeArgs,
		Methods: methods, AssocTypeDefs: assocTypeDefs, AssocDataDefs: assocDataDefs,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseInstBody() ([]InstMethod, []AssocTypeDef, []AssocDataDef) {
	p.expect(TokLBrace)
	var methods []InstMethod
	var assocTypeDefs []AssocTypeDef
	var assocDataDefs []AssocDataDef
	for p.peek().Kind != TokRBrace && p.peek().Kind != TokEOF {
		mStart := p.peek().S.Start
		if p.peek().Kind == TokData {
			// Associated data family definition: data Name patterns = Con ... | Con ...
			add := p.parseAssocDataDef()
			if add != nil {
				assocDataDefs = append(assocDataDefs, *add)
			}
			if p.peek().Kind == TokSemicolon {
				p.advance()
			}
			continue
		} else if p.peek().Kind == TokType {
			// Associated type definition: type Name patterns = TypeExpr
			atd := p.parseAssocTypeDef()
			if atd != nil {
				assocTypeDefs = append(assocTypeDefs, *atd)
			}
		} else if p.peek().Kind == TokLower {
			name := p.expectLower()
			p.expect(TokColonEq)
			expr := p.parseExpr()
			methods = append(methods, InstMethod{Name: name, Expr: expr, S: span.Span{Start: mStart, End: p.prevEnd()}})
		} else {
			p.addError("expected method name or 'type' in instance declaration")
			p.advance()
			continue
		}
		if p.peek().Kind == TokSemicolon {
			p.advance()
		}
	}
	p.expect(TokRBrace)
	return methods, assocTypeDefs, assocDataDefs
}

// parseAssocTypeDef parses an associated type definition in an instance body:
//
//	type Name patterns = TypeExpr
func (p *Parser) parseAssocTypeDef() *AssocTypeDef {
	start := p.peek().S.Start
	p.expect(TokType)
	name := p.expectUpper()
	var patterns []TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != TokEqColon {
		patterns = append(patterns, p.parseTypeAtom())
	}
	p.expect(TokEqColon)
	rhs := p.parseType()
	return &AssocTypeDef{
		Name:     name,
		Patterns: patterns,
		RHS:      rhs,
		S:        span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseAssocDataDef parses an associated data family definition in an instance body:
//
//	data Name patterns =: Con fields | Con fields
func (p *Parser) parseAssocDataDef() *AssocDataDef {
	start := p.peek().S.Start
	p.expect(TokData)
	name := p.expectUpper()
	// Parse type patterns (same as type family equations).
	var patterns []TypeExpr
	for p.isTypeAtomStart() && p.peek().Kind != TokEqColon {
		patterns = append(patterns, p.parseTypeAtom())
	}
	p.expect(TokEqColon)
	// Parse constructors: Con fields | Con fields | ...
	var cons []DeclCon
	for {
		conStart := p.peek().S.Start
		conName := p.expectUpper()
		var fields []TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != TokPipe && p.peek().Kind != TokSemicolon && p.peek().Kind != TokRBrace {
			fields = append(fields, p.parseTypeAtom())
		}
		cons = append(cons, DeclCon{Name: conName, Fields: fields, S: span.Span{Start: conStart, End: p.prevEnd()}})
		if p.peek().Kind == TokPipe {
			p.advance()
			continue
		}
		break
	}
	return &AssocDataDef{
		Name:     name,
		Patterns: patterns,
		Cons:     cons,
		S:        span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) isOperatorDeclStart() bool {
	// Check for (operator) pattern: ( op ) — TokDot also valid as operator.
	return p.pos+2 < len(p.tokens) &&
		(p.tokens[p.pos+1].Kind == TokOp || p.tokens[p.pos+1].Kind == TokDot) &&
		p.tokens[p.pos+2].Kind == TokRParen
}

func (p *Parser) parseOperatorDecl() Decl {
	start := p.expect(TokLParen)
	// Accept both TokOp and TokDot as operator names.
	var op Token
	if p.peek().Kind == TokDot {
		op = p.peek()
		op.Text = "."
		p.advance()
	} else {
		op = p.expect(TokOp)
	}
	p.expect(TokRParen)
	switch p.peek().Kind {
	case TokColonColon:
		p.advance()
		ty := p.parseType()
		return &DeclTypeAnn{Name: op.Text, Type: ty, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	case TokColonEq:
		p.advance()
		expr := p.parseExpr()
		return &DeclValueDef{Name: op.Text, Expr: expr, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	default:
		p.addError("expected :: or := after operator name")
		return nil
	}
}

func (p *Parser) parseNamedDecl() Decl {
	start := p.peek().S.Start
	name := p.peek().Text
	p.advance()

	switch p.peek().Kind {
	case TokColonColon:
		// Type annotation: f :: T
		p.advance()
		ty := p.parseType()
		return &DeclTypeAnn{
			Name: name, Type: ty,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	case TokColonEq:
		// Value definition: f := e
		p.advance()
		expr := p.parseExpr()
		return &DeclValueDef{
			Name: name, Expr: expr,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	default:
		p.addError("expected :: or := after name")
		return nil
	}
}

// Expression and pattern parsing is in parse_expr.go.

// --- Helpers ---

func (p *Parser) skipSemicolons() {
	for p.peek().Kind == TokSemicolon {
		p.advance()
	}
}

func (p *Parser) peek() Token {
	if p.halted || p.pos >= len(p.tokens) {
		return Token{Kind: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	if p.halted {
		return Token{Kind: TokEOF}
	}
	p.steps++
	if p.steps > p.maxSteps {
		p.halt("parser step limit exceeded")
		return Token{Kind: TokEOF}
	}
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	switch tok.Kind {
	case TokLParen, TokLBrace, TokLBracket:
		p.depth++
	case TokRParen, TokRBrace, TokRBracket:
		if p.depth > 0 {
			p.depth--
		}
	}
	return tok
}

// enterRecurse increments recursion depth and returns false if limit exceeded.
func (p *Parser) enterRecurse() bool {
	if p.halted {
		return false
	}
	p.recurseDepth++
	if p.recurseDepth > p.maxRecurseDepth {
		p.halt("parser recursion depth limit exceeded")
		return false
	}
	return true
}

func (p *Parser) leaveRecurse() {
	p.recurseDepth--
}

func (p *Parser) halt(msg string) {
	if !p.halted {
		p.halted = true
		p.addError(msg)
	}
}

func (p *Parser) expect(kind TokenKind) Token {
	tok := p.peek()
	if tok.Kind != kind {
		p.addError("expected " + kind.String())
		if tok.Kind != TokEOF {
			p.advance() // skip unexpected token to prevent parser stalling
		}
		return tok
	}
	return p.advance()
}

func (p *Parser) expectUpper() string {
	tok := p.peek()
	if tok.Kind != TokUpper {
		p.addError("expected uppercase identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) expectLower() string {
	tok := p.peek()
	if tok.Kind != TokLower {
		p.addError("expected identifier")
		return "<error>"
	}
	p.advance()
	return tok.Text
}

func (p *Parser) prevEnd() span.Pos {
	if p.pos > 0 && p.pos <= len(p.tokens) {
		return p.tokens[p.pos-1].S.End
	}
	return span.Pos(p.pos)
}

// atDeclBoundary returns true if the next token is across a newline boundary
// at the top level and could start a new declaration. This prevents greedy
// consumption of tokens that belong to subsequent declarations.
func (p *Parser) atDeclBoundary() bool {
	if p.depth > 0 {
		return false
	}
	tok := p.peek()
	if !tok.NewlineBefore {
		return false
	}
	switch tok.Kind {
	case TokLower, TokUpper, TokData, TokType, TokInfixl, TokInfixr, TokInfixn, TokClass, TokInstance, TokImport:
		return true
	case TokLParen:
		// (op) declaration pattern
		return p.isOperatorDeclStart()
	}
	return false
}

func (p *Parser) isTypeAtomStart() bool {
	if p.atDeclBoundary() {
		return false
	}
	k := p.peek().Kind
	return k == TokLower || k == TokUpper || k == TokLParen || k == TokLBrace || k == TokUnderscore
}

// isClassKindedBinder checks if the next tokens form (name: Kind) pattern.
func (p *Parser) isClassKindedBinder() bool {
	// Current is '(', check if followed by lower: Kind )
	if p.pos+2 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == TokLower && p.tokens[p.pos+2].Kind == TokColon
}

// decomposeTupleConstraint extracts individual constraints from a parsed type.
// (Eq a)       → TyExprParen → [Eq a]
// (Eq a, Ord a) → tuple Record → [Eq a, Ord a]
// other        → [other]
func decomposeTupleConstraint(ty TypeExpr) []TypeExpr {
	if paren, ok := ty.(*TyExprParen); ok {
		return []TypeExpr{paren.Inner}
	}
	// Tuple: TyExprApp { Fun: TyExprCon{Record}, Arg: TyExprRow{Fields: ...} }
	if app, ok := ty.(*TyExprApp); ok {
		if con, ok := app.Fun.(*TyExprCon); ok && con.Name == "Record" {
			if row, ok := app.Arg.(*TyExprRow); ok && len(row.Fields) > 0 {
				cs := make([]TypeExpr, len(row.Fields))
				for i, f := range row.Fields {
					cs[i] = f.Type
				}
				return cs
			}
		}
	}
	return []TypeExpr{ty}
}

func (p *Parser) isFixityKeyword() bool {
	k := p.peek().Kind
	return k == TokInfixl || k == TokInfixr || k == TokInfixn
}

func (p *Parser) lookupFixity(op string) Fixity {
	if f, ok := p.fixity[op]; ok {
		return f
	}
	return Fixity{Assoc: AssocLeft, Prec: 9} // default
}

// tokensAdjacent checks if two tokens have no whitespace between them.
func tokensAdjacent(a, b Token) bool {
	return a.S.End == b.S.Start
}

func (p *Parser) addError(msg string) {
	tok := p.peek()
	p.errors.Add(&errs.Error{
		Code:    errs.ErrParseSyntax,
		Phase:   errs.PhaseParse,
		Span:    tok.S,
		Message: msg,
	})
}
