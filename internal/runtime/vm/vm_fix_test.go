// VM fix/rec tests — self-referential closures and thunks.
// Does NOT cover: general execution (vm_test.go), compiler (compiler_test.go).
package vm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestVMFixSimple(t *testing.T) {
	// fix (\self n. n) 42
	body := &ir.Var{Name: "n", Index: 0}
	lam := &ir.Lam{Param: "n", Body: body}
	fix := &ir.Fix{Name: "self", Body: lam}
	app := &ir.App{Fun: fix, Arg: &ir.Lit{Value: int64(42)}}

	c := NewCompiler(nil, nil)
	annotate(c, app)
	proto := c.CompileExpr(app)

	// Dump bytecode
	t.Log("Top-level bytecode:")
	dumpProto(t, proto, "  ")
	for i, p := range proto.Protos {
		t.Logf("Proto[%d]: NumLocals=%d, Captures=%v, Params=%v, FixSelfSlot=%d, IsThunk=%v",
			i, p.NumLocals, p.Captures, p.Params, p.FixSelfSlot, p.IsThunk)
		dumpProto(t, p, "    ")
	}

	// Execute
	b := budget.New(context.Background(), 10000, 100)
	b.SetAllocLimit(100 * 1024 * 1024)
	machine := NewVM(VMConfig{
		Globals:     make([]eval.Value, 0),
		GlobalSlots: map[ir.VarKey]int{},
		Prims:       eval.NewPrimRegistry(),
		Budget:      b,
		Ctx:         context.Background(),
	})
	result, err := machine.Run(proto, eval.EmptyCapEnv())
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	assertHostVal(t, result.Value, int64(42))
}

func dumpProto(t *testing.T, proto *Proto, indent string) {
	t.Helper()
	for i := 0; i < len(proto.Code); {
		op := Opcode(proto.Code[i])
		size := InstructionSize(op)
		switch size {
		case 1:
			t.Logf("%s%3d: %s", indent, i, op)
		case 3:
			operand := DecodeU16(proto.Code, i+1)
			t.Logf("%s%3d: %s %d", indent, i, op, operand)
		case 4:
			a := DecodeU16(proto.Code, i+1)
			b := proto.Code[i+3]
			t.Logf("%s%3d: %s %d %d", indent, i, op, a, b)
		case 5:
			a := DecodeU16(proto.Code, i+1)
			b := DecodeU16(proto.Code, i+3)
			t.Logf("%s%3d: %s %d %d", indent, i, op, a, b)
		default:
			t.Logf("%s%3d: %s (size=%d)", indent, i, op, size)
		}
		i += size
	}
	t.Logf("%sConstants: %v", indent, fmtConstants(proto.Constants))
	t.Logf("%sStrings: %v", indent, proto.Strings)
}

func fmtConstants(cs []eval.Value) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = fmt.Sprintf("%v", c)
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}

// containsOp returns true if the given opcode appears anywhere in the
// proto's bytecode. Does not recurse into child protos.
func containsOp(p *Proto, want Opcode) bool {
	for i := 0; i < len(p.Code); {
		op := Opcode(p.Code[i])
		if op == want {
			return true
		}
		i += InstructionSize(op)
	}
	return false
}

// containsOpAnywhere walks the proto and all nested child protos,
// reporting true if the opcode is found in any of them.
func containsOpAnywhere(p *Proto, want Opcode) bool {
	if containsOp(p, want) {
		return true
	}
	for _, child := range p.Protos {
		if containsOpAnywhere(child, want) {
			return true
		}
	}
	return false
}

// TestVMFixRecurseSelfTailBytecode pins that a saturated self-call in
// tail position inside a fix body compiles to OpTailRecurseSelf (not
// OpTailApplyN). This is the specialized dispatch introduced for
// recursion-heavy workloads where type/arity dispatch is redundant.
func TestVMFixRecurseSelfTailBytecode(t *testing.T) {
	// fix (\self. \n. self n) — saturated 1-arg tail self-call.
	// Infinite recursion at runtime (no base case); we only compile.
	selfCall := &ir.App{
		Fun: &ir.Var{Name: "self", Index: 0},
		Arg: &ir.Var{Name: "n", Index: 0},
	}
	lamN := &ir.Lam{Param: "n", Body: selfCall}
	fix := &ir.Fix{Name: "self", Body: lamN}
	c := NewCompiler(nil, nil)
	annotate(c, fix)
	proto := c.CompileExpr(fix)

	if !containsOpAnywhere(proto, OpTailRecurseSelf) {
		t.Error("expected OpTailRecurseSelf for saturated tail self-call, bytecode lacks it")
		dumpProto(t, proto, "  ")
		for i, p := range proto.Protos {
			t.Logf("Proto[%d]:", i)
			dumpProto(t, p, "    ")
		}
	}
	if containsOpAnywhere(proto, OpTailApplyN) || containsOpAnywhere(proto, OpApplyN) {
		t.Error("expected the saturated self-call to skip OpApplyN paths entirely")
	}
}

// TestVMFixRecurseSelfNonTailBytecode pins the non-tail variant. The
// self-call sits inside a primitive application, making it non-tail
// w.r.t. the fix body's return. Compiler must emit OpRecurseSelf.
func TestVMFixRecurseSelfNonTailBytecode(t *testing.T) {
	// fix (\self. \n. _inc (self n))
	// The self-call is the argument of _inc, so it is NOT in tail
	// position. Specialization still fires, just as the non-tail variant.
	innerCall := &ir.App{
		Fun: &ir.Var{Name: "self", Index: 0},
		Arg: &ir.Var{Name: "n", Index: 0},
	}
	wrap := &ir.PrimOp{
		Name: "_inc", Arity: 1,
		Args: []ir.Core{innerCall},
	}
	lamN := &ir.Lam{Param: "n", Body: wrap}
	fix := &ir.Fix{Name: "self", Body: lamN}
	c := NewCompiler(nil, nil)
	annotate(c, fix)
	proto := c.CompileExpr(fix)

	if !containsOpAnywhere(proto, OpRecurseSelf) {
		t.Error("expected OpRecurseSelf for saturated non-tail self-call, bytecode lacks it")
		dumpProto(t, proto, "  ")
		for i, p := range proto.Protos {
			t.Logf("Proto[%d]:", i)
			dumpProto(t, p, "    ")
		}
	}
	// The non-tail self-call should never be emitted as tail dispatch.
	if containsOpAnywhere(proto, OpTailRecurseSelf) {
		t.Error("non-tail self-call unexpectedly emitted as OpTailRecurseSelf")
	}
}

// TestVMFixRecurseSelfInnerLambda pins that a self-call living inside
// a nested lambda (child proto) does NOT use the specialized dispatch.
// The child frame has FixSelfSlot == -1, so the fn resolves via a
// capture, not a local fix-self slot — the target proto is the outer
// fix body, not the child frame's proto.
func TestVMFixRecurseSelfInnerLambda(t *testing.T) {
	// fix (\self. \n. (\y. self y) n)
	// The `self y` call is inside an inner lambda (\y. ...). The inner
	// frame's FixSelfSlot is -1; specialization should not fire inside
	// the child proto. The outer frame has a non-self call (Lam applied
	// to n), which also should not fire.
	innerSelfCall := &ir.App{
		Fun: &ir.Var{Name: "self", Index: 0},
		Arg: &ir.Var{Name: "y", Index: 0},
	}
	innerLam := &ir.Lam{Param: "y", Body: innerSelfCall}
	outerBody := &ir.App{Fun: innerLam, Arg: &ir.Var{Name: "n", Index: 0}}
	lamN := &ir.Lam{Param: "n", Body: outerBody}
	fix := &ir.Fix{Name: "self", Body: lamN}
	c := NewCompiler(nil, nil)
	annotate(c, fix)
	proto := c.CompileExpr(fix)

	if containsOpAnywhere(proto, OpRecurseSelf) || containsOpAnywhere(proto, OpTailRecurseSelf) {
		t.Error("inner-lambda self-call unexpectedly specialized (child frame has no fix self slot)")
		dumpProto(t, proto, "  ")
		for i, p := range proto.Protos {
			t.Logf("Proto[%d]:", i)
			dumpProto(t, p, "    ")
		}
	}
}

// TestVMFixRecurseSelfPartialApplication pins that an under-saturated
// self-call (fewer args than the fix body's param count) falls through
// to the general dispatch path. The specialization requires exact
// arity match.
func TestVMFixRecurseSelfPartialApplication(t *testing.T) {
	// fix (\self. \x. \y. self x) — `self x` is a partial application.
	// Multi-param fix with a single-arg self-call must go through the
	// general OpApplyN path because applyN produces a PAPVal.
	selfCall := &ir.App{
		Fun: &ir.Var{Name: "self", Index: 0},
		Arg: &ir.Var{Name: "x", Index: 0},
	}
	lamY := &ir.Lam{Param: "y", Body: selfCall}
	lamX := &ir.Lam{Param: "x", Body: lamY}
	fix := &ir.Fix{Name: "self", Body: lamX}
	c := NewCompiler(nil, nil)
	annotate(c, fix)
	proto := c.CompileExpr(fix)

	if containsOpAnywhere(proto, OpRecurseSelf) || containsOpAnywhere(proto, OpTailRecurseSelf) {
		t.Error("partial self-application unexpectedly specialized")
		dumpProto(t, proto, "  ")
		for i, p := range proto.Protos {
			t.Logf("Proto[%d]:", i)
			dumpProto(t, p, "    ")
		}
	}
}
