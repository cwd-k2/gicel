// Engine grade algebra tests — user-defined grade algebra via GradeAlgebra class.
// Does NOT cover: checker unit tests (check/grade_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// --- User-defined grade algebra (Level) ---

const levelGradeSource = `
import Prelude

form Level := { Public: Level; Secret: Level }

type LevelJoin :: Level -> Level -> Level := \(a: Level) (b: Level). case (a, b) {
  (Secret, _) => Secret;
  (_, Secret) => Secret;
  (x, _)      => x
}

impl GradeAlgebra Level := {
  type GradeJoin := LevelJoin;
  type GradeDrop := Public
}
`

func TestUserDefinedGradeAlgebraCompiles(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), levelGradeSource+`
main := 42
`)
	if err != nil {
		t.Fatalf("user-defined grade algebra should compile: %v", err)
	}
}

func TestUserDefinedGradeSecretPreservation(t *testing.T) {
	// @Secret field CAN be preserved in Level algebra — Join(Public, Secret) = Secret = Secret.
	// Level algebra tracks information flow, not consumption: all levels are preservable.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), levelGradeSource+`
ok :: () -> Computation { key: Int @Secret } { key: Int @Secret } Int
ok := \_. do { pure 42 }
main := 42
`)
	if err != nil {
		t.Fatalf("@Secret preservation should be allowed in Level algebra: %v", err)
	}
}

func TestUserDefinedGradePublicPreservation(t *testing.T) {
	// @Public field can be preserved — Join(Public, Public) = Public.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), levelGradeSource+`
ok :: () -> Computation { tag: Int @Public } { tag: Int @Public } Int
ok := \_. do { pure 42 }
main := 42
`)
	if err != nil {
		t.Fatalf("@Public preservation should be allowed: %v", err)
	}
}

// --- Standard Mult grade algebra ---

func TestStandardLinearMustBeConsumed(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
bad :: () -> Computation { x: Int @Linear } { x: Int @Linear } Int
bad := \_. do { pure 42 }
main := 42
`)
	if err == nil {
		t.Fatal("@Linear preservation should be rejected")
	}
	if !strings.Contains(err.Error(), "must be consumed") {
		t.Fatalf("expected 'must be consumed' error, got: %v", err)
	}
}

func TestStandardZeroPreservation(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
ok :: () -> Computation { x: Int @Zero } { x: Int @Zero } Int
ok := \_. do { pure 42 }
main := 42
`)
	if err != nil {
		t.Fatalf("@Zero preservation should be allowed: %v", err)
	}
}

func TestStandardUnrestrictedPreservation(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
ok :: () -> Computation { x: Int @Unrestricted } { x: Int @Unrestricted } Int
ok := \_. do { pure 42 }
main := 42
`)
	if err != nil {
		t.Fatalf("@Unrestricted preservation should be allowed: %v", err)
	}
}
