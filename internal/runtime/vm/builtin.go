package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// CompileBuiltinGlobals compiles the built-in globals (pure, bind, force,
// and optionally fix/rec) into VMClosure values. This replaces
// eval.BuiltinGlobals which produces tree-walker Closures.
func CompileBuiltinGlobals(compiler *Compiler, enableFix, enableRec bool) map[string]eval.Value {
	globals := make(map[string]eval.Value, 8)

	// Compile each builtin as a Lam expression.
	globals["pure"] = compileBuiltinLam(compiler, "pure", "_v",
		&ir.Var{Name: "_v", Index: 0})

	globals["bind"] = compileBuiltinLam(compiler, "bind", "_comp",
		&ir.Lam{
			Param:     "_f",
			FV:        []string{"_comp"},
			FVIndices: []int{0},
			Body:      &ir.App{Fun: &ir.Var{Name: "_f", Index: 0}, Arg: &ir.Var{Name: "_comp", Index: 1}},
		})

	globals["force"] = compileBuiltinLam(compiler, "force", "_thk",
		&ir.Force{Expr: &ir.Var{Name: "_thk", Index: 0}})

	if enableFix {
		globals["fix"] = compileBuiltinLam(compiler, "fix", "_f",
			eval.FixBody())
	}
	if enableRec {
		globals["rec"] = compileBuiltinLam(compiler, "rec", "_f",
			eval.RecBody())
	}

	return globals
}

// compileBuiltinLam compiles a Lam{Param, Body} expression into a VMClosure.
func compileBuiltinLam(compiler *Compiler, name, param string, body ir.Core) eval.Value {
	lam := &ir.Lam{Param: param, Body: body}
	ir.AnnotateFreeVars(lam)
	ir.AssignIndices(lam)
	proto := compiler.CompileBinding(ir.Binding{Name: name, Expr: lam})
	// Run the compiled proto to produce the VMClosure value.
	// Since builtins have no captures and no effects, this is safe.
	// The result will be a VMClosure with the compiled body.

	// Actually, we need to run the proto to get the VMClosure.
	// But that requires a VM instance. Instead, directly construct
	// the VMClosure from the proto's nested prototype.
	if len(proto.Protos) == 1 {
		childProto := proto.Protos[0]
		return &eval.VMClosure{
			Captured: nil,
			Proto:    childProto,
			Name:     name,
		}
	}
	// Invariant: builtin Lam always compiles to exactly one nested proto.
	panic(fmt.Sprintf("vm/builtin: expected 1 nested proto for %s, got %d", name, len(proto.Protos)))
}
