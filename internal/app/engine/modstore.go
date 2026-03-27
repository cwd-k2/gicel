package engine

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

type compiledModule struct {
	prog           *ir.Program
	exports        *check.ModuleExports
	deps           []string
	fixity         map[string]parse.Fixity
	sortedBindings []ir.Binding // pre-sorted for evalBindingsCore
	source         *span.Source // source text for error attribution
}

// ModuleStore manages compiled modules, their ordering, and dependency checking.
type ModuleStore struct {
	modules   map[string]*compiledModule
	order     []string
	recursion bool
}

func newModuleStore() ModuleStore {
	return ModuleStore{
		modules: make(map[string]*compiledModule),
	}
}

// Has reports whether a module with the given name is registered.
func (s *ModuleStore) Has(name string) bool {
	_, ok := s.modules[name]
	return ok
}

// Register adds a compiled module and appends it to the ordering.
func (s *ModuleStore) Register(name string, mod *compiledModule) {
	s.modules[name] = mod
	s.order = append(s.order, name)
}

// CheckCircularDeps detects circular dependencies before registering a module.
func (s *ModuleStore) CheckCircularDeps(name string, deps []string) error {
	visited := map[string]bool{name: true}
	var walk func(modName string) error
	walk = func(modName string) error {
		if visited[modName] {
			return fmt.Errorf("circular module dependency involving %s", modName)
		}
		visited[modName] = true
		if mod, ok := s.modules[modName]; ok {
			for _, dep := range mod.deps {
				if err := walk(dep); err != nil {
					return err
				}
			}
		}
		visited[modName] = false
		return nil
	}
	for _, dep := range deps {
		if err := walk(dep); err != nil {
			return err
		}
	}
	return nil
}

// Entries returns moduleEntry values in registration order for runtime assembly.
func (s *ModuleStore) Entries() []moduleEntry {
	entries := make([]moduleEntry, 0, len(s.order))
	for _, name := range s.order {
		mod := s.modules[name]
		entries = append(entries, moduleEntry{
			name:           name,
			prog:           mod.prog,
			sortedBindings: mod.sortedBindings,
			source:         mod.source,
		})
	}
	return entries
}

// CollectFixity injects fixity declarations from all registered modules
// into the parser, in registration order.
// Deprecated: use CollectFixityForImports for import-scoped fixity.
func (s *ModuleStore) CollectFixity(p *parse.Parser) {
	for _, name := range s.order {
		p.AddFixity(s.modules[name].fixity)
	}
}

// CollectFixityForImports injects fixity from the transitive closure of
// the given import names into the parser. Only modules reachable through
// the import graph contribute fixity, preventing unimported modules from
// affecting operator precedence.
func (s *ModuleStore) CollectFixityForImports(p *parse.Parser, imports []string) {
	visited := make(map[string]bool)
	var walk func(string)
	walk = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		mod, ok := s.modules[name]
		if !ok {
			return
		}
		p.AddFixity(mod.fixity)
		for _, dep := range mod.deps {
			walk(dep)
		}
	}
	for _, name := range imports {
		walk(name)
	}
}
