package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// GradeAlgebra associated type family names. These are the canonical names
// used in the GradeAlgebra class definition; the compiler must know them to
// extract grade algebra operations from resolved instances.
const (
	gradeAssocJoin    = "GradeJoin"
	gradeAssocCompose = "GradeCompose"
	gradeAssocDrop    = "GradeDrop"
	gradeAssocUnit    = "GradeUnit"
)

// gradeAlgebraKind returns the kind to use for grade algebra parameters.
// If "Mult" is registered as a promoted kind (via DataKinds), returns
// PromotedDataKind("Mult"); otherwise falls back to TypeOfTypes.
func gradeAlgebraKind(ch *Checker) types.Type {
	if k, ok := ch.reg.LookupPromotedKind("Mult"); ok {
		return k
	}
	return types.TypeOfTypes
}

// gradeAlgebraClassName is the name of the user-facing grade algebra class.
const gradeAlgebraClassName = "GradeAlgebra"

// resolvedGradeAlgebra holds the resolved join family name and drop value
// for a grade kind.
type resolvedGradeAlgebra struct {
	joinFamily    string     // name of the GradeJoin type family
	composeFamily string     // name of the GradeCompose type family
	dropValue     types.Type // the Drop element (promoted constructor, e.g. Zero)
	unitValue     types.Type // the Unit element (identity of Compose, e.g. Linear)
	valid         bool       // false if no GradeAlgebra instance found
}

// resolveGradeAlgebra looks up a GradeAlgebra instance for the given grade kind
// and extracts the associated type family names by reducing the associated types.
// Returns a result with valid=false if no GradeAlgebra instance is found;
// callers must check valid before using the algebra.
func (ch *Checker) resolveGradeAlgebra(gradeKind types.Type) resolvedGradeAlgebra {
	classInfo, hasClass := ch.reg.LookupClass(gradeAlgebraClassName)
	if hasClass {
		// Match grade kind against instance type args via the promoted kind
		// registry. GradeAlgebra takes a Kind-kinded parameter (g: Kind).
		// Instance: impl GradeAlgebra Mult := ...
		// Instance TypeArgs[0] = TyCon("Mult") at L0 (value-level type).
		// Grade kind = PromotedDataKind("Mult") at L1 (promoted kind).
		// The promotion relationship is established by DataKinds and stored
		// in the registry. We look up each instance type arg's promoted form
		// and compare structurally with the grade kind.
		instances := ch.reg.InstancesForClass(gradeAlgebraClassName)
		for _, inst := range instances {
			if len(inst.TypeArgs) == 0 {
				continue
			}
			con, ok := inst.TypeArgs[0].(*types.TyCon)
			if !ok {
				continue
			}
			promoted, hasPromoted := ch.reg.LookupPromotedKind(con.Name)
			if !hasPromoted {
				continue
			}
			if types.Equal(promoted, gradeKind) {
				result := ch.extractGradeAlgebra(classInfo, inst)
				result.valid = true
				return result
			}
		}
	}
	// No GradeAlgebra instance found. Grade enforcement not available.
	return resolvedGradeAlgebra{valid: false}
}

// extractGradeAlgebra extracts GradeJoin and GradeDrop from a matched instance
// by reducing the associated type families with the instance's type args.
func (ch *Checker) extractGradeAlgebra(classInfo *ClassInfo, inst *InstanceInfo) resolvedGradeAlgebra {
	var result resolvedGradeAlgebra
	for _, assocName := range classInfo.AssocTypes {
		if _, ok := ch.reg.LookupFamily(assocName); !ok {
			continue
		}
		// Reduce the associated type with the instance's type args.
		reduced, didReduce := ch.reduceTyFamily(assocName, inst.TypeArgs, inst.S)
		if !didReduce {
			continue
		}
		switch assocName {
		case gradeAssocJoin:
			if con, ok := reduced.(*types.TyCon); ok {
				result.joinFamily = con.Name
			} else {
				return resolvedGradeAlgebra{}
			}
		case gradeAssocCompose:
			if con, ok := reduced.(*types.TyCon); ok {
				result.composeFamily = con.Name
			} else {
				return resolvedGradeAlgebra{}
			}
		case gradeAssocDrop:
			result.dropValue = reduced
		case gradeAssocUnit:
			result.unitValue = reduced
		}
	}
	return result
}

// gradeContainsMeta reports whether ty contains any unsolved metavariable.
func gradeContainsMeta(ty types.Type) bool {
	return types.AnyType(ty, func(t types.Type) bool {
		_, ok := t.(*types.TyMeta)
		return ok
	})
}

// reduceConcreteEqs matches type family equations against concrete
// (meta-free) args. Pure function — no side effects.
func reduceConcreteEqs(eqs []env.TFEquation, args []types.Type) (types.Type, bool) {
	for _, eq := range eqs {
		if len(eq.Patterns) != len(args) {
			continue
		}
		subst := make(map[string]types.Type)
		matched := true
		for i, pat := range eq.Patterns {
			if !matchConcretePattern(pat, args[i], subst) {
				matched = false
				break
			}
		}
		if matched {
			return substType(eq.RHS, subst), true
		}
	}
	return nil, false
}

// gradeConEqual compares two grade constructor values by name only.
// Grade constructors in axiom verification come from two sources with
// potentially different Level fields (equation RHS vs freshly constructed).
// Since grade constructors are always nullary promoted data constructors,
// name equality is sufficient and correct.
func gradeConEqual(a, b types.Type) bool {
	ac, ok1 := a.(*types.TyCon)
	bc, ok2 := b.(*types.TyCon)
	if ok1 && ok2 {
		return ac.Name == bc.Name
	}
	return types.Equal(a, b)
}

// matchConcretePattern matches a TF equation pattern against a concrete
// (meta-free) argument. Handles TyVar (pattern variable), TyCon
// (constructor literal), and TyApp (type application, e.g. tuple patterns).
func matchConcretePattern(pat, arg types.Type, subst map[string]types.Type) bool {
	switch p := pat.(type) {
	case *types.TyVar:
		if p.Name == "_" {
			return true
		}
		if existing, ok := subst[p.Name]; ok {
			return types.Equal(existing, arg)
		}
		subst[p.Name] = arg
		return true
	case *types.TyCon:
		c, ok := arg.(*types.TyCon)
		return ok && p.Name == c.Name
	case *types.TyApp:
		a, ok := arg.(*types.TyApp)
		if !ok {
			return false
		}
		return matchConcretePattern(p.Fun, a.Fun, subst) &&
			matchConcretePattern(p.Arg, a.Arg, subst)
	default:
		return types.Equal(pat, arg)
	}
}

// substType substitutes pattern variables in a type family RHS.
func substType(t types.Type, subst map[string]types.Type) types.Type {
	switch x := t.(type) {
	case *types.TyVar:
		if s, ok := subst[x.Name]; ok {
			return s
		}
		return t
	case *types.TyApp:
		newFun := substType(x.Fun, subst)
		newArg := substType(x.Arg, subst)
		if newFun == x.Fun && newArg == x.Arg {
			return t
		}
		return &types.TyApp{Fun: newFun, Arg: newArg, IsGrade: x.IsGrade}
	default:
		return t
	}
}
