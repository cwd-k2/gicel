// OpRecurseSelf presence verification — ensures that compiled fib-style
// programs actually emit the specialized self-recursion opcodes.
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/vm"
)

// TestVMFibUsesRecurseSelf pins that the recursive fib defined with
// `fix $ \self n. ...` compiles to bytecode containing OpRecurseSelf
// (the specialized dispatch introduced for saturated self-calls inside
// fix bodies). Regressions here would silently fall back to OpApplyN,
// losing the per-call dispatch savings.
func TestVMFibUsesRecurseSelf(t *testing.T) {
	eng := NewEngine()
	eng.EnableRecursion()
	eng.Use(func() Pack { return stdlib.Prelude }())
	src := `import Prelude
fib :: Int -> Int
fib := fix $ \self n. case n {
  0 => 0;
  1 => 1;
  _ => self (n - 1) + self (n - 2)
}
main := fib 5`
	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	// Walk entry + all main + module bindings, counting each opcode of
	// interest.
	counts := make(map[vm.Opcode]int)
	walk := func(p *vm.Proto) {
		if p == nil {
			return
		}
		var visit func(*vm.Proto)
		visit = func(p *vm.Proto) {
			for i := 0; i < len(p.Code); {
				op := vm.Opcode(p.Code[i])
				counts[op]++
				i += vm.InstructionSize(op)
			}
			for _, c := range p.Protos {
				visit(c)
			}
		}
		visit(p)
	}
	walk(rt.vmEntryProto)
	for _, bp := range rt.vmMainProtos {
		walk(bp.proto)
	}
	for _, mod := range rt.vmModuleProtos {
		for _, bp := range mod {
			walk(bp.proto)
		}
	}

	// Expectation: the fib body has TWO saturated self-calls. Both are
	// argument positions of the outer `+`, so neither is in tail
	// position. Both should appear as OpRecurseSelf (non-tail), and
	// neither should appear as OpApplyN for the self-call itself.
	t.Logf("opcode counts: RecurseSelf=%d TailRecurseSelf=%d ApplyN=%d TailApplyN=%d",
		counts[vm.OpRecurseSelf], counts[vm.OpTailRecurseSelf],
		counts[vm.OpApplyN], counts[vm.OpTailApplyN])

	// Dump the main IR to understand what the checker produced.
	t.Logf("Entry IR:\n%s", rt.Program().Pretty())

	if counts[vm.OpRecurseSelf] < 2 {
		t.Errorf("expected at least 2 OpRecurseSelf (fib's two self-calls), got %d",
			counts[vm.OpRecurseSelf])
	}

	// Sanity: ensure execution still produces the right answer through
	// the specialized path.
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime err: %v", err)
	}
	assertVMInt(t, res, 5) // fib(5) = 5
}
