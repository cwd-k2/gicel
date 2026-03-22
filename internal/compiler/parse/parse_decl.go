package parse

import (
	"strconv"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// --- Declarations ---

func (p *Parser) collectFixity() {
	saved := p.pos
	for p.peek().Kind != syn.TokEOF {
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

// parseDecl dispatches on the leading keyword to the appropriate declaration parser.
//
// Unified syntax declarations:
//
//	data Name [:: Kind] := TypeExpr;
//	type Name [:: Kind] := TypeExpr;
//	impl [name ::] TypeExpr := Expr;
//	name :: Type;
//	name := Expr;
//	infixl/infixr/infixn N op;
//	import Module;
func (p *Parser) parseDecl() syn.Decl {
	switch {
	case p.peek().Kind == syn.TokData:
		return p.parseDataDecl()
	case p.peek().Kind == syn.TokType:
		return p.parseTypeDecl()
	case p.peek().Kind == syn.TokImpl:
		return p.parseImplDecl()
	case p.isFixityKeyword():
		return p.parseFixityDecl()
	case p.peek().Kind == syn.TokLParen && p.isOperatorDeclStart():
		return p.parseOperatorDecl()
	case p.peek().Kind == syn.TokLower:
		return p.parseNamedDecl()
	default:
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected declaration")
		p.advance()
		return nil
	}
}

// parseDataDecl parses a nominal type declaration.
//
// New format:
//
//	data Name [:: Kind] := Body;
//
// Legacy class format (detected by params before { or =>):
//
//	data Name params { methods }   (was: class Name params { ... })
//	data Name := \params. Ctx => { methods }  (superclass form)
//
// Legacy ADT format (detected by params before :=):
//
//	data Name params := Con1 | Con2 ...
//
// All forms produce DeclData with Body as a TypeExpr.
func (p *Parser) parseDataDecl() *syn.DeclData {
	start := p.peek().S.Start
	p.expect(syn.TokData)
	name := p.expectUpper()

	// Detect legacy formats.
	// If next token is lowercase or kinded-binder "(", params are present.
	if p.peek().Kind == syn.TokLower || (p.peek().Kind == syn.TokLParen && p.isKindedBinder()) {
		return p.parseDataDeclLegacy(name, start)
	}

	// Check for superclass form: data Name := \params. Ctx => { ... }
	// (already handled by parseType via TyExprQual)

	var kindAnn syn.KindExpr
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn = p.parseKindExpr()
	}

	p.expect(syn.TokColonEq)
	body := p.parseType()

	return &syn.DeclData{
		Name:    name,
		KindAnn: kindAnn,
		Body:    body,
		S:       span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseDataDeclLegacy handles data declarations with LHS parameters.
// Supports both ADT format (Con1 | Con2) and class format ({ methods }).
func (p *Parser) parseDataDeclLegacy(name string, start span.Pos) *syn.DeclData {
	params := p.parseTyBinderList()

	// Check for class format: params followed by { (no :=)
	if p.peek().Kind == syn.TokLBrace {
		// Class-like: data Name params { methods }
		row := p.parseRowType()
		var body syn.TypeExpr = row
		if len(params) > 0 {
			body = &syn.TyExprForall{Binders: params, Body: row, S: span.Span{Start: start, End: p.prevEnd()}}
		}
		return &syn.DeclData{Name: name, Body: body, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// Check for superclass: params followed by => then {
	// e.g., data Ord := \a. Eq a => { ... }  — but this is new format.
	// Legacy: (Eq a) => Name params { ... }  — already converted by migration tool
	// to: data Name := \params. Eq a => { ... }

	p.expect(syn.TokColonEq)

	// ADT format: Con1 | Con2 fields
	if p.peek().Kind == syn.TokUpper {
		body := p.parseADTConsAsRow(params, start)
		return &syn.DeclData{Name: name, Body: body, S: span.Span{Start: start, End: p.prevEnd()}}
	}

	// Otherwise, parse as type expression (new format with params on LHS — transitional)
	inner := p.parseType()
	var body syn.TypeExpr = inner
	if len(params) > 0 {
		body = &syn.TyExprForall{Binders: params, Body: inner, S: span.Span{Start: start, End: p.prevEnd()}}
	}
	return &syn.DeclData{Name: name, Body: body, S: span.Span{Start: start, End: p.prevEnd()}}
}

// parseADTConsAsRow parses ADT constructors (Con1 fields | Con2 fields)
// and synthesizes a TyExprRow with constructor entries.
func (p *Parser) parseADTConsAsRow(params []syn.TyBinder, start span.Pos) syn.TypeExpr {
	type adtCon struct {
		name   string
		fields []syn.TypeExpr
		s      span.Span
	}

	var cons []adtCon
	// Parse first constructor
	for {
		cStart := p.peek().S.Start
		cName := p.expectUpper()
		var fields []syn.TypeExpr
		for p.isTypeAtomStart() && !p.atStmtBoundary() && p.peek().Kind != syn.TokPipe {
			fields = append(fields, p.parseTypeAtom())
		}
		cons = append(cons, adtCon{name: cName, fields: fields, s: span.Span{Start: cStart, End: p.prevEnd()}})
		if p.peek().Kind == syn.TokPipe {
			p.advance()
		} else {
			break
		}
	}

	// Convert to row fields.
	rowFields := make([]syn.TyRowField, len(cons))
	for i, c := range cons {
		var ty syn.TypeExpr
		switch len(c.fields) {
		case 0:
			// Nullary: Con → Con: ()
			ty = &syn.TyExprApp{
				Fun: &syn.TyExprCon{Name: "Record", S: c.s},
				Arg: &syn.TyExprRow{S: c.s},
				S:   c.s,
			}
		case 1:
			ty = c.fields[0]
		default:
			// Multi-field: synthesize tuple (f1, f2, ...) → Record { _1: f1, _2: f2 }
			tupleFields := make([]syn.TyRowField, len(c.fields))
			for j, f := range c.fields {
				tupleFields[j] = syn.TyRowField{
					Label: syn.TupleLabel(j + 1),
					Type:  f,
					S:     f.Span(),
				}
			}
			ty = &syn.TyExprApp{
				Fun: &syn.TyExprCon{Name: "Record", S: c.s},
				Arg: &syn.TyExprRow{Fields: tupleFields, S: c.s},
				S:   c.s,
			}
		}
		rowFields[i] = syn.TyRowField{Label: c.name, Type: ty, S: c.s}
	}

	row := &syn.TyExprRow{Fields: rowFields, S: span.Span{Start: start, End: p.prevEnd()}}
	if len(params) > 0 {
		return &syn.TyExprForall{Binders: params, Body: row, S: span.Span{Start: start, End: p.prevEnd()}}
	}
	return row
}

// parseTypeDecl parses a structural type declaration.
//
// New format:
//
//	type Name [:: Kind] := Body;
//
// Legacy equation format (type families):
//
//	type Name params :: Kind := { Name Pats =: RHS; ... }
//
// Both produce DeclTypeAlias. The legacy format desugars params into
// a lambda prefix and equations into TyExprCase alternatives.
func (p *Parser) parseTypeDecl() *syn.DeclTypeAlias {
	start := p.peek().S.Start
	p.expect(syn.TokType)
	name := p.expectUpper()

	// Detect legacy type family format: name followed by params (lowercase or "(").
	if p.peek().Kind == syn.TokLower || (p.peek().Kind == syn.TokLParen && p.isKindedBinder()) {
		return p.parseTypeFamilyLegacy(name, start)
	}

	var kindAnn syn.KindExpr
	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn = p.parseKindExpr()
	}

	p.expect(syn.TokColonEq)
	body := p.parseType()

	return &syn.DeclTypeAlias{
		Name:    name,
		KindAnn: kindAnn,
		Body:    body,
		S:       span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTypeFamilyLegacy parses the legacy type family equation format
// and desugars it into a DeclTypeAlias with lambda + case body.
//
//	type Elem (c: Type) :: Type := { Elem (List a) =: a }
//	→ DeclTypeAlias { Name: "Elem", Body: \(c: Type). case c { List a => a } }
func (p *Parser) parseTypeFamilyLegacy(name string, start span.Pos) *syn.DeclTypeAlias {
	params := p.parseTyBinderList()

	var kindAnn syn.KindExpr
	var resultName string
	var deps []syn.TyRowTypeDecl // reuse for injectivity deps (unused but parsed)
	_ = deps

	if p.peek().Kind == syn.TokColonColon {
		p.advance()
		kindAnn, resultName = p.parseResultKindLegacy()
	}

	p.expect(syn.TokColonEq)

	// If body doesn't start with {, it's a plain type alias with params, not a type family.
	if p.peek().Kind != syn.TokLBrace {
		inner := p.parseType()
		var body syn.TypeExpr = inner
		if len(params) > 0 {
			body = &syn.TyExprForall{Binders: params, Body: inner, S: span.Span{Start: start, End: p.prevEnd()}}
		}
		_ = resultName
		return &syn.DeclTypeAlias{
			Name:    name,
			KindAnn: kindAnn,
			Body:    body,
			S:       span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Parse equations: { Name Pats =: RHS; ... }
	openTok := p.expect(syn.TokLBrace)
	var alts []syn.TyAlt
	p.parseBody("type family declaration", openTok.S, func() {
		eqStart := p.peek().S.Start
		_ = p.expectUpper() // family name (repeated, skip)
		var patterns []syn.TypeExpr
		for p.isTypeAtomStart() && p.peek().Kind != syn.TokEqColon {
			patterns = append(patterns, p.parseTypeAtom())
		}
		p.expect(syn.TokEqColon)
		rhs := p.parseType()
		// Build pattern: for single param, use the pattern directly.
		// For multi-param, build a tuple pattern.
		var pat syn.TypeExpr
		if len(patterns) == 1 {
			pat = patterns[0]
		} else {
			// Multi-param: synthesize application (approximate)
			pat = patterns[0]
			for _, extra := range patterns[1:] {
				pat = &syn.TyExprApp{Fun: pat, Arg: extra, S: span.Span{Start: eqStart, End: p.prevEnd()}}
			}
		}
		alts = append(alts, syn.TyAlt{
			Pattern: pat,
			Body:    rhs,
			S:       span.Span{Start: eqStart, End: p.prevEnd()},
		})
	})

	// Build body: \params. case <first_param> { alts }
	var scrutinee syn.TypeExpr
	if len(params) > 0 {
		scrutinee = &syn.TyExprVar{Name: params[0].Name, S: span.Span{Start: start, End: start}}
	} else {
		scrutinee = &syn.TyExprCon{Name: "_", S: span.Span{Start: start, End: start}}
	}

	caseExpr := &syn.TyExprCase{
		Scrutinee: scrutinee,
		Alts:      alts,
		S:         span.Span{Start: start, End: p.prevEnd()},
	}

	var body syn.TypeExpr = caseExpr
	if len(params) > 0 {
		body = &syn.TyExprForall{
			Binders: params,
			Body:    caseExpr,
			S:       span.Span{Start: start, End: p.prevEnd()},
		}
	}

	// Encode injectivity result name in KindAnn if present.
	_ = resultName

	return &syn.DeclTypeAlias{
		Name:    name,
		KindAnn: kindAnn,
		Body:    body,
		S:       span.Span{Start: start, End: p.prevEnd()},
	}
}

// parseTyBinderList parses a sequence of type binders: bare or kinded.
func (p *Parser) parseTyBinderList() []syn.TyBinder {
	var params []syn.TyBinder
	for p.peek().Kind == syn.TokLower || (p.peek().Kind == syn.TokLParen && p.isKindedBinder()) {
		if p.peek().Kind == syn.TokLParen {
			lp := p.peek().S.Start
			p.advance()
			pName := p.expectLower()
			p.expect(syn.TokColon)
			kind := p.parseKindExpr()
			p.expect(syn.TokRParen)
			params = append(params, syn.TyBinder{Name: pName, Kind: kind, S: span.Span{Start: lp, End: p.prevEnd()}})
		} else {
			pName := p.peek().Text
			pS := p.peek().S
			p.advance()
			params = append(params, syn.TyBinder{Name: pName, S: pS})
		}
	}
	return params
}

// isKindedBinder checks if the next tokens form (name: Kind) pattern.
func (p *Parser) isKindedBinder() bool {
	if p.pos+2 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == syn.TokLower && p.tokens[p.pos+2].Kind == syn.TokColon
}

// parseResultKindLegacy parses a result kind, optionally with injectivity annotation.
func (p *Parser) parseResultKindLegacy() (syn.KindExpr, string) {
	// Check for injective form: (name: Kind) | deps
	if p.peek().Kind == syn.TokLParen && p.isInjectiveResult() {
		p.advance() // consume (
		resultName := p.expectLower()
		p.expect(syn.TokColon)
		kind := p.parseKindExpr()
		p.expect(syn.TokRParen)
		// Parse | deps (consume but ignore for now)
		if p.peek().Kind == syn.TokPipe {
			p.advance()
			p.parseFunDepListLegacy()
		}
		return kind, resultName
	}
	kind := p.parseKindExpr()
	return kind, ""
}

// isInjectiveResult checks if the tokens form (lower: Kind) | ...
func (p *Parser) isInjectiveResult() bool {
	if p.pos+3 >= len(p.tokens) {
		return false
	}
	return p.tokens[p.pos+1].Kind == syn.TokLower && p.tokens[p.pos+2].Kind == syn.TokColon
}

// parseFunDepListLegacy consumes functional dependency annotations.
func (p *Parser) parseFunDepListLegacy() {
	for {
		if p.peek().Kind == syn.TokLower {
			p.advance() // from
		}
		if p.peek().Kind == syn.TokEqColon {
			p.advance() // =:
			for p.peek().Kind == syn.TokLower {
				p.advance() // to params
			}
		}
		if p.peek().Kind == syn.TokComma {
			p.advance()
		} else {
			break
		}
	}
}

// parseImplDecl parses an axiom registration.
//
//	impl name :: Type := Expr;    (named)
//	impl Type := Expr;            (unnamed)
//
// Disambiguation: if the token after `impl` is a lowercase name followed by `::`,
// it's a named impl. Otherwise the tokens form the type annotation directly.
func (p *Parser) parseImplDecl() *syn.DeclImpl {
	start := p.peek().S.Start
	p.expect(syn.TokImpl)

	// Disambiguate named vs unnamed.
	var name string
	if p.peek().Kind == syn.TokLower && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == syn.TokColonColon {
		name = p.peek().Text
		p.advance() // consume name
		p.advance() // consume ::
	}

	// Parse type annotation, preventing { from being consumed as a row type.
	// This ensures "impl Eq Bool { ... }" parses Eq Bool as annotation, not Eq Bool { ... }.
	savedNoBrace := p.noBraceAtom
	p.noBraceAtom = true
	ann := p.parseType()
	p.noBraceAtom = savedNoBrace

	// Accept both := (new) and direct { (legacy instance format).
	if p.peek().Kind == syn.TokColonEq {
		p.advance()
	}
	body := p.parseExpr()

	return &syn.DeclImpl{
		Name: name,
		Ann:  ann,
		Body: body,
		S:    span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) parseFixityDecl() *syn.DeclFixity {
	start := p.peek().S.Start
	var assoc syn.Assoc
	switch p.peek().Kind {
	case syn.TokInfixl:
		assoc = syn.AssocLeft
	case syn.TokInfixr:
		assoc = syn.AssocRight
	case syn.TokInfixn:
		assoc = syn.AssocNone
	}
	p.advance()
	prec := 9
	if p.peek().Kind == syn.TokIntLit {
		n, err := strconv.Atoi(p.peek().Text)
		if err == nil {
			prec = n
		}
		p.advance()
	}
	var op string
	if p.peek().Kind == syn.TokOp {
		op = p.peek().Text
		p.advance()
	} else if p.peek().Kind == syn.TokDot {
		op = "."
		p.advance()
	} else {
		p.addErrorCode(diagnostic.ErrFixitySyntax, "expected operator in fixity declaration")
		return nil
	}
	return &syn.DeclFixity{
		Assoc: assoc, Prec: prec, Op: op,
		S: span.Span{Start: start, End: p.prevEnd()},
	}
}

func (p *Parser) isOperatorDeclStart() bool {
	return p.pos+2 < len(p.tokens) &&
		(p.tokens[p.pos+1].Kind == syn.TokOp || p.tokens[p.pos+1].Kind == syn.TokDot) &&
		p.tokens[p.pos+2].Kind == syn.TokRParen
}

func (p *Parser) parseOperatorDecl() syn.Decl {
	start := p.expect(syn.TokLParen)
	var op syn.Token
	if p.peek().Kind == syn.TokDot {
		op = p.peek()
		op.Text = "."
		p.advance()
	} else {
		op = p.expect(syn.TokOp)
	}
	p.expect(syn.TokRParen)
	switch p.peek().Kind {
	case syn.TokColonColon:
		p.advance()
		ty := p.parseType()
		return &syn.DeclTypeAnn{Name: op.Text, Type: ty, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	case syn.TokColonEq:
		p.advance()
		expr := p.parseExpr()
		return &syn.DeclValueDef{Name: op.Text, Expr: expr, S: span.Span{Start: start.S.Start, End: p.prevEnd()}}
	default:
		p.addErrorCode(diagnostic.ErrInvalidOperator, "expected :: or := after operator name")
		return nil
	}
}

func (p *Parser) parseNamedDecl() syn.Decl {
	start := p.peek().S.Start
	name := p.peek().Text
	p.advance()

	switch p.peek().Kind {
	case syn.TokColonColon:
		p.advance()
		ty := p.parseType()
		return &syn.DeclTypeAnn{
			Name: name, Type: ty,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	case syn.TokColonEq:
		eqTok := p.advance()
		if p.atStmtBoundary() || p.peek().Kind == syn.TokEOF || p.peek().Kind == syn.TokSemicolon {
			p.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrMissingBody,
				Phase:   diagnostic.PhaseParse,
				Span:    eqTok.S,
				Message: "expected expression after :=",
			})
			return nil
		}
		expr := p.parseExpr()
		return &syn.DeclValueDef{
			Name: name, Expr: expr,
			S: span.Span{Start: start, End: p.prevEnd()},
		}
	default:
		p.addErrorCode(diagnostic.ErrUnexpectedToken, "expected :: or := after name")
		return nil
	}
}

// --- Declaration helpers ---

func (p *Parser) isFixityKeyword() bool {
	k := p.peek().Kind
	return k == syn.TokInfixl || k == syn.TokInfixr || k == syn.TokInfixn
}
