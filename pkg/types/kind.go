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

// KArrow is the kind of type constructors (K1 -> K2).
type KArrow struct {
	From Kind
	To   Kind
}

func (KType) kindNode()  {}
func (KRow) kindNode()   {}
func (*KArrow) kindNode() {}

func (KType) Equal(other Kind) bool {
	_, ok := other.(KType)
	return ok
}

func (KRow) Equal(other Kind) bool {
	_, ok := other.(KRow)
	return ok
}

func (k *KArrow) Equal(other Kind) bool {
	o, ok := other.(*KArrow)
	if !ok {
		return false
	}
	return k.From.Equal(o.From) && k.To.Equal(o.To)
}

func (KType) String() string  { return "Type" }
func (KRow) String() string   { return "Row" }
func (k *KArrow) String() string {
	from := k.From.String()
	if _, ok := k.From.(*KArrow); ok {
		from = "(" + from + ")"
	}
	return fmt.Sprintf("%s -> %s", from, k.To.String())
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
