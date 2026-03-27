package types

// Arity returns the number of arguments a kind accepts.
// After Kind→Type unification, kinds are represented as Type (TyArrow chains).
func Arity(k Type) int {
	if ka, ok := k.(*TyArrow); ok {
		return 1 + Arity(ka.To)
	}
	return 0
}

// ResultKind returns the kind after all arguments are applied.
// After Kind→Type unification, kinds are represented as Type.
func ResultKind(k Type) Type {
	if ka, ok := k.(*TyArrow); ok {
		return ResultKind(ka.To)
	}
	return k
}
