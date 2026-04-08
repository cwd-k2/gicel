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
		GlobalSlots: map[string]int{},
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
