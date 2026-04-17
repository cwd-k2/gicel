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
			superArgs[j] = ps.Apply(ch.typeOps, a)
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
	recurse := func(child types.Type) types.Type {
		return ch.satAssocWalk(child, fa)
	}

	switch t := ty.(type) {
	case *types.TyCon:
		// Intercept bare associated type family TyCons → TyFamilyApp.
		if args, ok := fa[t.Name]; ok {
			if fam, ok := ch.reg.LookupFamily(t.Name); ok {
				return ch.typeOps.FamilyAppAt(t.Name, args, fam.ResultKind, t.S)
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
				if fam, famOK := ch.reg.LookupFamily(t.Name); famOK {
					var result types.Type = ch.typeOps.FamilyAppAt(t.Name, famClassArgs, fam.ResultKind, t.S)
					for _, a := range t.Args {
						result = ch.typeOps.AppAt(result, recurse(a), t.S)
					}
					return result
				}
			}
		}
		// Correct or not a target — structural recursion via MapType.
		return ch.typeOps.MapType(ty, recurse)

	case *types.TyApp:
		// Unwind the app chain to check if head is an associated type family.
		head, appArgs := types.UnwindApp(ty)
		if con, ok := head.(*types.TyCon); ok {
			if famClassArgs, ok := fa[con.Name]; ok {
				if fam, famOK := ch.reg.LookupFamily(con.Name); famOK {
					var result types.Type = ch.typeOps.FamilyAppAt(con.Name, famClassArgs, fam.ResultKind, con.S)
					for _, a := range appArgs {
						result = ch.typeOps.AppAt(result, recurse(a), t.S)
					}
					return result
				}
			}
		}
		// Not a family head — structural recursion via MapType.
		return ch.typeOps.MapType(ty, recurse)

	default:
		// All structural nodes (TyArrow, TyForall, TyCBPV, TyEvidence,
		// TyEvidenceRow) and true leaves (TyVar, TyMeta, TySkolem, TyError).
		// MapType handles identity-preserving reconstruction and panics on
		// unknown variants — no silent drop.
		return ch.typeOps.MapType(ty, recurse)
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
