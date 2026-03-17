package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// Generated name prefixes for internal variables.
const (
	prefixDict      = "$d"    // dictionary parameters
	prefixDictCV    = "$d_cv" // constraint-variable dictionary parameters
	prefixDictDefer = "$dict" // deferred dictionary placeholders
	prefixPat       = "$pat"  // desugared structured pattern variables
	prefixBind      = "$bind" // anonymous bind variables
	prefixSel       = "$sel"  // class method selectors
)

// CheckConfig provides environment for type checking.
type CheckConfig struct {
	RegisteredTypes map[string]types.Kind
	Assumptions     map[string]types.Type
	Bindings        map[string]types.Type
	GatedBuiltins   map[string]bool
	Trace           CheckTraceHook
	ImportedModules map[string]*ModuleExports
	StrictTypeNames bool // when true, reject unregistered type constructor names
}

// ModuleExports carries the type-level information exported by a compiled module.
type ModuleExports struct {
	Types         map[string]types.Kind      // registered type constructors
	ConTypes      map[string]types.Type      // constructor → full type
	ConInfo       map[string]*DataTypeInfo   // constructor → data type info
	Aliases       map[string]*aliasInfo      // type aliases
	Classes       map[string]*ClassInfo      // class declarations
	Instances     []*InstanceInfo            // instance declarations
	Values        map[string]types.Type      // top-level value types
	PromotedKinds map[string]types.Kind      // DataKinds promotions
	PromotedCons  map[string]types.Kind      // promoted constructors
	TypeFamilies  map[string]*TypeFamilyInfo // type family declarations
	DataDecls     []core.DataDecl            // for evaluator constructor registration
}

// CheckTraceKind classifies trace events.
type CheckTraceKind int

const (
	TraceUnify CheckTraceKind = iota
	TraceSolveMeta
	TraceInfer
	TraceCheck
	TraceInstantiate
	TraceRowUnify
)

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent struct {
	Kind    CheckTraceKind
	Depth   int
	Message string
	Span    span.Span // internal byte offsets (retained for internal use)
	Line    int       // 1-based line number
	Col     int       // 1-based column number
}

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook func(CheckTraceEvent)

// Checker holds mutable state during type checking.
type Checker struct {
	ctx               *Context
	unifier           *Unifier
	errors            *errs.Errors
	source            *span.Source
	freshID           int
	config            *CheckConfig
	conTypes          map[string]types.Type
	conInfo           map[string]*DataTypeInfo
	aliases           map[string]*aliasInfo
	classes           map[string]*ClassInfo
	instances         []*InstanceInfo
	instancesByClass  map[string][]*InstanceInfo
	importedInstances map[*InstanceInfo]bool
	promotedKinds     map[string]types.Kind      // DataKinds: data name → KData
	promotedCons      map[string]types.Kind      // DataKinds: nullary con → KData
	kindVars          map[string]bool            // HKT: kind variables in scope (from \ (k: Kind))
	families          map[string]*TypeFamilyInfo // type family declarations
	reductionDepth    int                        // current type family reduction depth
	deferred          []deferredConstraint
	depth             int
	resolveDepth      int                        // instance resolution recursion depth
	qualifiedScopes   map[string]*qualifiedScope // alias → qualified module scope
	strictTypeNames   bool                       // enabled after declaration processing
}

// qualifiedScope holds a module's exports for qualified name resolution.
type qualifiedScope struct {
	moduleName string         // canonical module name
	exports    *ModuleExports // the module's full exports
}

// DataTypeInfo carries constructor information for exhaustiveness.
type DataTypeInfo struct {
	Name         string
	Constructors []ConInfo
}

// ConInfo is a constructor's name, arity, and optional GADT return type.
type ConInfo struct {
	Name       string
	Arity      int
	ReturnType types.Type // GADT: non-nil if constructor has refined return type
}

type aliasInfo struct {
	params     []string
	paramKinds []types.Kind
	body       types.Type
}

// deferredConstraint records a constraint to be resolved after type inference.
type deferredConstraint struct {
	placeholder   string
	className     string
	args          []types.Type
	s             span.Span
	group         int                         // constraints from same qualified type chain; 0 = ungrouped
	quantified    *types.QuantifiedConstraint // non-nil for quantified constraints
	constraintVar types.Type                  // non-nil for constraint variable entries (Dict reification)
}

// Check type-checks a surface AST program and produces Core IR.
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *errs.Errors) {
	coreProg, _, errors := CheckModule(prog, source, config)
	return coreProg, errors
}

// CheckModule type-checks a program and returns both Core IR and module exports.
func CheckModule(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *ModuleExports, *errs.Errors) {
	if config == nil {
		config = &CheckConfig{}
	}
	ch := &Checker{
		ctx:               NewContext(),
		errors:            &errs.Errors{Source: source},
		source:            source,
		config:            config,
		conTypes:          make(map[string]types.Type),
		conInfo:           make(map[string]*DataTypeInfo),
		aliases:           make(map[string]*aliasInfo),
		classes:           make(map[string]*ClassInfo),
		instancesByClass:  make(map[string][]*InstanceInfo),
		importedInstances: make(map[*InstanceInfo]bool),
		promotedKinds:     make(map[string]types.Kind),
		promotedCons:      make(map[string]types.Kind),
		kindVars:          make(map[string]bool),
		families:          make(map[string]*TypeFamilyInfo),
		qualifiedScopes:   make(map[string]*qualifiedScope),
	}
	ch.unifier = NewUnifierShared(&ch.freshID)
	ch.initContext()
	ch.importModules(prog.Imports)
	coreProgram := ch.checkDecls(prog.Decls)
	exports := ch.ExportModule(coreProgram)
	return coreProgram, exports, ch.errors
}

// importModules injects exported declarations from imported modules into the checker state.
func (ch *Checker) importModules(imports []syntax.DeclImport) {
	if ch.config.ImportedModules == nil {
		if len(imports) > 0 {
			ch.addCodedError(errs.ErrImport, imports[0].S, fmt.Sprintf("unknown module: %s", imports[0].ModuleName))
		}
		return
	}

	seen := make(map[string]bool)      // module names already imported
	aliases := make(map[string]string) // alias → module name (for collision detection)

	for _, imp := range imports {
		// Core is implicit and user-invisible. Selective/qualified Core is an error.
		if imp.ModuleName == "Core" && (imp.Alias != "" || imp.Names != nil) {
			ch.addCodedError(errs.ErrImport, imp.S,
				"Core module cannot be selectively or qualifiedly imported")
			continue
		}

		// Duplicate import detection.
		if seen[imp.ModuleName] {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("duplicate import: %s", imp.ModuleName))
			continue
		}
		seen[imp.ModuleName] = true

		mod, ok := ch.config.ImportedModules[imp.ModuleName]
		if !ok {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("unknown module: %s", imp.ModuleName))
			continue
		}

		switch {
		case imp.Alias != "":
			// Qualified import: import M as N
			if prev, exists := aliases[imp.Alias]; exists {
				ch.addCodedError(errs.ErrImport, imp.S,
					fmt.Sprintf("alias %s already used for module %s", imp.Alias, prev))
				continue
			}
			aliases[imp.Alias] = imp.ModuleName
			ch.qualifiedScopes[imp.Alias] = &qualifiedScope{
				moduleName: imp.ModuleName,
				exports:    mod,
			}
			// Instances always imported (coherence requirement).
			ch.importInstances(mod)

		case imp.Names != nil:
			// Selective import: import M (x, T(..), C(A,B))
			ch.importSelective(mod, imp)

		default:
			// Open import: import M — merge all exports.
			ch.importOpen(mod)
		}
	}
}

// importOpen merges all exports from a module into the checker state (open import).
func (ch *Checker) importOpen(mod *ModuleExports) {
	for name, kind := range mod.Types {
		ch.config.RegisteredTypes[name] = kind
	}
	for name, ty := range mod.ConTypes {
		ch.conTypes[name] = ty
		ch.ctx.Push(&CtxVar{Name: name, Type: ty})
	}
	for name, info := range mod.ConInfo {
		ch.conInfo[name] = info
	}
	for name, alias := range mod.Aliases {
		ch.aliases[name] = alias
	}
	for name, cls := range mod.Classes {
		ch.classes[name] = cls
	}
	ch.importInstances(mod)
	for name, ty := range mod.Values {
		ch.ctx.Push(&CtxVar{Name: name, Type: ty})
	}
	for name, kind := range mod.PromotedKinds {
		ch.promotedKinds[name] = kind
	}
	for name, kind := range mod.PromotedCons {
		ch.promotedCons[name] = kind
	}
	for name, fam := range mod.TypeFamilies {
		ch.families[name] = fam.Clone()
	}
}

// importInstances imports all instances from a module (for coherence).
func (ch *Checker) importInstances(mod *ModuleExports) {
	for _, inst := range mod.Instances {
		if ch.importedInstances[inst] {
			continue
		}
		ch.instances = append(ch.instances, inst)
		ch.instancesByClass[inst.ClassName] = append(ch.instancesByClass[inst.ClassName], inst)
		ch.importedInstances[inst] = true
	}
}

// importSelective imports only the names specified in the import list.
func (ch *Checker) importSelective(mod *ModuleExports, imp syntax.DeclImport) {
	// Instances always imported regardless of selective list (coherence).
	ch.importInstances(mod)

	for _, in := range imp.Names {
		name := in.Name
		found := false

		// Value binding (lowercase name or operator)
		if ty, ok := mod.Values[name]; ok {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
			found = true
		}

		// Type constructor
		if kind, ok := mod.Types[name]; ok {
			ch.config.RegisteredTypes[name] = kind
			found = true

			// Import constructors if HasSub
			if in.HasSub {
				ch.importTypeSubs(mod, name, in)
			}
		}

		// Type alias
		if alias, ok := mod.Aliases[name]; ok {
			ch.aliases[name] = alias
			found = true
		}

		// Class
		if cls, ok := mod.Classes[name]; ok {
			ch.classes[name] = cls
			found = true

			// Import class methods
			if in.HasSub {
				ch.importClassSubs(mod, cls, in)
			} else if !in.HasSub {
				// Bare class name: import all methods
				for _, m := range cls.Methods {
					if ty, ok := mod.Values[m.Name]; ok {
						ch.ctx.Push(&CtxVar{Name: m.Name, Type: ty})
					}
				}
			}
		}

		// Type family
		if fam, ok := mod.TypeFamilies[name]; ok {
			ch.families[name] = fam.Clone()
			found = true
		}

		// Promoted kinds
		if kind, ok := mod.PromotedKinds[name]; ok {
			ch.promotedKinds[name] = kind
			found = true
		}
		if kind, ok := mod.PromotedCons[name]; ok {
			ch.promotedCons[name] = kind
			found = true
		}

		if !found {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("module %s does not export: %s", imp.ModuleName, name))
		}
	}
}

// importTypeSubs imports constructors for a type based on the import name spec.
func (ch *Checker) importTypeSubs(mod *ModuleExports, typeName string, in syntax.ImportName) {
	for conName, info := range mod.ConInfo {
		if info.Name != typeName {
			continue
		}
		if in.AllSubs || containsStr(in.SubList, conName) {
			if ty, ok := mod.ConTypes[conName]; ok {
				ch.conTypes[conName] = ty
				ch.ctx.Push(&CtxVar{Name: conName, Type: ty})
			}
			ch.conInfo[conName] = info
		}
	}
}

// importClassSubs imports class methods based on the import name spec.
func (ch *Checker) importClassSubs(mod *ModuleExports, cls *ClassInfo, in syntax.ImportName) {
	for _, m := range cls.Methods {
		if in.AllSubs || containsStr(in.SubList, m.Name) {
			if ty, ok := mod.Values[m.Name]; ok {
				ch.ctx.Push(&CtxVar{Name: m.Name, Type: ty})
			}
		}
	}
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// ExportModule captures the current checker state as a ModuleExports.
// Names starting with '_' are private and excluded from exports.
func (ch *Checker) ExportModule(prog *core.Program) *ModuleExports {
	values := make(map[string]types.Type)
	for _, b := range prog.Bindings {
		if !isPrivateName(b.Name) {
			values[b.Name] = b.Type
		}
	}
	return &ModuleExports{
		Types:         copyMap(ch.config.RegisteredTypes),
		ConTypes:      copyMap(ch.conTypes),
		ConInfo:       ch.conInfo,
		Aliases:       ch.aliases,
		Classes:       ch.classes,
		Instances:     ch.instances,
		Values:        values,
		PromotedKinds: copyMap(ch.promotedKinds),
		PromotedCons:  copyMap(ch.promotedCons),
		TypeFamilies:  cloneFamilies(ch.families),
		DataDecls:     prog.DataDecls,
	}
}

// isPrivateName reports whether a name is module-private (starts with '_').
// Compiler-generated names (containing '$') are always internal and never exported.
func isPrivateName(name string) bool {
	return len(name) > 0 && name[0] == '_'
}

func copyMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (ch *Checker) initContext() {
	// Host-declared bindings.
	for name, ty := range ch.config.Bindings {
		ch.ctx.Push(&CtxVar{Name: name, Type: ty})
	}
	// Built-in identifiers.
	for name, ty := range builtinTypes {
		if gatedBuiltins[name] {
			if ch.config.GatedBuiltins != nil && ch.config.GatedBuiltins[name] {
				ch.ctx.Push(&CtxVar{Name: name, Type: ty})
			}
		} else {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
	}
	// Assumptions.
	for name, ty := range ch.config.Assumptions {
		ch.ctx.Push(&CtxVar{Name: name, Type: ty})
	}
	// Registered opaque types.
	if ch.config.RegisteredTypes == nil {
		ch.config.RegisteredTypes = make(map[string]types.Kind)
	}
	// Built-in type constructors.
	ch.config.RegisteredTypes["Record"] = &types.KArrow{From: types.KRow{}, To: types.KType{}}
}

func (ch *Checker) fresh() int {
	ch.freshID++
	return ch.freshID
}

func (ch *Checker) freshMeta(k types.Kind) *types.TyMeta {
	id := ch.fresh()
	return &types.TyMeta{ID: id, Kind: k}
}

func (ch *Checker) freshSkolem(name string, k types.Kind) *types.TySkolem {
	id := ch.fresh()
	return &types.TySkolem{ID: id, Name: name, Kind: k}
}

func (ch *Checker) freshKindMeta() *types.KMeta {
	id := ch.fresh()
	return &types.KMeta{ID: id}
}

func (ch *Checker) mkType(name string) types.Type {
	return &types.TyCon{Name: name}
}

func (ch *Checker) errorPair(s span.Span) (types.Type, core.Core) {
	return &types.TyError{S: s}, &core.Var{Name: "<error>", S: s}
}

// saveUnifierState snapshots the unifier state for later rollback.
func (ch *Checker) saveUnifierState() UnifierSnapshot {
	return ch.unifier.Snapshot()
}

// restoreUnifierState rolls back the unifier to a previously saved snapshot.
func (ch *Checker) restoreUnifierState(snap UnifierSnapshot) {
	ch.unifier.Restore(snap)
}

// withTrial runs fn in a trial unification scope. If fn returns false,
// the unifier state is rolled back to the snapshot taken before fn was called.
func (ch *Checker) withTrial(fn func() bool) bool {
	saved := ch.saveUnifierState()
	if fn() {
		return true
	}
	ch.restoreUnifierState(saved)
	return false
}

// tryUnify attempts to unify a and b, rolling back on failure.
func (ch *Checker) tryUnify(a, b types.Type) bool {
	return ch.withTrial(func() bool {
		return ch.unifier.Unify(a, b) == nil
	})
}

func (ch *Checker) addCodedError(code errs.Code, s span.Span, msg string) {
	ch.errors.Add(&errs.Error{
		Code:    code,
		Phase:   errs.PhaseCheck,
		Span:    s,
		Message: msg,
	})
}

func (ch *Checker) trace(kind CheckTraceKind, s span.Span, format string, args ...any) {
	if ch.config.Trace != nil {
		line, col := ch.source.Location(s.Start)
		ch.config.Trace(CheckTraceEvent{
			Kind:    kind,
			Depth:   ch.depth,
			Message: fmt.Sprintf(format, args...),
			Span:    s,
			Line:    line,
			Col:     col,
		})
	}
}
