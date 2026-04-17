// Check helpers tests — shared test helpers for internal/check/ tests.
// Does NOT cover: direct feature tests (see adjacent test files).
package check

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/compiler/desugar"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkSource parses and type-checks source, failing on any error.
func checkSource(t *testing.T, source string, config *CheckConfig) *ir.Program {
	t.Helper()
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatal("lex errors:", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	desugar.Program(ast)
	prog, checkErrs := Check(ast, src, config)
	if checkErrs.HasErrors() {
		t.Fatal("check errors:", checkErrs.Format())
	}
	return prog
}

// checkSourceExpectError parses and type-checks source, expecting at least one error.
// Returns the formatted error string.
func checkSourceExpectError(t *testing.T, source string, config *CheckConfig) string {
	t.Helper()
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatal("lex errors:", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	desugar.Program(ast)
	_, checkErrs := Check(ast, src, config)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check errors, got none")
	}
	return checkErrs.Format()
}

// checkSourceExpectCode parses and type-checks source, expecting at least one error
// with the given error code. Returns the formatted error string.
func checkSourceExpectCode(t *testing.T, source string, config *CheckConfig, code diagnostic.Code) string {
	t.Helper()
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatal("lex errors:", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	desugar.Program(ast)
	_, checkErrs := Check(ast, src, config)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check errors, got none")
	}
	found := false
	for _, e := range checkErrs.Errs {
		if e.Code == code {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error code E%04d, got: %s", code, checkErrs.Format())
	}
	return checkErrs.Format()
}

// checkSourceTryBoth parses and type-checks source, returning the error string
// (empty if no errors). Useful when checking both success and failure paths.
func checkSourceTryBoth(t *testing.T, source string, config *CheckConfig) string {
	t.Helper()
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatal("lex errors:", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	desugar.Program(ast)
	_, checkErrs := Check(ast, src, config)
	if checkErrs.HasErrors() {
		return checkErrs.Format()
	}
	return ""
}

// checkSourceNoPanic compiles source and verifies no panic occurs.
// Errors are acceptable; panics are not.
func checkSourceNoPanic(t *testing.T, source string, config *CheckConfig) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC during type checking: %v", r)
		}
	}()
	if config == nil {
		config = &CheckConfig{}
	}
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		return // lex error is fine, not a bug
	}
	if es.HasErrors() {
		return // parse error is fine, not a bug
	}
	desugar.Program(ast)
	Check(ast, src, config)
}

// setupCheckerWithPrelude creates a Checker with minimal Num/Eq/Show instances
// registered, sufficient for solver unit tests.
func setupCheckerWithPrelude(t *testing.T) *Checker {
	t.Helper()
	ch := newTestChecker()
	// Register ground instances: Num Int, Eq Int, Eq Bool, Show Int.
	instances := []*InstanceInfo{
		{ClassName: "Num", TypeArgs: []types.Type{&types.TyCon{Name: "Int"}}, DictBindName: "Num$Int"},
		{ClassName: "Eq", TypeArgs: []types.Type{&types.TyCon{Name: "Int"}}, DictBindName: "Eq$Int"},
		{ClassName: "Eq", TypeArgs: []types.Type{&types.TyCon{Name: "Bool"}}, DictBindName: "Eq$Bool"},
		{ClassName: "Show", TypeArgs: []types.Type{&types.TyCon{Name: "Int"}}, DictBindName: "Show$Int"},
	}
	for _, inst := range instances {
		ch.reg.RegisterInstance(inst)
	}
	return ch
}

// newTestChecker creates a minimal Checker for unit tests.
func newTestChecker() *Checker {
	ch := &Checker{
		CheckState: &CheckState{
			ctx:    NewContext(),
			errors: &diagnostic.Errors{Source: span.NewSource("test", "")},
			config: &CheckConfig{},
		},
		reg: &Registry{
			typeKinds:         make(map[string]types.Type),
			conModules:        make(map[string]string),
			conTypes:          make(map[string]types.Type),
			conInfo:           make(map[string]*DataTypeInfo),
			dataTypeByName:    make(map[string]*DataTypeInfo),
			aliases:           make(map[string]*AliasInfo),
			classes:           make(map[string]*ClassInfo),
			dictToClass:       make(map[string]string),
			instancesByClass:  make(map[string][]*InstanceInfo),
			importedInstances: make(map[*InstanceInfo]bool),
			promotedKinds:     make(map[string]types.Type),
			promotedCons:      make(map[string]types.Type),
			KindScope: KindScope{
				kindVars:  make(map[string]bool),
				levelVars: make(map[string]bool),
			},
			families: make(map[string]*TypeFamilyInfo),
		},
	}
	ch.budget = budget.NewCheckBudget(context.Background())
	ch.budget.SetTFStepLimit(family.MaxReductionWork)
	ch.budget.SetSolverStepLimit(100_000)
	ch.budget.SetResolveDepthLimit(64)
	ch.unifier = unify.NewUnifierShared(&ch.freshID, &types.TypeOps{})
	ch.unifier.Budget = ch.budget
	ch.solver = solve.New(ch, &types.TypeOps{})
	ch.solverLevel = ch.solver.Level
	return ch
}

// typeHasMeta returns true if the type contains an unsolved TyMeta. Test-only helper.
func typeHasMeta(ty types.Type) bool {
	return types.AnyType(ty, func(t types.Type) bool {
		_, ok := t.(*types.TyMeta)
		return ok
	})
}
