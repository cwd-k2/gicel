package eval

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gomputation/internal/core"
)

// Value is a runtime value.
type Value interface {
	valueNode()
	String() string
}

// HostVal wraps an opaque Go value injected from the host.
type HostVal struct {
	Inner any
}

// Closure is a function value capturing its definition environment.
type Closure struct {
	Env   *Env
	Param string
	Body  core.Core
}

// ConVal is a fully-applied constructor value.
type ConVal struct {
	Con  string
	Args []Value
}

// ThunkVal is a suspended computation captured by `thunk`.
type ThunkVal struct {
	Env  *Env
	Comp core.Core
}

func (*HostVal) valueNode()  {}
func (*Closure) valueNode()  {}
func (*ConVal) valueNode()   {}
func (*ThunkVal) valueNode() {}

func (v *HostVal) String() string {
	return fmt.Sprintf("HostVal(%v)", v.Inner)
}

func (v *Closure) String() string {
	return fmt.Sprintf("Closure(%s, ...)", v.Param)
}

func (v *ConVal) String() string {
	if len(v.Args) == 0 {
		return v.Con
	}
	args := make([]string, len(v.Args))
	for i, a := range v.Args {
		args[i] = a.String()
	}
	return fmt.Sprintf("(%s %s)", v.Con, strings.Join(args, " "))
}

func (v *ThunkVal) String() string {
	return "ThunkVal(...)"
}
