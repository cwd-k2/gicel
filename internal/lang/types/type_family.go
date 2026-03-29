package types

import "github.com/cwd-k2/gicel/internal/infra/span"

// TyFamilyApp is a saturated type family application.
// It only exists when all parameters are supplied.
// During type checking, it reduces to a concrete type via equation matching.
// It never survives into Core IR — it is fully erased at compile time.
type TyFamilyApp struct {
	Name  string
	Args  []Type
	Kind  Type // result kind (from declaration)
	Flags uint8
	S     span.Span
}

func (*TyFamilyApp) typeNode() {}

func (t *TyFamilyApp) Span() span.Span { return t.S }

func (t *TyFamilyApp) Children() []Type {
	ch := make([]Type, len(t.Args)+1)
	ch[0] = t.Kind
	copy(ch[1:], t.Args)
	return ch
}
