// Package check implements type checking, elaboration, and constraint
// solving for GICEL. The entry point is Checker, which consumes a
// parsed syntax.Program and produces an ir.Program annotated with
// types, dictionary passing for type classes, and CBPV effect rows.
//
// # Architecture
//
// The package is layered bottom-up. Each subpackage is a closed
// abstraction layer; higher layers compose lower ones.
//
//	unify/      — structural unification, zonking, row unification,
//	              level/label mechanics
//	solve/      — constraint solver: wanted/given sets, dictionary
//	              resolution, implication solving (uses unify)
//	family/     — type family reduction and pattern matching (uses
//	              unify)
//	exhaust/    — pattern exhaustiveness (Maranget) used by decl/case
//	modscope/   — module scope resolution and qualified import
//	              handling
//	env/        — context and environment types shared across the
//	              checker state
//
// The root package (check) contains the orchestrator Checker and
// the per-feature handlers. Files are grouped by concern:
//
//	Orchestrator / state
//	  checker.go              Checker struct, state, budgets, fresh
//	                          name generators
//	  context.go              CtxEntry hierarchy and Context stack
//	  registry.go              environment registry (classes, instances,
//	                          families, types)
//
//	Declaration processing
//	  decl.go                 top-level declaration pipeline
//	  decl_form.go            data/form declarations
//	  decl_generalize.go      binding generalization (MonoLocalBinds)
//	  class.go                type class declarations
//	  instance.go             instance header / context validation
//	  instance_body.go        instance method checking and dictionary
//	                          construction
//	  instance_assoc_family.go associated type family saturation
//	                          for instance method signatures
//	  alias.go                type alias declarations
//	  type_family.go          open/closed type family declarations
//	  export.go               module export handling
//
//	Bidirectional checker
//	  bidir.go                infer/check dispatch over Expr
//	  bidir_app.go            application with spine-based arity
//	  bidir_case.go           case expression checking
//	  bidir_cbpv.go           CBPV layer: Pure/Bind/Thunk/Force/Merge
//	  bidir_fix.go            fix/rec checking
//	  bidir_inst.go           generalization and instantiation
//	  bidir_lookup.go          variable lookup and module qualification
//	  bidir_ql.go             Quick Look impredicative inference
//	  bidir_suggest.go        "did you mean" suggestion hints
//	  elaborate_do.go         do-block elaboration (doStrategy interface,
//	                          doInfer, doChecked)
//	  elaborate_do_monadic.go GIMonad-specific do elaboration (doGraded)
//	  elaborate_block.go      block-expression and pure-bind elaboration
//	                          (localLetGen, inferBlock)
//	  elaborate_record.go     record literal / projection / update
//	  pattern.go              pattern checking and binding extraction
//
//	Type resolution
//	  resolve_type.go         TypeExpr → types.Type
//	  resolve_name.go         name lookup with module disambiguation
//	  resolve_kind.go         kind checking of type expressions
//	  resolve_bridge.go       bridges resolve to checker state
//	  skolem.go               skolem introduction and escape checks
//	  grade.go                grade algebra checking
//	  validate_label.go       label validity for rows and records
//
//	Diagnostics / bridges
//	  diag.go                 structured diagnostic types and unify
//	                          error mapping
//	  solve_bridge.go         wraps solve.Solver with checker state
//	                          (constraint entry/emit)
//	  builtin.go              built-in identifiers (fix, rec, pure,
//	                          bind, thunk, force)
//	  syntax_adapt.go         adapters between syntax and checker
//	                          primitives
//
// # Invariants
//
// Meta variables are single-assignment and threaded through the
// unifier's Zonk. Evidence rows carry FlagMetaFree and
// FlagNoFamilyApp bits that callers must preserve on reconstruction.
// Instance resolution is phase-ordered: headers register before any
// body is checked, so mutual recursion across instances is valid.
package check
