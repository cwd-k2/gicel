# Comprehensive Review

Date: 2026-03-18

Scope:

- Sandbox API
- Embedding API and host boundary
- Type checker
- Evaluator/runtime
- Module/import system
- Code quality and maintainability

## Review Boundary

This review uses the following responsibility boundary.

### GICEL responsibility

The minimum line is the GICEL-controlled pipeline for untrusted GICEL source:

- lex / parse
- type check
- module/import handling
- Core elaboration/optimization
- evaluation of GICEL code under GICEL's own limits and semantics

In other words, when the untrusted party is the AI writing GICEL code, the review treats the GICEL implementation itself as responsible for:

- not panicking on hostile GICEL input
- not hanging unreasonably on hostile GICEL input
- keeping parser/checker/runtime behavior bounded enough for the advertised sandbox use case
- preserving language/module/type-system correctness

### Host responsibility

The Go embedding surface is treated as trusted host code.

That includes:

- `RegisterPrim`
- `DeclareBinding`
- `DeclareAssumption`
- custom `Pack`s
- capability values passed from Go
- any side effects performed by host primitives
- whether custom primitives respect `context.Context`

Under this boundary, a host primitive that blocks forever, performs I/O, ignores `ctx`, or panics is primarily a host-side responsibility, not a defect in GICEL's core language semantics.

### Review interpretation

Because of that boundary:

- findings about parser/checker/runtime behavior on GICEL input remain first-class findings
- findings about custom Go primitives are interpreted mainly as trusted-boundary documentation and API-shape concerns
- documentation should make this trust boundary explicit so "sandbox" is not read as "untrusted arbitrary Go callbacks are also contained"

Verification performed:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`

All three completed successfully at review time.

## Findings

### 1. High: `RunSandbox.Timeout` does not actually bound compilation time

Severity: High

Why it matters:

- The README and sandbox API position `RunSandbox` as the single-call safety boundary for AI-agent or untrusted-code use.
- The timeout is started before compilation, but compilation itself is not cancellable.
- A pathological source program can therefore exceed the configured timeout while still consuming CPU during parse/check/optimize.

Evidence:

- `RunSandbox` creates `ctx` with timeout before compilation in `sandbox.go`, but then calls `eng.NewRuntime(source)` directly without a context-aware compile path.
- Timeout enforcement only becomes active once evaluation begins.

Relevant code:

- `sandbox.go:80-90`
- `engine.go:376-416`

Why this is a real vulnerability path:

- In the sandbox threat model, the attacker controls source text.
- Several checker subsystems already carry explicit security/performance regression tests, which implies hostile inputs are in scope.
- A CPU-heavy compile is enough to degrade service even if runtime limits are correct.

Impact:

- Timeout is weaker than documented for sandbox use.
- A host expecting "compile + execute total is bounded by Timeout" can be misled.

Recommended fix:

- Introduce a context-aware compile pipeline and thread cancellation through lex/parse/check/opt phases, or
- Move sandbox compilation to a separate worker goroutine/process with an externally enforceable deadline, and
- Narrow documentation until the implementation truly enforces total-wall-clock bounds.

Documentation mismatch:

- `README.md:62-64`
- `README.md:149-160`

### 2. High: primitives can bypass sandbox timeout and side-effect guarantees unless they are fully trusted

Severity: High

Why it matters:

- The evaluator checks `ctx.Done()` only at eval-step boundaries.
- Primitive invocation is synchronous and cooperative.
- A primitive that blocks, loops, ignores `ctx`, performs external I/O, or spawns goroutines cannot be forcibly stopped by GICEL.

Evidence:

- Context cancellation is checked in `internal/eval/eval.go:111-117`.
- Primitive calls are executed directly in `callPrim` in `internal/eval/eval_apply.go:62-73`.
- `callPrim` recovers panics, but does not enforce cancellation or wall-clock bounds once inside host code.

Relevant code:

- `internal/eval/eval.go:111-117`
- `internal/eval/eval_apply.go:62-73`
- `internal/eval/eval_apply.go:81-128`

Why this is a real vulnerability path:

- The embedding API explicitly encourages host-registered primitives.
- Once a primitive is exposed to untrusted GICEL code, the actual trust boundary is the primitive implementation, not the language runtime.
- This is especially important for sandbox marketing and agent-facing documentation.

Impact:

- `Timeout` is best-effort for trusted/cooperative primitives, not a hard stop.
- "No side effects by default" remains true for core GICEL, but exposed primitives can trivially reintroduce filesystem/network/process side effects.

Recommended fix:

- Document the trust requirement explicitly: primitives are part of the trusted computing base.
- Consider offering a stricter embedding mode that rejects host primitives for `RunSandbox`.
- For long-running or I/O primitives, provide guidance and helper wrappers that enforce `ctx` checks internally.

Documentation mismatch:

- `README.md:58-64`
- `docs/agent-guide/go-api.md:52-65`

### 3. Medium: selective class import bypasses ambiguity checks for imported methods

Severity: Medium

Why it matters:

- Normal open/selective value imports run through ambiguity detection.
- Bare class selective import uses a separate path that injects all methods without calling `checkAmbiguousName`.
- This can silently shadow an existing imported method with the same name from another module.

Evidence:

- `importOpen` checks `checkAmbiguousName` for values and constructors.
- In `importSelective`, `import M (C)` imports all class methods in the `else` branch without ambiguity validation.
- `importClassSubs` also pushes methods directly without ambiguity validation.

Relevant code:

- `internal/check/import.go:131-159`
- `internal/check/import.go:190-239`
- `internal/check/import.go:282-289`

Impact:

- Import semantics become inconsistent.
- Multi-module code can become order-dependent for class method names.
- This is a module-system correctness issue, not just style.

Recommended fix:

- Route class-method imports through the same ambiguity checks as normal values.
- Add regression tests covering:
  - `import A (C)` plus `import B (f)` where both export method `f`
  - `import A (C(f))` plus `import B (C(f))`

## Architectural Review

This section is intentionally stricter than the findings section. It focuses on whether the codebase is shaped in a way that will remain maintainable as the language grows, not just whether it works today.

### 4. Medium: package boundaries are mostly sensible, but the checker package is becoming a "god subsystem"

Severity: Medium

Assessment:

- The top-level package split is broadly reasonable:
  - `internal/check`
  - `internal/eval`
  - `internal/syntax/parse`
  - `internal/types`
  - `internal/core`
  - `internal/stdlib`
- The problem is not the top-level package map. The problem is that `internal/check` has become the convergence point for too many concerns:
  - bidirectional typing
  - import/module loading semantics
  - class and instance registration
  - evidence handling
  - type family reduction
  - elaboration
  - exhaustiveness
  - resolution and deferred solving

Evidence:

- `internal/check` alone is about 24k lines.
- `Checker` carries a very large amount of mutable cross-cutting state in one struct.
- Many checker files are separated by topic, but the runtime invariants still flow through one mutable object rather than through narrower subsystem interfaces.

Relevant code:

- `internal/check/checker.go`
- `internal/check/bidir.go`
- `internal/check/instance.go`
- `internal/check/resolve.go`
- `internal/check/type_family.go`
- `internal/check/import.go`

Why this matters:

- This shape raises the regression risk of any nontrivial language change.
- It is becoming harder to prove whether an operation is local or whether it mutates subtle global checker state.
- The code is still readable for the original author. It is materially less readable for a new maintainer.

Strict judgment:

- The package structure is acceptable.
- The subsystem structure inside `internal/check` is not scaling cleanly.
- This is not yet a collapse, but it is well past the point where architecture debt should be acknowledged explicitly.

Recommended direction:

- Split the checker into narrower collaboration units with explicit APIs, even if they stay in one package:
  - import/export environment builder
  - class/instance registry
  - family reducer
  - evidence/deferred solver
  - elaboration layer
- Reduce direct mutation of global checker maps from unrelated flows.

### 5. Medium: file splitting exists, but several files are still too large and too semantically dense

Severity: Medium

Assessment:

- The repository has file separation, but some files are "topic buckets" rather than crisp units.
- Large files are not automatically bad; large files that mix multiple policy decisions are.

Notable examples:

- `internal/check/bidir.go` at 773 lines
- `internal/check/type_family.go` at 747 lines
- `internal/check/instance.go` at 519 lines
- `internal/check/resolve.go` at 445 lines
- `internal/syntax/parse/parse_decl.go` at 955 lines
- `internal/syntax/parse/parse_expr.go` at 775 lines
- `internal/eval/eval.go` at 393 lines
- `internal/eval/explain.go` at 396 lines

Strict judgment:

- `internal/eval` is still mostly under control.
- `internal/syntax/parse` is borderline but understandable for a hand-written parser.
- `internal/check` is where file size correlates with too many policy branches, not just too much syntax.

Why this matters:

- Reviewability drops sharply when correctness depends on remembering hidden interaction across 500-900 line files.
- Refactoring becomes harder because helpers are not obviously grouped by invariants.
- Audit quality degrades: the reader can read the code, but cannot easily trust they have seen all relevant state transitions.

Recommended direction:

- Split by invariant, not by syntax category alone.
- Example: `type_family.go` likely wants separation between:
  - declaration/registration
  - reduction
  - injectivity/validation
  - diagnostics/security limits
- Example: `bidir.go` likely wants expression-family splits or helper subfiles for:
  - literals/apps/vars
  - lambda/case/do
  - records/project/update

### 6. Medium: function granularity is uneven; some functions do too much policy work in one pass

Severity: Medium

Examples:

- `Checker.infer`
- `Checker.processInstanceHeader`
- `Checker.processInstanceBody`
- `Checker.importSelective`
- `Engine.RegisterModule`
- `Runtime.execute`

Assessment:

- These functions are not merely long. They also encode multiple semantic phases in one body.
- That increases the chance that an apparently local change introduces behavioral drift elsewhere.

Example concerns:

- `processInstanceHeader` validates class existence, resolves args, lifts kinds, checks context, checks cycles, builds naming, validates methods, checks overlap, mutates associated type families, mutates associated data definitions, and registers the instance.
- `RegisterModule` handles validation, lexing, parsing, implicit import injection, dependency extraction, cycle checks, type-checking, fixity harvesting, annotation, and module registration.

Strict judgment:

- The code is doing too much orchestration inline.
- The behavior is still understandable, but only with sustained concentration.
- This is adequate for a private research prototype; it is not ideal for a library that presents itself as a serious embeddable sandbox/runtime.

Recommended direction:

- Introduce more phase-return objects.
- Prefer:
  - `parseModule`
  - `resolveModuleDeps`
  - `checkModule`
  - `collectModuleFixities`
  - `finalizeModule`
- For checker flows, use smaller helpers that return structured intermediate state rather than mutating everything immediately.

### 7. Medium: cohesion is weaker in public-facing root files than it should be

Severity: Medium

Assessment:

- The root package API is pleasant to consume, but implementation responsibility is spread across several root-level files with mixed concerns:
  - `engine.go`
  - `runtime.go`
  - `sandbox.go`
  - `convert.go`
  - `gicel.go`
- This is workable now, but the root package is acting as both:
  - public facade
  - lifecycle coordinator
  - conversion helper namespace
  - sandbox API

Strict judgment:

- The facade is clean for users.
- Internally, the root package is starting to accumulate too much responsibility.

Recommended direction:

- Keep the public import path unchanged, but consider clearer internal separation:
  - lifecycle/compiler API
  - runtime execution API
  - sandbox convenience API
  - conversion helpers
- At minimum, document which root files are facade-only and which carry substantial orchestration logic.

### 8. Low to Medium: test organization is thorough but unwieldy

Severity: Low to Medium

Assessment:

- The test depth is a major strength.
- The downside is that several enormous test files are becoming hard to navigate and hard to mine for intent.

Evidence:

- `gicel_test.go` is 2876 lines.
- `internal/syntax/parse/parse_test.go` is 2406 lines.
- `internal/stdlib/stdlib_test.go` is 2159 lines.
- Several probe/stress files exceed 1000 lines.

Strict judgment:

- This is not just cosmetic.
- Very large test files reduce failure-locality and make it harder to understand coverage gaps.

Recommended direction:

- Split tests by behavior family, not just package:
  - imports/modules
  - sandbox/resource limits
  - bindings/primitives
  - typeclasses/families
  - runtime/reuse/concurrency

### 9. Medium: exported module state uses shared pointer-rich structures with weak immutability discipline

Severity: Medium

Assessment:

- `ExportModule` clones some maps, but directly shares several pointer-rich structures:
  - `ConInfo`
  - `Aliases`
  - `Classes`
  - `Instances`
- `TypeFamilies` are cloned, which shows the code already recognizes the danger in some areas.
- The asymmetry is the problem: the API shape implies "compiled module exports are stable snapshots", but the implementation is only partially snapshotting.

Relevant code:

- `internal/check/checker.go:190-211`

Why this matters:

- Even if the current code treats these structures immutably in practice, the safety here is by convention, not by construction.
- As the checker evolves, a later mutation to one of these shared objects can leak across module boundaries and create cross-compilation contamination that is difficult to diagnose.

Strict judgment:

- This is survivable today.
- It is still an unnecessary aliasing hazard in one of the most stateful parts of the system.

Recommended direction:

- Either deep-clone exported checker metadata consistently, or
- Make the shared structures explicitly immutable by design and document that invariant.

### 10. Low: parser dot-import is a readability tax that buys too little

Severity: Low

Assessment:

- `internal/syntax/parse/parser.go` uses a dot import of `internal/syntax`.
- The justification comment is understandable, but the trade-off is still poor:
  - worse grepability
  - weaker namespace clarity
  - more cognitive load for readers trying to distinguish parser-local names from syntax AST names

Relevant code:

- `internal/syntax/parse/parser.go:3-10`

Strict judgment:

- This is not a correctness issue.
- It is also not the kind of cleverness that ages well in a nontrivial compiler codebase.

Recommended direction:

- Replace the dot import with explicit qualification or a short alias.

### 11. Low to Medium: public examples normalize panic-oriented and side-effectful embedding style

Severity: Low to Medium

Assessment:

- The public API includes `MustHost`, which is fine as a convenience helper.
- The problem is the surrounding guidance:
  - docs show `MustHost` in custom primitive examples
  - docs also show `fmt.Println` directly inside a primitive
- That nudges users toward embedding patterns that are brittle under malformed input and that bypass the "result-only" sandbox story.

Relevant code:

- `convert.go:118-128`
- `docs/agent-guide/go-api.md:52-65`

Why this matters:

- For an advanced library, examples shape user practice.
- The examples currently bias toward convenience over defensive embedding.

Strict judgment:

- This is not a security bug by itself.
- It is a documentation-quality bug because it trains users toward the least robust pattern at the trust boundary.

Recommended direction:

- Keep `MustHost`, but bias docs toward checked extraction in embedding examples.
- Show primitives that respect `ctx` and return structured errors.
- Avoid normalizing direct stdout side effects in the first custom primitive example.

## Go API Over-Defensiveness

Under the responsibility boundary used in this review, the Go embedding surface is trusted host code. From that perspective, some current defensive behavior is broader than necessary and may reduce debuggability.

### 11A. Medium: `RunSandbox` top-level panic recovery may be too broad for trusted-host failures

Severity: Medium

Assessment:

- `RunSandbox` wraps the entire compile + execute path in a broad `recover`.
- This is useful for hostile GICEL input reaching an internal compiler/runtime bug.
- But it also catches panics originating from trusted host extensions used during execution.

Relevant code:

- `sandbox.go:35-42`

Why this is arguably over-defensive:

- If the host registers buggy primitives or passes malformed host values, swallowing those panics into `gicel: internal panic: ...` reduces signal.
- That makes development-time diagnosis worse and can blur the line between:
  - GICEL implementation bug
  - host primitive bug
  - host misuse of the embedding API

Strict judgment:

- For pure "AI writes GICEL source, host is trusted" positioning, this recovery is broader than ideal.
- It improves resilience for a convenience API, but it also mixes fault domains.

Recommended direction:

- Keep a panic-safe convenience path if desired, but make the behavior explicit.
- Consider distinguishing:
  - internal GICEL panic during parse/check/eval
  - host primitive panic during trusted callback execution
- An alternative is to expose two modes:
  - safe convenience API
  - fail-fast API for host debugging

### 11B. Medium: `callPrim` panic recovery hides trusted primitive failures

Severity: Medium

Assessment:

- `callPrim` recovers any panic from a primitive and turns it into an error string.
- There is an explicit test locking in this behavior.

Relevant code:

- `internal/eval/eval_apply.go:62-73`
- `internal/eval/probe_e_test.go:523-550`

Why this is arguably over-defensive:

- Custom primitives are trusted host code under the stated boundary.
- Recovering and stringifying all panics:
  - weakens fail-fast debugging
  - removes stack information
  - can turn serious host bugs into ordinary-looking runtime errors

Strict judgment:

- This is defensible for a "never crash the process" posture.
- It is less defensible if the stated model is "host code is responsible for its own correctness."

Recommended direction:

- Consider making primitive panic handling configurable.
- At minimum, document that primitive panics are converted to runtime errors and therefore may hide host bugs during development.

### 11C. Low to Medium: nil-return sanitization in primitives is safe but pushes API style toward runtime policing

Severity: Low to Medium

Assessment:

- `callPrim` also converts `nil` value returns into errors.
- This is sensible API hardening, but it continues the same pattern: host misuse is normalized into runtime error rather than surfacing immediately.

Relevant code:

- `internal/eval/eval_apply.go:69-72`
- `internal/eval/probe_e_test.go:542-550`

Strict judgment:

- This is a softer case than panic recovery.
- It is still part of the same design posture: "protect the host from its own embedding mistakes."

Recommended direction:

- Keeping this behavior is reasonable.
- The main need is to document it clearly as host-API sanitization, not language-level safety.

### Bottom line on over-defensiveness

The current Go API errs on the side of:

- preserving process liveness
- smoothing host misuse into ordinary errors

That is not inherently wrong. But given the chosen boundary, it should be described as:

- convenience/safety behavior for embedders

not as:

- evidence that arbitrary host callbacks are part of the sandboxed trust model

The main architectural concern is not that these defenses exist. It is that they currently blur fault ownership unless clearly documented.

## Checker Performance Review

This section focuses on algorithmic shape, hotspot likelihood, and scale risk in `internal/check`. The current suite demonstrates that the checker is not casually slow. The concern is not "it is already unusable"; the concern is "several central operations scale by repeated full-state trial work."

### Overall assessment

The checker is performance-aware:

- there are explicit security/performance tests
- the unifier uses zonk path compression
- type family reduction has both depth and type-size guards
- parser safety limits exist

That said, the checker still relies heavily on:

- linear scans over context/instances
- repeated `Snapshot/Restore` trial unification
- repeated free-variable and substitution passes

This is acceptable at current scale. It is not an obviously scalable architecture for much larger programs, instance sets, or module graphs.

### 12. Medium to High: `Snapshot/Restore`-driven trial unification is the dominant structural cost center

Severity: Medium to High

Assessment:

- Trial unification is used pervasively:
  - `tryUnify`
  - overlap checks
  - ambiguity checks
  - injectivity verification
  - fundep improvement
  - quantified constraint matching
- Each trial snapshots:
  - unifier solutions
  - label contexts
  - kind solutions
  - stuck-family index

Relevant code:

- `internal/check/checker.go:303-335`
- `internal/check/unify/unify.go:112-144`

Why this matters:

- Snapshot cost scales with the current size of the unifier state, not just the local comparison being attempted.
- As the number of metas and row contexts grows, each trial becomes more expensive.
- Because trial scopes are used inside loops over equations or instances, the real cost becomes multiplicative.

Examples of affected flows:

- `verifyInjectivity`
- `instancesOverlap`
- `isAmbiguousInstance`
- `applyFunDepImprovement`
- `resolveQuantifiedConstraint`

Strict judgment:

- This is the single most important checker performance design issue.
- It is not a bug, but it is the main reason the checker may hit a scaling wall before the language design does.

Recommended direction:

- Prefer append-only trail/undo logging for unifier mutations instead of full-map snapshot copies.
- If full rollback remains, at least separate:
  - cheap local solve rollback
  - full checker rollback only when family rework state truly needs it

### 13. Medium: instance resolution is mostly linear search with recursive expansion

Severity: Medium

Assessment:

- `resolveInstance` scans `instancesByClass[className]` linearly.
- Before that, it also linearly scans context entries for:
  - direct dictionaries
  - superclass dictionaries
  - quantified evidence
- It then recursively resolves instance contexts.

Relevant code:

- `internal/check/resolve.go:42-125`
- `internal/check/context.go:48-92`

Why this matters:

- The cost model is roughly:
  - O(context size)
  - plus O(instances for class)
  - plus recursive resolution for matched instance premises
- This is fine for small class environments.
- It becomes less fine when combined with:
  - many imported instances
  - deep superclass chains
  - quantified constraints
  - repeated calls from deferred resolution or elaboration

Strict judgment:

- The implementation is straightforward and auditable.
- It is also intentionally non-indexed beyond `instancesByClass`.
- That is a reasonable first implementation, but no longer a particularly cheap one.

Recommended direction:

- Cache successful instance resolutions for zonked `(className, args)` queries within a single checker run.
- Consider a lightweight index on obvious head constructors to reduce candidate instance scans.

### 14. Medium: functional dependency improvement is best-effort but still potentially scan-heavy

Severity: Medium

Assessment:

- `applyFunDepImprovement` iterates all instances of a class whenever "from" positions are determined.
- For each candidate, it creates a fresh substitution and runs trial unification on the `from` positions.
- It stops at first match, which helps, but worst-case behavior is still linear in instance count per invocation.

Relevant code:

- `internal/check/resolve.go:378-430`

Why this matters:

- Fundep improvement may be invoked repeatedly during resolution.
- Programs with many instances in one class can pay the candidate-scan cost often.
- The current performance test uses 30 instances; that validates sanity, not long-term headroom.

Strict judgment:

- Current implementation is acceptable for the present scale.
- It is not sophisticated enough to dismiss as "solved."

Recommended direction:

- Pre-index instances for classes with fundeps by normalized head constructor patterns in the `from` positions when possible.
- Memoize improvement attempts for already-zonked argument tuples.

### 15. Medium: injectivity verification is knowingly quadratic and built on repeated trial unification

Severity: Medium

Assessment:

- `verifyInjectivity` compares every pair of equations.
- For each pair, it:
  - instantiates pattern variables
  - trial-unifies RHSes
  - if needed, trial-unifies LHS patterns component-wise
- This is O(n^2) in equation count, with a nontrivial constant factor.

Relevant code:

- `internal/check/type_family.go:347-381`
- `internal/check/security_test.go:187-237`

Why this matters:

- The repository already has a performance test because this cost is real.
- As soon as type families accumulate more equations or more complex RHS structure, compile-time cost rises sharply.

Strict judgment:

- The code is honest about the cost.
- The algorithm is still brute-force.

Recommended direction:

- If injective families become a larger feature surface, consider partitioning equations by normalized RHS head before pairwise comparison.
- At minimum, document expected practical limits.

### 16. Medium: ambiguity checks and overlap checks duplicate similar expensive matching work

Severity: Medium

Assessment:

- `isAmbiguousInstance` scans instances and performs rollback unification.
- `instancesOverlap` does the same class of work for overlap detection.
- `resolveQuantifiedConstraint` performs another structurally similar matching loop over instances.

Relevant code:

- `internal/check/deferred.go:115-141`
- `internal/check/instance.go:345-359`
- `internal/check/resolve.go:286-329`

Why this matters:

- These operations are all individually reasonable.
- Together, they suggest the checker lacks a shared "candidate matching" engine or cache.
- The same instance-set can be walked repeatedly in slightly different modes during one compile.

Strict judgment:

- This is duplicated performance policy.
- It is also duplicated complexity, which increases the chance of future semantic skew.

Recommended direction:

- Centralize instance candidate matching.
- Reuse one candidate enumeration path with policy flags for:
  - overlap
  - ambiguity
  - resolution
  - quantified evidence matching

### 17. Low to Medium: context operations are simple but linear, which compounds in complex programs

Severity: Low to Medium

Assessment:

- `Context` is a plain stack with reverse scans.
- `LookupVar`, `LookupVarFull`, `LookupEvidence`, and `Scan` are all O(n).

Relevant code:

- `internal/check/context.go`

Why this matters:

- Ordered contexts make this simplicity attractive.
- But the cost compounds because context scans happen on hot checker paths, especially in resolution.

Strict judgment:

- This is an acceptable trade-off today.
- It would likely become visible in larger modules with heavier elaboration and evidence search.

Recommended direction:

- Keep ordered context semantics, but consider auxiliary side indexes for:
  - recent variable bindings by name
  - evidence entries by class

### 18. Medium: type family reduction is guarded well, but still structurally expensive

Severity: Medium

Assessment:

- `reduceTyFamily` itself is guarded by:
  - `maxReductionDepth`
  - `maxReductionTypeSize`
- That is good and necessary.
- But it still linearly scans equations and performs recursive pattern matching and substitution work for each reduction step.

Relevant code:

- `internal/check/type_family.go:240-345`

Why this matters:

- The code is safe against catastrophic blowup better than many hobby implementations.
- It is not cheap.
- The dominant cost here is not just recursion depth; it is repeated substitution and repeated matching against equation lists.

Strict judgment:

- Safety posture is good.
- Performance posture is defensive, not optimized.

Recommended direction:

- Introduce simple head-symbol partitioning for equations where possible.
- Avoid repeated `collectPatternVars`/kind collection on hot paths if families are stable after registration.

### 19. Low to Medium: repeated free-variable collection likely adds avoidable allocation pressure

Severity: Low to Medium

Assessment:

- Several paths repeatedly call `types.FreeVars`, `types.SubstMany`, `collectPatternVars`, `collectPatternVarKinds`, and similar structural traversals.
- `freshInstanceSubst` in particular traverses type args and context to rebuild free-var substitutions every time it is called.

Relevant code:

- `internal/check/resolve.go:349-376`
- `internal/check/type_family.go:383-420`

Why this matters:

- These are classic "small costs everywhere" functions.
- They are often fine in isolation and painful in aggregate.

Strict judgment:

- Not a crisis.
- Worth revisiting if checker latency starts mattering for editor/LSP use.

Recommended direction:

- Cache per-instance free type variables after elaboration.
- Cache per-family pattern-variable metadata after registration.

### Performance strengths

The checker does several things right:

- It explicitly tests pathological and hostile inputs.
- It bounds family reduction depth and output size.
- The unifier uses zonk path compression.
- The code usually chooses correctness and auditability over clever but fragile optimization.

### Performance bottom line

Current state:

- good enough for the repository's present scale
- thoughtfully defended against catastrophic cases
- not yet architected for large-scale asymptotic comfort

Most likely future performance ceiling:

- repeated full-state trial unification combined with linear instance/context scans

If checker performance becomes a first-order product requirement, the first redesign target should be:

1. rollback strategy in the unifier
2. instance-resolution caching/indexing
3. deduplication of candidate-matching logic
4. memoization of stable metadata derived from instances and families

## Quality Notes

These are not primary defects, but they matter for maintainability and auditability.

### Strong aspects

- The repository has unusually good adversarial testing depth for a language runtime:
  - resource-limit tests
  - stress suites
  - probe tests
  - race test coverage
  - explicit security/performance regression tests in the checker
- Public lifecycle separation is clean:
  - mutable `Engine`
  - immutable `Runtime`
  - per-run options and result objects
- Runtime comments are generally accurate and useful, especially around:
  - TCO trampoline
  - copy-on-write capabilities
  - constructor registration
  - recursion gating

### Areas where the code quality bar drops

- Safety claims are stronger than the implementation boundary actually supports.
  - This is the main credibility problem in the repository.
  - The code is often careful; the problem is that the surrounding claims are not careful enough.
- Some import-system logic is fragmented.
  - Value imports, constructor imports, class imports, and type imports do not all share the same collision path.
  - This is a design smell, not just a missed condition.
  - The module system is small enough that import policy should be centralized, but currently it is not.
- Several internal panic paths remain in non-test code.
  - Some are acceptable for internal invariants.
  - But for a sandbox-oriented project, "it should be unreachable" is not a sufficient audit posture by itself.
  - Each panic path should be explicitly classified as either unreachable-by-user-input, converted to structured error, or guarded at the API boundary.

Examples:

- `engine.go:70`
- `internal/check/check_pattern.go:79`
- `internal/types/evidence.go:301`
- `internal/opt/optimize.go:259`
- `internal/stdlib/embed.go:11`

These are not automatically bugs, but they deserve a short audit table in docs or comments.

### Aesthetic/readability assessment

- Overall style is disciplined and readable.
- Naming is mostly strong and domain-appropriate.
- Comments usually explain invariants rather than paraphrasing syntax.
- The code is serious and intentional; it does not read like accidental growth.

What most hurts readability:

- The checker now carries a large amount of cross-cutting mutable state in `Checker`.
- Import/type-family/class/evidence behavior is spread across many files with implicit coupling.
- A new contributor can still reason about local code, but global invariants are getting harder to recover from source alone.

More bluntly:

- The codebase is closer to "maintainable by an expert steward" than "easy to maintain by a strong generalist Go team".
- That is a valid state for a language implementation, but it should be recognized as such.
- Right now the external posture reads more production-sandbox-polished than the internal architecture fully justifies.

Suggested refactors:

- Consolidate import conflict policy into one shared helper path.
- Document trusted-boundary assumptions for:
  - `RunSandbox`
  - host primitives
  - panic recovery scope
- Add a short architecture note for checker subsystems:
  - imports
  - family reduction
  - evidence/deferred solving
  - instance resolution

## Residual Risk

Even with the current test suite, the main residual risks are:

- compile-time denial of service from hostile source programs
- host primitive misuse undermining sandbox claims
- subtle module/import shadowing regressions as the language grows

## Overall Judgment

This is a strong codebase with real engineering discipline, not a toy. But the strict judgment is:

- The runtime/evaluator quality is better than the sandbox claims around it.
- The checker is technically capable but architecturally over-concentrated.
- The module/import system is serviceable but not clean enough yet to inspire full confidence under ongoing feature growth.
- The test suite is impressive, but its current bulk is starting to hide intent rather than reveal it.

If judged as:

- a language implementation by a technically strong author: good
- an embeddable sandbox whose guarantees should be relied on literally: not yet strict enough
- a codebase ready for steady multi-maintainer evolution without architecture tightening: also not yet

## Summary

The codebase quality is high for an embedded language project, especially in testing depth and runtime structure. The main problems are not random implementation sloppiness; they are boundary-definition and architecture-shape problems:

- the sandbox timeout is not a hard bound for compilation
- primitives are part of the trusted boundary but the public docs do not state that strongly enough
- the import system has at least one correctness hole around class-method ambiguity
- the checker subsystem is accumulating too much responsibility in too little architectural structure

Those are the issues I would fix before strengthening any "safe sandbox" claim further.
