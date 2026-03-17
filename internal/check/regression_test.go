package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================
// Targeted Regression Tests
// for specific issues found during code analysis.
// Branch: feature/type-system-extensions
// ==========================================

// -----------------------------------------------
// 1. FreeVars Mult traversal
// -----------------------------------------------

// TestRegressionFreeVarsMultTraversal verifies that types.FreeVars
// correctly collects type variables from Mult annotations on row fields.
// If the Mult traversal were missing, the variable "m" would be lost.
func TestRegressionFreeVarsMultTraversal(t *testing.T) {
	row := types.ClosedRow(types.RowField{
		Label: "handle",
		Type:  &types.TyCon{Name: "FileHandle"},
		Mult:  &types.TyVar{Name: "m"},
	})
	fv := types.FreeVars(row)
	if _, ok := fv["m"]; !ok {
		t.Errorf("expected 'm' in FreeVars, got %v", fv)
	}
}

// TestRegressionFreeVarsMultTraversalMultiField verifies FreeVars with
// multiple fields where Mult uses different type variables.
func TestRegressionFreeVarsMultTraversalMultiField(t *testing.T) {
	row := types.ClosedRow(
		types.RowField{
			Label: "x",
			Type:  &types.TyVar{Name: "a"},
			Mult:  &types.TyVar{Name: "m1"},
		},
		types.RowField{
			Label: "y",
			Type:  &types.TyVar{Name: "b"},
			Mult:  &types.TyVar{Name: "m2"},
		},
	)
	fv := types.FreeVars(row)
	for _, v := range []string{"a", "b", "m1", "m2"} {
		if _, ok := fv[v]; !ok {
			t.Errorf("expected %q in FreeVars, got %v", v, fv)
		}
	}
	if len(fv) != 4 {
		t.Errorf("expected 4 free vars, got %d: %v", len(fv), fv)
	}
}

// TestRegressionFreeVarsMultNilIgnored verifies that nil Mult does
// not contribute free variables.
func TestRegressionFreeVarsMultNilIgnored(t *testing.T) {
	row := types.ClosedRow(types.RowField{
		Label: "handle",
		Type:  &types.TyCon{Name: "FileHandle"},
		Mult:  nil, // no multiplicity annotation
	})
	fv := types.FreeVars(row)
	if len(fv) != 0 {
		t.Errorf("expected 0 free vars for nil Mult, got %v", fv)
	}
}

// -----------------------------------------------
// 2. reduceFamilyApps structural recursion
// -----------------------------------------------

// TestRegressionReduceFamilyInArrowType verifies that type family
// applications nested inside arrow types are reduced during type checking.
// The reduction happens via the unifier's normalize pass, which is called
// when unifying subcomponents of TyArrow.
func TestRegressionReduceFamilyInArrowType(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// TestRegressionReduceFamilyInCompType verifies that type family
// applications nested inside Computation types are reduced.
func TestRegressionReduceFamilyInCompType(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: Computation {} {} (Elem (List Unit)) -> Computation {} {} Unit
f := \c. c
`
	checkSource(t, source, nil)
}

// TestRegressionReduceFamilyInNestedArrow verifies a more complex case:
// type family in the argument of an arrow within another arrow.
func TestRegressionReduceFamilyInNestedArrow(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: (Elem (List Unit) -> Unit) -> Unit -> Unit
f := \g x. g x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 3. Injectivity with non-Type-kinded parameters
// -----------------------------------------------

// TestRegressionInjectivityNonTypeKind verifies injectivity checking
// when type family parameters have kinds other than Type (DataKinds).
func TestRegressionInjectivityNonTypeKind(t *testing.T) {
	// Session is a promoted data kind. Dual maps Session -> Session
	// and is injective.
	source := `
data Session := Send Session | Recv Session | End
type Dual (s: Session) :: (r: Session) | r =: s := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
}
`
	checkSource(t, source, nil)
}

// TestRegressionInjectivityNonTypeKindViolation verifies that injectivity
// violations are detected even when parameter kinds are non-Type.
func TestRegressionInjectivityNonTypeKindViolation(t *testing.T) {
	// Both equations map to End, but from different LHS patterns.
	source := `
data Session := Send Session | Recv Session | End
type Bad (s: Session) :: (r: Session) | r =: s := {
  Bad (Send s) =: End;
  Bad (Recv s) =: End
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrInjectivity)
}

// -----------------------------------------------
// 4. Data family constructor collision detection
// -----------------------------------------------

// TestRegressionDataFamilyConstructorCollision verifies that a data family
// instance with a constructor name that conflicts with an existing data type
// produces a duplicate declaration error.
func TestRegressionDataFamilyConstructorCollision(t *testing.T) {
	source := `
data Wrapper a := Wrap a
data Unit := Unit

class Container c {
  data Elem c :: Type
}

instance Container (Wrapper a) {
  data Elem (Wrapper a) =: Wrap a
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrDuplicateDecl)
}

// TestRegressionDataFamilyConstructorNoCollision verifies that distinct
// constructor names in data family instances do not produce errors.
func TestRegressionDataFamilyConstructorNoCollision(t *testing.T) {
	source := `
data Wrapper a := Wrap a
data Unit := Unit

class Container c {
  data Elem c :: Type
}

instance Container (Wrapper a) {
  data Elem (Wrapper a) =: WrapElem a
}

x :: Elem (Wrapper Unit)
x := WrapElem Unit
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 5. Exponential type growth bound
// -----------------------------------------------

// TestRegressionExponentialTypeGrowthBound verifies that a type family
// of the form `Grow a =: Grow (Pair a a)` is bounded by the reduction
// limits and terminates with an appropriate error.
func TestRegressionExponentialTypeGrowthBound(t *testing.T) {
	source := `
data Pair a b := MkPair a b
data Unit := Unit
type Grow (a: Type) :: Type := {
  Grow a =: Grow (Pair a a)
}
f :: Grow Unit -> Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
}

// -----------------------------------------------
// 6. Fundep improvement is best-effort
// -----------------------------------------------

// TestRegressionFundepBestEffort verifies that fundep improvement failure
// does not prevent successful type checking when types can be resolved by
// other means (e.g., annotation, direct instance resolution).
func TestRegressionFundepBestEffort(t *testing.T) {
	// The type of `extract xs` is determined by the annotation f :: ... -> Unit,
	// not solely by the fundep improvement. Even if fundep improvement were
	// disabled, the instance resolution for Elem (List a) a should succeed.
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
class Elem c e | c =: e {
  extract :: c -> e
}
instance Elem (List a) a {
  extract := \xs. case xs { Cons x rest -> x; Nil -> extract Nil }
}
f :: List Unit -> Unit
f := \xs. extract xs
`
	checkSource(t, source, nil)
}

// TestRegressionFundepImprovementFromMeta verifies that fundep improvement
// does not fire when the "from" position is an unsolved meta. The program
// should still compile via normal instance resolution.
func TestRegressionFundepImprovementFromMeta(t *testing.T) {
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
class Collection c e | c =: e {
  empty :: c
}
instance Collection (List a) a {
  empty := Nil
}
main :: List Unit
main := empty
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 7. Mangled name distinctness
// -----------------------------------------------

// TestRegressionMangledNameDistinctness verifies that the arity-prefixed
// mangling produces distinct names for cases that would collide under a
// naive scheme (without arity prefix).
func TestRegressionMangledNameDistinctness(t *testing.T) {
	ch := newTestChecker()

	// Case 1: Family "F" with patterns [A, B] vs [A$B]
	name1 := ch.mangledDataFamilyName("F", []types.Type{
		&types.TyCon{Name: "A"},
		&types.TyCon{Name: "B"},
	})
	name2 := ch.mangledDataFamilyName("F", []types.Type{
		&types.TyCon{Name: "A$B"},
	})
	if name1 == name2 {
		t.Errorf("arity collision: F [A, B] == F [A$B] == %q", name1)
	}

	// Case 2: Family "A" with pattern [B$C] vs Family "A$B" with pattern [C]
	name3 := ch.mangledDataFamilyName("A", []types.Type{
		&types.TyCon{Name: "B$C"},
	})
	name4 := ch.mangledDataFamilyName("A$B", []types.Type{
		&types.TyCon{Name: "C"},
	})
	if name3 == name4 {
		t.Errorf("family name collision: A [B$C] == A$B [C] == %q", name3)
	}

	// Case 3: Same family, different arity
	name5 := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyCon{Name: "List"},
	})
	name6 := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyCon{Name: "List"},
		&types.TyCon{Name: "Int"},
	})
	if name5 == name6 {
		t.Errorf("arity collision: Elem [List] == Elem [List, Int] == %q", name5)
	}

	// Case 4: Zero-arity vs non-zero-arity
	name7 := ch.mangledDataFamilyName("Elem$List", []types.Type{})
	name8 := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyCon{Name: "List"},
	})
	if name7 == name8 {
		t.Errorf("zero-arity collision: Elem$List [] == Elem [List] == %q", name7)
	}
}

// -----------------------------------------------
// 8. matchTyPatterns length mismatch
// -----------------------------------------------

// TestRegressionMatchTyPatternsLengthMismatch verifies that calling
// matchTyPatterns with mismatched pattern/arg lengths returns matchFail
// (not panic). The length guard in matchTyPatterns prevents an index
// out-of-range panic when len(patterns) > len(args).
func TestRegressionMatchTyPatternsLengthMismatch(t *testing.T) {
	ch := newTestChecker()

	// More patterns than args.
	patterns := []types.Type{
		&types.TyCon{Name: "Int"},
		&types.TyCon{Name: "Bool"},
		&types.TyCon{Name: "Unit"},
	}
	args := []types.Type{
		&types.TyCon{Name: "Int"},
	}

	_, result := ch.matchTyPatterns(patterns, args)
	if result != matchFail {
		t.Fatalf("expected matchFail for length mismatch (more patterns), got %d", result)
	}
}

// TestRegressionMatchTyPatternsLengthMismatchMoreArgs verifies the
// reverse case: more args than patterns also returns matchFail.
func TestRegressionMatchTyPatternsLengthMismatchMoreArgs(t *testing.T) {
	ch := newTestChecker()

	patterns := []types.Type{
		&types.TyCon{Name: "Int"},
	}
	args := []types.Type{
		&types.TyCon{Name: "Int"},
		&types.TyCon{Name: "Bool"},
	}

	_, result := ch.matchTyPatterns(patterns, args)
	if result != matchFail {
		t.Fatalf("expected matchFail for length mismatch (more args), got %d", result)
	}
}

// -----------------------------------------------
// 9. SubstMany equivalence
// -----------------------------------------------

// TestRegressionSubstManyEquivalence verifies that SubstMany produces
// the same result as sequential Subst calls for independent variables
// (no mutual references). This matters for type family reduction where
// pattern variables bind to independent types.
func TestRegressionSubstManyEquivalence(t *testing.T) {
	// Type: Pair a b
	ty := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyCon{Name: "Pair"},
			Arg: &types.TyVar{Name: "a"},
		},
		Arg: &types.TyVar{Name: "b"},
	}

	intTy := &types.TyCon{Name: "Int"}
	boolTy := &types.TyCon{Name: "Bool"}

	// Sequential Subst.
	seqResult := types.Subst(ty, "a", intTy)
	seqResult = types.Subst(seqResult, "b", boolTy)

	// SubstMany.
	manyResult := types.SubstMany(ty, map[string]types.Type{
		"a": intTy,
		"b": boolTy,
	})

	if !types.Equal(seqResult, manyResult) {
		t.Errorf("SubstMany != sequential Subst:\n  seq:  %s\n  many: %s",
			types.Pretty(seqResult), types.Pretty(manyResult))
	}
}

// TestRegressionSubstManyWithShadowing tests SubstMany when one variable's
// replacement contains another variable that is also being substituted.
// Sequential Subst would substitute into the replacement, but SubstMany
// (which is implemented as sequential Subst) would also do the same.
// This is important to understand: SubstMany is NOT simultaneous.
func TestRegressionSubstManyWithShadowing(t *testing.T) {
	// Type: a -> b
	ty := &types.TyArrow{
		From: &types.TyVar{Name: "a"},
		To:   &types.TyVar{Name: "b"},
	}

	// Substitute a=b, b=Int.
	// Sequential: first a=b gives b -> b, then b=Int gives Int -> Int.
	// A truly simultaneous subst would give b -> Int.
	// SubstMany uses sequential, so it should give Int -> Int.
	result := types.SubstMany(ty, map[string]types.Type{
		"a": &types.TyVar{Name: "b"},
		"b": &types.TyCon{Name: "Int"},
	})

	// The result depends on map iteration order (non-deterministic).
	// If a is substituted first: a->b gives (b -> b), then b->Int gives (Int -> Int).
	// If b is substituted first: b->Int gives (a -> Int), then a->b gives (b -> Int).
	// This test documents that SubstMany is order-dependent.
	// For type family reduction, pattern variables bind to concrete types
	// that do not contain other pattern variables, so this is not an issue.
	pretty := types.Pretty(result)
	t.Logf("SubstMany(a -> b, {a=b, b=Int}) = %s (order-dependent)", pretty)

	// Verify that it at least does not panic and produces a valid type.
	if result == nil {
		t.Fatal("SubstMany returned nil")
	}
}

// TestRegressionSubstManyIndependentVars verifies equivalence when
// substitution variables are independent (the typical case in type
// family reduction).
func TestRegressionSubstManyIndependentVars(t *testing.T) {
	// RHS: Pair (List a) (Maybe b) - typical type family RHS
	ty := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyCon{Name: "Pair"},
			Arg: &types.TyApp{
				Fun: &types.TyCon{Name: "List"},
				Arg: &types.TyVar{Name: "a"},
			},
		},
		Arg: &types.TyApp{
			Fun: &types.TyCon{Name: "Maybe"},
			Arg: &types.TyVar{Name: "b"},
		},
	}

	unitTy := &types.TyCon{Name: "Unit"}
	boolTy := &types.TyCon{Name: "Bool"}

	// Sequential in both orders should be the same.
	seq1 := types.Subst(types.Subst(ty, "a", unitTy), "b", boolTy)
	seq2 := types.Subst(types.Subst(ty, "b", boolTy), "a", unitTy)

	if !types.Equal(seq1, seq2) {
		t.Errorf("sequential Subst order matters for independent vars:\n  a-first: %s\n  b-first: %s",
			types.Pretty(seq1), types.Pretty(seq2))
	}

	// SubstMany should equal both.
	many := types.SubstMany(ty, map[string]types.Type{
		"a": unitTy,
		"b": boolTy,
	})
	if !types.Equal(seq1, many) {
		t.Errorf("SubstMany differs from sequential Subst:\n  seq: %s\n  many: %s",
			types.Pretty(seq1), types.Pretty(many))
	}
}
