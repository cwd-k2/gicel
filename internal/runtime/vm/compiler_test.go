// Compiler tests — Core IR to bytecode compilation.
// Does NOT cover: vm execution (vm_test.go).
package vm

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// annotate runs the same IR annotation passes the pipeline uses and
// installs the resulting FVAnnotations on the compiler. Call BEFORE
// c.CompileExpr / c.CompileBinding so that compileLam/compileFix/
// compileThunk/compileMerge can look up per-node FV metadata.
func annotate(c *Compiler, expr ir.Core) {
	annots := ir.AnnotateFreeVars(expr)
	ir.AssignIndices(expr, annots)
	c.SetFVAnnots(annots)
}

func TestCompileLit(t *testing.T) {
	expr := &ir.Lit{Value: int64(42)}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	if proto == nil {
		t.Fatal("CompileExpr returned nil")
	}
	if len(proto.Code) == 0 {
		t.Fatal("empty bytecode")
	}
	// Lit is a value form — no OpStep (only reductions emit steps).
	// Should contain: CONST, FORCE_EFFECTFUL, RETURN
	assertOpAt(t, proto, 0, OpConst)
}

func TestCompileVar(t *testing.T) {
	globals := map[string]int{"x": 0}
	expr := &ir.Var{Name: "x", Index: -1, Key: "x"}
	c := NewCompiler(globals, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpLoadGlobal)
	assertOp(t, proto, OpReturn)
}

func TestCompileLam(t *testing.T) {
	// \x. x
	body := &ir.Var{Name: "x", Index: 0}
	expr := &ir.Lam{Param: "x", Body: body}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpClosure)
	if len(proto.Protos) != 1 {
		t.Fatalf("expected 1 nested proto, got %d", len(proto.Protos))
	}
	child := proto.Protos[0]
	if len(child.Params) != 1 || child.Params[0] != "x" {
		t.Errorf("expected params [\"x\"], got %v", child.Params)
	}
}

func TestCompileApp(t *testing.T) {
	fn := &ir.Var{Name: "f", Index: -1, Key: "f"}
	arg := &ir.Lit{Value: int64(1)}
	expr := &ir.App{Fun: fn, Arg: arg}
	globals := map[string]int{"f": 0}
	c := NewCompiler(globals, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	// Non-tail at top level (CompileExpr wraps with ForceEffectful+Return).
	assertOp(t, proto, OpApply)
}

func TestCompileAppNonTail(t *testing.T) {
	// Non-tail: bind result, then return it
	fn := &ir.Var{Name: "f", Index: -1, Key: "f"}
	arg := &ir.Lit{Value: int64(1)}
	app := &ir.App{Fun: fn, Arg: arg}
	// Wrap in a bind to make app non-tail
	bind := &ir.Bind{Comp: app, Var: "r", Body: &ir.Var{Name: "r", Index: 0}}
	globals := map[string]int{"f": 0}
	c := NewCompiler(globals, nil)
	annotate(c, bind)
	proto := c.CompileExpr(bind)

	assertOp(t, proto, OpApply) // non-tail because of bind
	assertOp(t, proto, OpBind)
}

func TestCompileCon(t *testing.T) {
	expr := &ir.Con{Name: "Just", Args: []ir.Core{&ir.Lit{Value: int64(42)}}}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpCon)
}

func TestCompileCaseSimple(t *testing.T) {
	// case x of { True => 1; False => 0 }
	scrut := &ir.Var{Name: "x", Index: -1, Key: "x"}
	cs := &ir.Case{
		Scrutinee: scrut,
		Alts: []ir.Alt{
			{Pattern: &ir.PCon{Con: "True"}, Body: &ir.Lit{Value: int64(1)}},
			{Pattern: &ir.PCon{Con: "False"}, Body: &ir.Lit{Value: int64(0)}},
		},
	}
	globals := map[string]int{"x": 0}
	c := NewCompiler(globals, nil)
	annotate(c, cs)
	proto := c.CompileExpr(cs)

	assertOp(t, proto, OpMatchCon)
}

func TestCompileBind(t *testing.T) {
	comp := &ir.Lit{Value: int64(10)}
	body := &ir.Var{Name: "x", Index: 0}
	expr := &ir.Bind{Comp: comp, Var: "x", Body: body}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpBind)
}

func TestCompileThunkForce(t *testing.T) {
	thunk := &ir.Thunk{Comp: &ir.Lit{Value: int64(7)}}
	expr := &ir.Force{Expr: thunk}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpThunk)
	assertOp(t, proto, OpForce) // non-tail at top level (CompileExpr adds ForceEffectful+Return)
}

func TestCompileFix(t *testing.T) {
	// fix (\self x. x)
	body := &ir.Lam{
		Param: "x",
		Body:  &ir.Var{Name: "x", Index: 0},
	}
	expr := &ir.Fix{Name: "self", Body: body}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpFixClosure)
	if len(proto.Protos) != 1 {
		t.Fatalf("expected 1 proto, got %d", len(proto.Protos))
	}
	child := proto.Protos[0]
	if child.FixSelfSlot < 0 {
		t.Error("expected FixSelfSlot >= 0")
	}
}

func TestCompileRecordLit(t *testing.T) {
	expr := &ir.RecordLit{
		Fields: []ir.Field{
			{Label: "x", Value: &ir.Lit{Value: int64(1)}},
			{Label: "y", Value: &ir.Lit{Value: int64(2)}},
		},
	}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpRecord)
}

func TestCompileRecordProj(t *testing.T) {
	rec := &ir.RecordLit{
		Fields: []ir.Field{
			{Label: "x", Value: &ir.Lit{Value: int64(1)}},
		},
	}
	expr := &ir.RecordProj{Record: rec, Label: "x"}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpRecordProj)
}

func TestCompileTyAppErased(t *testing.T) {
	inner := &ir.Lit{Value: int64(42)}
	expr := &ir.TyApp{Expr: inner}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	// TyApp is erased — should just compile the inner Lit.
	// No TyApp opcode should exist.
	assertOp(t, proto, OpConst) // the literal
	assertOp(t, proto, OpReturn)
}

func TestCompilePrimOp(t *testing.T) {
	expr := &ir.PrimOp{
		Name:  "_add",
		Arity: 2,
		Args:  []ir.Core{&ir.Lit{Value: int64(1)}, &ir.Lit{Value: int64(2)}},
		S:     span.Span{},
	}
	c := NewCompiler(nil, nil)
	annotate(c, expr)
	proto := c.CompileExpr(expr)

	assertOp(t, proto, OpPrim)
}

func TestCompileAppKnownArity(t *testing.T) {
	// fix \self. \x. \y. self x y
	// Inside the fix body, self has known arity 2 and the call is in
	// tail position with matching spine length. The saturated self-call
	// specialization fires, emitting OpTailRecurseSelf in place of the
	// general OpTailApplyN path. Either way, the compiler must not fall
	// back to a sequential OpApply chain.
	selfCall := &ir.App{
		Fun: &ir.App{Fun: &ir.Var{Name: "self", Index: 0}, Arg: &ir.Var{Name: "x", Index: 0}},
		Arg: &ir.Var{Name: "y", Index: 0},
	}
	lamY := &ir.Lam{Param: "y", Body: selfCall}
	lamX := &ir.Lam{Param: "x", Body: lamY}
	fix := &ir.Fix{Name: "self", Body: lamX}
	c := NewCompiler(nil, nil)
	annotate(c, fix)
	proto := c.CompileExpr(fix)

	// Walk every nested proto looking for a batched self-call dispatch
	// (the saturated-self specialization, or the general multi-arg path).
	batchedFound := false
	sequentialApplyOffsets := []int{}
	var walk func(p *Proto)
	walk = func(p *Proto) {
		for i := 0; i < len(p.Code); {
			op := Opcode(p.Code[i])
			switch op {
			case OpTailRecurseSelf, OpRecurseSelf, OpApplyN, OpTailApplyN:
				batchedFound = true
			case OpApply, OpTailApply:
				sequentialApplyOffsets = append(sequentialApplyOffsets, i)
			}
			i += InstructionSize(op)
		}
		for _, child := range p.Protos {
			walk(child)
		}
	}
	walk(proto)
	if !batchedFound {
		t.Error("expected batched self-call dispatch in fix body bytecode, but none found")
	}
	if len(sequentialApplyOffsets) > 0 {
		t.Errorf("unexpected sequential OpApply at offsets %v — the compiler should batch saturated self-calls", sequentialApplyOffsets)
	}
}

func TestCompileAppMultiArg(t *testing.T) {
	// f 1 2 (multi-arg call): the compiler emits a single OpApplyN(2)
	// regardless of f's static arity. applyN dispatches at runtime over
	// every value type and every saturation case, so the compiler does
	// not need to know f's arity to batch the spine.
	globals := map[string]int{"f": 0}
	fn := &ir.Var{Name: "f", Index: -1, Key: "f"}
	app1 := &ir.App{Fun: fn, Arg: &ir.Lit{Value: int64(1)}}
	app2 := &ir.App{Fun: app1, Arg: &ir.Lit{Value: int64(2)}}
	c := NewCompiler(globals, nil)
	annotate(c, app2)
	proto := c.CompileExpr(app2)

	// Should contain OpApplyN, NOT a sequential OpApply chain.
	// (CompileExpr compiles the body with tail=false, so OpApplyN is
	// emitted instead of OpTailApplyN.)
	assertOp(t, proto, OpApplyN)
	for i := 0; i < len(proto.Code); {
		op := Opcode(proto.Code[i])
		if op == OpApply || op == OpTailApply {
			t.Errorf("unexpected sequential OpApply at offset %d for multi-arg call", i)
		}
		i += InstructionSize(op)
	}
}

// --- helpers ---

func assertOpAt(t *testing.T, proto *Proto, offset int, expected Opcode) {
	t.Helper()
	if offset >= len(proto.Code) {
		t.Fatalf("offset %d out of range (code len %d)", offset, len(proto.Code))
	}
	got := Opcode(proto.Code[offset])
	if got != expected {
		t.Errorf("at offset %d: expected %s, got %s", offset, expected, got)
	}
}

func assertOp(t *testing.T, proto *Proto, expected Opcode) {
	t.Helper()
	for i := 0; i < len(proto.Code); {
		op := Opcode(proto.Code[i])
		if op == expected {
			return
		}
		i += InstructionSize(op)
	}
	t.Errorf("opcode %s not found in bytecode", expected)
}
