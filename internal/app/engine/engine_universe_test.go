// Universe polymorphism Phase B-C integration tests — Level kind, LevelVar scoping.
// Does NOT cover: level unification internals (unify/level_unify_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

func TestUniverseLevelKindInForm(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	_, err := eng.NewRuntime(context.Background(), `
import Prelude

form LevelProxy := \(l: Level). { MkLP: LevelProxy l; }

main := MkLP
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUniverseLevelVarInference(t *testing.T) {
	// Level parameter in form is properly scoped and inferred.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	_, err := eng.NewRuntime(context.Background(), `
import Prelude

form Leveled := \(l: Level). { MkLev: Leveled l; }

-- No explicit type annotation: level is inferred
x := MkLev

main := 42
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUniverseKindCumulativity(t *testing.T) {
	// Type ≤ Kind: a type-level param should accept Kind values.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	_, err := eng.NewRuntime(context.Background(), `
import Prelude

form Proxy := \(a: Kind). { MkProxy: Proxy a; }

p :: Proxy Int
p := MkProxy

main := 42
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUniverseKDataPromotion(t *testing.T) {
	// KData (promoted data kind) should be usable as a kind annotation.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	_, err := eng.NewRuntime(context.Background(), `
import Prelude

form Color := { Red: Color; Blue: Color; }
form Tagged := \(c: Color). { MkTagged: Tagged c; }

red :: Tagged Red
red := MkTagged

main := 42
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestUniverseKindMismatchError(t *testing.T) {
	// Wrong arity in type application should produce an error.
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	_, err := eng.NewRuntime(context.Background(), `
import Prelude

form Box := \a. { MkBox: a -> Box a; }

-- Applying Box to two type args should fail
bad :: Box Int Bool
bad := MkBox 42

main := 42
`)
	if err == nil {
		t.Fatal("expected type error for wrong arity")
	}
	if !strings.Contains(err.Error(), "mismatch") && !strings.Contains(err.Error(), "expected") {
		t.Fatalf("expected type mismatch error, got: %v", err)
	}
}
