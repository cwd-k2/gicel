package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
	"github.com/cwd-k2/gicel/internal/runtime/vm"
)

// initBuiltinGlobals registers builtin names and data constructors in the
// globals map. Builtin values (pure, bind, fix, rec) are placeholder
// nils here; precompileVM replaces them with VMClosures. `thunk` and
// `force` are syntactic special forms with no runtime value.
func (r *Runtime) initBuiltinGlobals(gatedBuiltins map[string]bool) {
	globals := make(map[string]eval.Value, 8)
	// Register builtin names (values are set by precompileVM).
	for _, name := range []string{"pure", "bind"} {
		globals[name] = nil
	}
	if gatedBuiltins["fix"] {
		globals["fix"] = nil
	}
	if gatedBuiltins["rec"] {
		globals["rec"] = nil
	}

	for _, me := range r.moduleEntries {
		for _, d := range me.prog.DataDecls {
			for _, con := range d.Cons {
				globals[ir.QualifiedKey(me.name, con.Name)] = &eval.ConVal{Con: con.Name}
			}
		}
	}

	for _, d := range r.prog.DataDecls {
		for _, con := range d.Cons {
			globals[con.Name] = &eval.ConVal{Con: con.Name}
		}
	}

	r.builtinGlobals = globals
}

// buildGlobalSlots collects all global names and assigns each a slot index.
// The resulting map is used by the evaluator to resolve global Var nodes
// (Index == -1) by name at eval time. No IR mutation occurs — the program
// IR remains immutable after compilation.
func (r *Runtime) buildGlobalSlots() {
	slots := make(map[string]int, len(r.builtinGlobals)+len(r.bindings))

	// Assign slots from builtinGlobals (builtins + constructors).
	// Sort keys for deterministic slot assignment — Go map iteration is
	// non-deterministic, and varying slot indices can expose latent bugs.
	builtinKeys := make([]string, 0, len(r.builtinGlobals))
	for k := range r.builtinGlobals {
		builtinKeys = append(builtinKeys, k)
	}
	sort.Strings(builtinKeys)
	for _, k := range builtinKeys {
		slots[k] = len(slots)
	}
	// Host binding names (values provided at RunWith time).
	hostKeys := make([]string, 0, len(r.bindings))
	for name := range r.bindings {
		hostKeys = append(hostKeys, name)
	}
	sort.Strings(hostKeys)
	for _, name := range hostKeys {
		if _, ok := slots[name]; !ok {
			slots[name] = len(slots)
		}
	}
	// Module binding names.
	for _, me := range r.moduleEntries {
		for _, b := range me.prog.Bindings {
			key := ir.QualifiedKey(me.name, b.Name)
			if _, ok := slots[key]; !ok {
				slots[key] = len(slots)
			}
		}
	}
	// Main binding names.
	for _, b := range r.prog.Bindings {
		if _, ok := slots[b.Name]; !ok {
			slots[b.Name] = len(slots)
		}
	}

	r.globalSlots = slots
	r.numGlobals = len(slots)
}

// buildGlobalArray creates the global value array from the slot map.
// Builtin values and host bindings are filled; module/main bindings
// are left nil (filled by evalPrecompiledBindings).
func (r *Runtime) buildGlobalArray(hostBindings map[string]eval.Value) ([]eval.Value, error) {
	arr := make([]eval.Value, r.numGlobals)
	for k, v := range r.builtinGlobals {
		arr[r.globalSlots[k]] = v
	}
	var missing []string
	for name := range r.bindings {
		v, ok := hostBindings[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		arr[r.globalSlots[name]] = v
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing host binding(s): %s (declared with DeclareBinding but not provided in RunOptions.Bindings)", strings.Join(missing, ", "))
	}
	// Warn about undeclared bindings (typos in the Bindings map).
	for name := range hostBindings {
		if _, isDeclared := r.bindings[name]; !isDeclared {
			if _, isBuiltin := r.builtinGlobals[name]; !isBuiltin {
				msg := fmt.Sprintf("gicel: warning: host binding %q was provided but not declared (possible typo)\n", name)
				if r.warnFunc != nil {
					r.warnFunc(msg)
				} else {
					fmt.Fprint(os.Stderr, msg)
				}
			}
		}
	}
	return arr, nil
}

// precompileVM compiles all module bindings, main bindings, entry expression,
// and builtins to bytecode at NewRuntime time. This avoids per-execution
// compilation overhead in RunWith.
//
// Pool overflow panics from the bytecode compiler (vm.PoolOverflowError) are
// caught and converted to a structured CompileError with code
// ErrCompilePoolOverflow. The recover deliberately type-asserts on
// *vm.PoolOverflowError so that any other panic shape (internal compiler
// invariant violations, nil dereferences) continues to propagate as a real
// crash — those are bugs, not user-recoverable conditions.
func (r *Runtime) precompileVM(gates map[string]bool) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if poe, isOverflow := rec.(*vm.PoolOverflowError); isOverflow {
				diags := &diagnostic.Errors{Source: r.source}
				diags.Add(&diagnostic.Error{
					Code:    diagnostic.ErrCompilePoolOverflow,
					Phase:   diagnostic.PhaseCheck,
					Message: poe.Error(),
				})
				err = &CompileError{errs: diags}
				return
			}
			panic(rec) // real bug — propagate
		}
	}()

	compiler := vm.NewCompiler(r.globalSlots, r.source)

	// Pre-pass: register prim-alias bindings (assumption declarations whose
	// IR expression is a bare 0-arg *ir.PrimOp) so that compileApp can emit
	// direct saturated OpPrim/OpEffectPrim calls instead of loading the
	// PrimVal stub and walking the OpApply partial-application chain. This
	// is the natural cross-module locus — precompileVM has every module
	// and main binding in scope simultaneously.
	for _, me := range r.moduleEntries {
		for _, b := range me.sortedBindings {
			if po, ok := b.Expr.(*ir.PrimOp); ok && len(po.Args) == 0 && po.Arity > 0 {
				compiler.RecordGlobalPrim(ir.QualifiedKey(me.name, b.Name), po.Name, po.Arity, po.Effectful)
			}
		}
	}
	for _, b := range r.sortedMainBindings {
		if po, ok := b.Expr.(*ir.PrimOp); ok && len(po.Args) == 0 && po.Arity > 0 {
			compiler.RecordGlobalPrim(b.Name, po.Name, po.Arity, po.Effectful)
		}
	}

	// Compile builtins. compileBuiltinLam installs per-builtin annots via
	// SetFVAnnots, so we must re-install the main annots afterward.
	r.vmBuiltins = vm.CompileBuiltinGlobals(compiler,
		gates["fix"], gates["rec"])

	// Compile module bindings.
	r.vmModuleProtos = make([][]vmBindingProto, len(r.moduleEntries))
	for i, me := range r.moduleEntries {
		compiler.SetSource(me.source)
		compiler.SetFVAnnots(me.annots)
		protos := make([]vmBindingProto, len(me.sortedBindings))
		for j, b := range me.sortedBindings {
			p := compiler.CompileBinding(b)
			protos[j] = vmBindingProto{name: b.Name, proto: p, generated: b.Generated}
		}
		r.vmModuleProtos[i] = protos
	}

	// Compile main bindings (non-entry).
	compiler.SetSource(r.source)
	compiler.SetFVAnnots(r.annots)
	for _, b := range r.sortedMainBindings {
		if b.Name != r.entryName {
			p := compiler.CompileBinding(b)
			r.vmMainProtos = append(r.vmMainProtos, vmBindingProto{
				name: b.Name, proto: p, generated: b.Generated,
			})
		}
	}

	// Compile entry expression.
	if r.entryExpr != nil {
		r.vmEntryProto = compiler.CompileExpr(r.entryExpr)
	}

	// Resolve primitive implementations at link time.
	// After this, OpPrim/OpEffectPrim use Proto.ResolvedPrims[nameIdx]
	// instead of runtime map lookups.
	for _, mp := range r.vmModuleProtos {
		for _, bp := range mp {
			bp.proto.ResolvePrims(r.prims)
		}
	}
	for _, bp := range r.vmMainProtos {
		bp.proto.ResolvePrims(r.prims)
	}
	if r.vmEntryProto != nil {
		r.vmEntryProto.ResolvePrims(r.prims)
	}
	return nil
}

// evalPrecompiledBindings runs pre-compiled binding protos on the VM.
func (r *Runtime) evalPrecompiledBindings(machine *vm.VM, protos []vmBindingProto, modulePrefix string, globalArray []eval.Value, obs *eval.ExplainObserver) error {
	type slotCell struct {
		slot int
		key  string
		cell *eval.IndirectVal
	}
	cells := make([]slotCell, len(protos))
	for i, bp := range protos {
		key := bp.name
		if modulePrefix != "" {
			key = ir.QualifiedKey(modulePrefix, bp.name)
		}
		slot := r.globalSlots[key]
		cell := &eval.IndirectVal{}
		globalArray[slot] = cell
		cells[i] = slotCell{slot, key, cell}
	}
	userVisible := modulePrefix == ""
	for i, bp := range protos {
		if userVisible {
			obs.Section(bp.name)
		}
		result, err := machine.Run(bp.proto, eval.NewCapEnv(nil))
		if err != nil {
			if budget.IsLimitError(err) || bp.generated.IsGenerated() {
				return err
			}
			return fmt.Errorf("evaluating %s: %w", bp.name, err)
		}
		v := result.Value
		if clo, ok := v.(*eval.VMClosure); ok {
			clo.Name = bp.name
			if !userVisible {
				obs.MarkInternal(bp.name)
			}
		}
		val := v
		cells[i].cell.Ref = &val
		globalArray[cells[i].slot] = v
	}
	return nil
}
