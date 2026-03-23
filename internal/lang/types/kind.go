package types

import "fmt"

// Kind classifies types and rows.
//
//	Kind ::= KType | KRow | KArrow(K1, K2)
type Kind interface {
	kindNode()
	Equal(Kind) bool
	String() string
}

// KType is the kind of value types.
type KType struct{}

// KRow is the kind of row types.
type KRow struct{}

// KConstraint is the kind of type class constraints.
type KConstraint struct{}

// KData is the kind of promoted data types (DataKinds).
// E.g., `form DBState := Opened | Closed` promotes to kind `DBState`.
type KData struct{ Name string }

// KArrow is the kind of type constructors (K1 -> K2).
type KArrow struct {
	From Kind
	To   Kind
}

// KMeta is a kind metavariable for kind inference/unification.
type KMeta struct {
	ID int
}

// KVar is a kind variable introduced by explicit kind annotation in \ binders.
// e.g., \ (k: Kind). \ (f: k -> Type). ...
type KVar struct {
	Name string
}

// KSort is the sort of kinds — the kind of kinds.
// Used in \ binders: \ (k: Kind). ...
type KSort struct{}

func (KType) kindNode()       {}
func (KRow) kindNode()        {}
func (KConstraint) kindNode() {}
func (KData) kindNode()       {}
func (*KArrow) kindNode()     {}
func (*KMeta) kindNode()      {}
func (KVar) kindNode()        {}
func (KSort) kindNode()       {}

func (KType) Equal(other Kind) bool {
	_, ok := other.(KType)
	return ok
}

func (KRow) Equal(other Kind) bool {
	_, ok := other.(KRow)
	return ok
}

func (KConstraint) Equal(other Kind) bool {
	_, ok := other.(KConstraint)
	return ok
}

func (k KData) Equal(other Kind) bool {
	o, ok := other.(KData)
	return ok && k.Name == o.Name
}

func (k *KArrow) Equal(other Kind) bool {
	o, ok := other.(*KArrow)
	if !ok {
		return false
	}
	return k.From.Equal(o.From) && k.To.Equal(o.To)
}

func (k *KMeta) Equal(other Kind) bool {
	o, ok := other.(*KMeta)
	return ok && k.ID == o.ID
}

func (k KVar) Equal(other Kind) bool {
	o, ok := other.(KVar)
	return ok && k.Name == o.Name
}

func (KSort) Equal(other Kind) bool {
	_, ok := other.(KSort)
	return ok
}

func (KType) String() string       { return "Type" }
func (KRow) String() string        { return "Row" }
func (KConstraint) String() string { return "Constraint" }
func (k KData) String() string     { return k.Name }
func (k *KArrow) String() string {
	from := k.From.String()
	if _, ok := k.From.(*KArrow); ok {
		from = "(" + from + ")"
	}
	return fmt.Sprintf("%s -> %s", from, k.To.String())
}

func (k *KMeta) String() string { return fmt.Sprintf("?k%d", k.ID) }
func (k KVar) String() string   { return k.Name }
func (KSort) String() string    { return "Kind" }

// KindSubst substitutes [varName := replacement] in a kind.
func KindSubst(k Kind, varName string, replacement Kind) Kind {
	switch kk := k.(type) {
	case KVar:
		if kk.Name == varName {
			return replacement
		}
		return kk
	case *KArrow:
		newFrom := KindSubst(kk.From, varName, replacement)
		newTo := KindSubst(kk.To, varName, replacement)
		if newFrom == kk.From && newTo == kk.To {
			return kk
		}
		return &KArrow{From: newFrom, To: newTo}
	default:
		return k
	}
}

// Arity returns the number of arguments a kind accepts.
func Arity(k Kind) int {
	if ka, ok := k.(*KArrow); ok {
		return 1 + Arity(ka.To)
	}
	return 0
}

// ResultKind returns the kind after all arguments are applied.
func ResultKind(k Kind) Kind {
	if ka, ok := k.(*KArrow); ok {
		return ResultKind(ka.To)
	}
	return k
}
