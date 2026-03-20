package engine

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/check"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
)

type compiledModule struct {
	prog           *core.Program
	exports        *check.ModuleExports
	deps           []string
	fixity         map[string]parse.Fixity
	sortedBindings []core.Binding // pre-sorted for evalBindingsCore
	source         *span.Source   // source text for error attribution
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

// Get returns the compiled module for the given name, or nil.
func (s *ModuleStore) Get(name string) *compiledModule {
	return s.modules[name]
}

// Register adds a compiled module and appends it to the ordering.
func (s *ModuleStore) Register(name string, mod *compiledModule) {
	s.modules[name] = mod
	s.order = append(s.order, name)
}

// Order returns the module registration order.
func (s *ModuleStore) Order() []string {
	return s.order
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
func (s *ModuleStore) CollectFixity(p *parse.Parser) {
	for _, name := range s.order {
		p.AddFixity(s.modules[name].fixity)
	}
}
