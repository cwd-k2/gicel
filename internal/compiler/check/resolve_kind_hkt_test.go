package check

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// =============================================================================
// Kind-polymorphic function resolution
// =============================================================================

func TestResolveKindSort(t *testing.T) {
	// \ (k: Kind). \ (a: k). a -> a
	// Should parse and resolve without errors.
	source := `id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x`
	checkSource(t, source, nil)
}

func TestResolveKindVarInArrow(t *testing.T) {
	// \ (k: Kind). \ (f: k -> Type). f Int -> f Int
	source := `apply_f :: \ (k: Kind). \ (f: k -> Type). f Int -> f Int
apply_f := \x. x`
	checkSource(t, source, nil)
}

func TestKindVarSingleForall(t *testing.T) {
	// Kind variable in a single \ with multiple binders
	source := `id_k :: \ (k: Kind) (a: k). a -> a
id_k := \x. x`
	checkSource(t, source, nil)
}

// =============================================================================
// Kind variable scoping
// =============================================================================

func TestKindVarNotInScopeOutside(t *testing.T) {
	// 'k' used in kind position but not bound as a kind variable.
	// KindExprName "k" should fall through to KType{} (not an error,
	// just treated as unknown → defaults to Type).
	source := `f :: \ (a: k). a -> a
f := \x. x`
	checkSource(t, source, nil)
}

// =============================================================================
// Kind-polymorphic instantiation (subsCheck)
// =============================================================================

func TestKindPolyInstantiation(t *testing.T) {
	// Define a kind-polymorphic identity and use it at a concrete kind.
	source := `
form Bool := { True: Bool; False: Bool; }

id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

use := id_k True
`
	checkSource(t, source, nil)
}

func TestKindPolyInstantiationArrow(t *testing.T) {
	// Kind-polymorphic function applied to a function-kinded type.
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

use_maybe := id_k (Just True)
`
	checkSource(t, source, nil)
}

// =============================================================================
// Parser round-trip validation
// =============================================================================

func parseKindPoly(t *testing.T, source string) {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), tokens, es)
	_ = p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
}

func TestParseKindPolyFunction(t *testing.T) {
	parseKindPoly(t, `f :: \ (k: Kind). \ (f: k -> Type). f Int -> f Int
f := \x. x`)
}

func TestParseKindPolyNestedArrow(t *testing.T) {
	parseKindPoly(t, `f :: \ (k: Kind) (j: Kind). \ (f: k -> j -> Type). Int
f := \x. x`)
}
