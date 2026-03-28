package stdlib

import (
	"errors"
	"reflect"
)

// ErrTypeMismatch is a structured error for stdlib type mismatches.
// Downstream code can inspect Op/Want/Got via errors.As.
type ErrTypeMismatch struct {
	Op   string // operation name (e.g. "length", "json")
	Want string // expected type description (e.g. "List", "Bool")
	Got  any    // actual value received
}

func (e *ErrTypeMismatch) Error() string {
	return e.Op + ": expected " + e.Want + ", got " + reflect.TypeOf(e.Got).String()
}

// ErrMalformed is a structured error for malformed ADT nodes.
type ErrMalformed struct {
	Op   string // operation name
	Kind string // node kind (e.g. "list node", "stream node")
	Con  string // actual constructor name
}

func (e *ErrMalformed) Error() string {
	return e.Op + ": malformed " + e.Kind + ": " + e.Con
}

// errExpected constructs an ErrTypeMismatch.
func errExpected(op, want string, got any) error {
	return &ErrTypeMismatch{Op: op, Want: want, Got: got}
}

// errMalformed constructs an ErrMalformed.
func errMalformed(op, kind, con string) error {
	return &ErrMalformed{Op: op, Kind: kind, Con: con}
}

// Sentinel errors for static messages shared across multiple stdlib files.
var (
	errDivisionByZero = errors.New("division by zero")
	errModuloByZero   = errors.New("modulo by zero")
)
