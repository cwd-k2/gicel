# Quality Assurance Review

Perform a systematic quality review of recent changes. Focus on correctness — not "does it pass tests," but "is it right."

## Arguments

$ARGUMENTS — Scope of review. If empty, review all unstaged/staged changes (`git diff HEAD`). If a commit range (e.g., `HEAD~3..HEAD`), review those commits. If a file path, review that file.

## Core Principle

> 「動いているように見える状態」を「正しい状態」と混同する誘惑に抗え。

Tests passing, no errors, plausible output — these indicate "it runs," not "it's correct." Evidence of correctness comes from correspondence with specification, invariant preservation, and faithful error propagation.

## Three Meta-Patterns to Detect

Every issue found should map to one of these meta-patterns:

### A. Detection–Halt Gap (検出と停止の分離)

Error is detected but processing continues. The error is reported (logged, collected) but does not prevent downstream effects.

**Signals:**
- `addError(...)` / `log.Warn(...)` followed by continued execution, not `return`
- Error is appended to a list but the invalid entity is still registered/stored
- `if err != nil { ... }` block that doesn't return, break, or skip

**Question for each error site:** Does this error **prevent** the bad state, or merely **report** it?

### B. Representation–Semantics Conflation (表現と意味の混同)

String/display representation is used for semantic operations (equality, comparison, dispatch).

**Signals:**
- `.String()` / `.Pretty()` / `.Error()` in `==` or `!=` comparisons
- Error message string matching to determine error kind
- Serialized form used as map key where structural identity is needed

**Question:** Is this comparing meaning or appearance?

### C. Optimistic Continuation (楽観的継続)

Problem detected but "probably fine" — processing continues with degraded/default state.

**Signals:**
- Default values returned on error (zero value, empty struct, sentinel -1)
- `recover()` without re-raise or error conversion
- Speculative/trial operations that leave side effects on failure (no rollback)
- `_ = err` or error return value ignored

**Question:** If this error path is exercised, does the caller know? Can the caller distinguish error from success?

## Review Checklist

### 1. Error Path Integrity

For every error-producing site in changed code:

- Does detection **stop** the invalid operation? (Pattern A)
- Or does it report and continue? If continue: explicit justification needed
- Is the error **propagated** to the caller? Or swallowed?
- Does the `(result, error)` contract hold? (When `err != nil`, is `result` safe to use?)

### 2. Speculative Operation Safety

For any trial/speculative operation (try-and-rollback, tentative matching, feature detection):

- Is mutable state saved before the trial?
- Is it restored on failure?
- Are side effects (counter increments, cache writes, registrations) also rolled back?
- Does success correctly commit the changes?

### 3. Semantic vs Representational Operations

Search for `.String()`, `.Pretty()`, `.Error()`, `.Format()` used in:
- `==` / `!=` comparisons
- Map keys
- Switch/case dispatch

Flag any use where structural/semantic comparison should be used instead.

### 4. Silent Failure Detection

Search for patterns that hide errors:

- `_ = someFunc()` or `result, _ := someFunc()` — error discarded
- `default:` / `else` branches returning zero values without error
- `recover()` without logging or error propagation
- Functions that return `(value, error)` where some paths return `(zeroValue, nil)` on failure

### 5. Test Fidelity

For each test in changed code:

- **Behavior vs structure:** Does it test what the code *does*, or how it's *written*?
- **Effect verification:** Does it verify the *consequence* of the error (entity not registered, state unchanged), not just the error code/message?
- **Non-trivial inputs:** Would a subtle bug (off-by-one, wrong variable, missing case) still pass?
- **Change detector check:** If the implementation changes but the behavior is preserved, does this test still pass?

### 6. Specification Consistency

For any code change that affects user-visible behavior:

- Are specs/docs updated to match?
- Are there phantom entries in specs that aren't implemented?
- Are there implemented features not reflected in specs?

## Domain Extensions

When reviewing type checker code (`internal/check/`), additionally apply:

- **Unifier safety:** Classify each `Unify` call as committed (result always wanted), trial (needs save/restore), or semantic (needs `addSemanticUnifyError`). Report trial calls without rollback.
- **Instance coherence:** Overlap/self-cycle detection must return nil (prevent registration), not merely report.
- **Evidence integrity:** Dictionary elaboration must use structural type comparison (`types.Equal`), never `types.Pretty`.
- **Error code coverage:** Cross-reference `internal/errs/error.go` codes against test files. List untested codes.

## Output

Report findings in severity order:

1. **CRITICAL**: Correctness violations — detection-halt gaps, unguarded speculation, representation-semantics conflation that affects logic
2. **MODERATE**: Design consistency — inconsistent error handling patterns, missing test variants, partial integration of new patterns
3. **MINOR**: Maintenance — dead code, documentation drift, style inconsistency

For each finding:
- Meta-pattern (A/B/C)
- File and line
- What's wrong (concrete, not vague)
- Suggested fix
