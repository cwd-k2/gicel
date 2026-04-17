package check

// Instance files:
//   instance.go              — processImplHeader, instance registration, validateInstanceContext
//   instance_body.go         — processInstanceBody, processAssocDataDef, validateInstanceMethods
//   instance_assoc_family.go — associated type family saturation for instance method types

import (
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// buildAssocFamilyArgs builds a mapping from associated type family names
// to the class-parameter arguments they should be applied to. This covers
// both the current class's own associated types and those of superclasses.
func (ch *Checker) buildAssocFamilyArgs(classInfo *ClassInfo, ps *types.PreparedSubst) map[string][]types.Type {
	m := make(map[string][]types.Type)
	// Only superclass associated types need saturation.
	// The current class's own associated types are already correctly
	// applied during type resolution (TyFamilyApp with class params).
	// Superclass associated types, however, may appear with the wrong
	// first argument (the method's quantified variable instead of the
	// class parameter) because the type resolver doesn't track the
	// cross-class parameter mapping.
	for _, sup := range classInfo.Supers {
		superInfo, ok := ch.reg.LookupClass(sup.ClassName)
		if !ok {
			continue
		}
		superArgs := make([]types.Type, len(sup.Args))
		for j, a := range sup.Args {
			superArgs[j] = ps.Apply(a)
		}
		for _, name := range superInfo.AssocTypes {
			m[name] = superArgs
		}
	}
	return m
}

// saturateAssocFamilies walks ty and converts bare associated type family
// TyCons into TyFamilyApp nodes by applying the substituted class parameters.
// In GICEL, associated type families in method signatures use implicit class
// parameter application (e.g. GradeDrop instead of GradeDrop g). After
// instance substitution, the TyCon remains bare. This function inserts the
// explicit arguments so the family reducer can process them.
func (ch *Checker) saturateAssocFamilies(ty types.Type, familyArgs map[string][]types.Type) types.Type {
	return ch.satAssocWalk(ty, familyArgs)
}

func (ch *Checker) satAssocWalk(ty types.Type, fa map[string][]types.Type) types.Type {
	switch t := ty.(type) {
	case *types.TyCon:
		if args, ok := fa[t.Name]; ok {
			fam, ok := ch.reg.LookupFamily(t.Name)
			if ok {
				return ch.typeOps.FamilyApp(t.Name, args, fam.ResultKind, t.S)
			}
		}
		return ty

	case *types.TyFamilyApp:
		// An existing TyFamilyApp whose args may be incorrectly populated.
		// When a superclass associated type family is used in a subclass method,
		// the type resolver consumes user-supplied arguments as family args
		// instead of the implicit class params. We detect this by comparing
		// the existing args with the expected class params (familyArgs):
		//   - Match: already correct (e.g. Elem l → Elem (List a) after subst)
		//   - Mismatch: wrong args consumed (e.g. GradeCompose e1 instead of GradeCompose g)
		//     → replace with class params, push original args to TyApp positions
		if famClassArgs, ok := fa[t.Name]; ok {
			if !assocArgsMatch(ch.typeOps, t.Args, famClassArgs) {
				fam, famOK := ch.reg.LookupFamily(t.Name)
				if famOK {
					var result types.Type = ch.typeOps.FamilyApp(t.Name, famClassArgs, fam.ResultKind, t.S)
					for _, a := range t.Args {
						rA := ch.satAssocWalk(a, fa)
						result = ch.typeOps.App(result, rA, t.S)
					}
					return result
				}
			}
		}
		// Already correct or not a target family — recurse into args.
		changed := false
		newArgs := make([]types.Type, len(t.Args))
		for i, a := range t.Args {
			newArgs[i] = ch.satAssocWalk(a, fa)
			if newArgs[i] != a {
				changed = true
			}
		}
		if !changed {
			return ty
		}
		return ch.typeOps.FamilyApp(t.Name, newArgs, t.Kind, t.S)

	case *types.TyApp:
		// Unwind the app chain to check if head is an associated type family.
		head, appArgs := types.UnwindApp(ty)
		if con, ok := head.(*types.TyCon); ok {
			if famClassArgs, ok := fa[con.Name]; ok {
				fam, famOK := ch.reg.LookupFamily(con.Name)
				if famOK {
					// Convert head to TyFamilyApp with class args,
					// then re-apply the remaining (user-supplied) args.
					var result types.Type = ch.typeOps.FamilyApp(con.Name, famClassArgs, fam.ResultKind, con.S)
					for _, a := range appArgs {
						rA := ch.satAssocWalk(a, fa)
						result = ch.typeOps.App(result, rA, t.S)
					}
					return result
				}
			}
		}
		// Not an associated type family head — recurse normally.
		rFun := ch.satAssocWalk(t.Fun, fa)
		rArg := ch.satAssocWalk(t.Arg, fa)
		if rFun == t.Fun && rArg == t.Arg {
			return ty
		}
		return ch.typeOps.App(rFun, rArg, t.S)

	case *types.TyArrow:
		rFrom := ch.satAssocWalk(t.From, fa)
		rTo := ch.satAssocWalk(t.To, fa)
		if rFrom == t.From && rTo == t.To {
			return ty
		}
		return ch.typeOps.Arrow(rFrom, rTo, t.S)

	case *types.TyForall:
		rKind := ch.satAssocWalk(t.Kind, fa)
		rBody := ch.satAssocWalk(t.Body, fa)
		if rKind == t.Kind && rBody == t.Body {
			return ty
		}
		return ch.typeOps.Forall(t.Var, rKind, rBody, t.S)

	case *types.TyCBPV:
		rPre := ch.satAssocWalk(t.Pre, fa)
		rPost := ch.satAssocWalk(t.Post, fa)
		rResult := ch.satAssocWalk(t.Result, fa)
		var rGrade types.Type
		if t.IsGraded() {
			rGrade = ch.satAssocWalk(t.Grade, fa)
		}
		if rPre == t.Pre && rPost == t.Post && rResult == t.Result && rGrade == t.Grade {
			return ty
		}
		if t.Tag == types.TagComp {
			return ch.typeOps.Comp(rPre, rPost, rResult, rGrade, t.S)
		}
		if rGrade != nil {
			return ch.typeOps.ThunkGraded(rPre, rPost, rResult, rGrade, t.S)
		}
		return ch.typeOps.Thunk(rPre, rPost, rResult, t.S)

	case *types.TyEvidence:
		rBody := ch.satAssocWalk(t.Body, fa)
		if rBody == t.Body {
			return ty
		}
		return ch.typeOps.EvidenceWrap(t.Constraints, rBody, t.S)

	default:
		// True leaves: TyMeta, TySkolem, TyVar, TyError, TyEvidenceRow.
		// These contain no substructure where associated type families can appear.
		return ty
	}
}

// assocArgsMatch checks if the TyFamilyApp's existing args structurally
// match the expected class params. If they match, the family is already
// correctly applied and no saturation is needed.
func assocArgsMatch(ops *types.TypeOps, existing, expected []types.Type) bool {
	if len(existing) != len(expected) {
		return false
	}
	for i := range existing {
		if !ops.Equal(existing[i], expected[i]) {
			return false
		}
	}
	return true
}
