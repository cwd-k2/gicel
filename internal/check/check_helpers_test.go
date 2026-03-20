// Shared test helpers for internal/check/ tests.

package check

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/check/family"
	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
)

// checkSource parses and type-checks source, failing on any error.
func checkSource(t *testing.T, source string, config *CheckConfig) *core.Program {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
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
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	_, checkErrs := Check(ast, src, config)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check errors, got none")
	}
	return checkErrs.Format()
}

// checkSourceExpectCode parses and type-checks source, expecting at least one error
// with the given error code. Returns the formatted error string.
func checkSourceExpectCode(t *testing.T, source string, config *CheckConfig, code errs.Code) string {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
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
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
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
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return // lex error is fine, not a bug
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		return // parse error is fine, not a bug
	}
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
		Session: &Session{
			ctx:    NewContext(),
			errors: &errs.Errors{Source: span.NewSource("test", "")},
			config: &CheckConfig{},
		},
		reg: &Registry{
			typeKinds:         make(map[string]types.Kind),
			conTypes:          make(map[string]types.Type),
			conInfo:           make(map[string]*DataTypeInfo),
			aliases:           make(map[string]*AliasInfo),
			classes:           make(map[string]*ClassInfo),
			instancesByClass:  make(map[string][]*InstanceInfo),
			importedInstances: make(map[*InstanceInfo]bool),
			promotedKinds:     make(map[string]types.Kind),
			promotedCons:      make(map[string]types.Kind),
			kindVars:          make(map[string]bool),
			families:          make(map[string]*TypeFamilyInfo),
		},
		solver: &Solver{},
	}
	ch.budget = budget.New(context.Background(), family.MaxReductionWork, 0)
	ch.unifier = unify.NewUnifierShared(&ch.freshID)
	ch.unifier.Budget = ch.budget
	return ch
}
