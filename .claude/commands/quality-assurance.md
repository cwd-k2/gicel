# Quality Assurance Review

Perform a systematic quality review of recent changes to the Gomputation type checker and evidence system. Focus on correctness-by-construction, not just "does it pass tests."

## Arguments

$ARGUMENTS — Scope of review. If empty, review all unstaged/staged changes (`git diff HEAD`). If a commit range (e.g., `HEAD~3..HEAD`), review those commits. If a file path, review that file.

## Philosophy

- **Theory purity**: Implementation must faithfully represent the theory. String comparison where structural equality is required, unguarded mutation where rollback is required — these are correctness bugs even if tests pass.
- **Fail correctly**: Detected errors must prevent downstream effects. An overlap-detected instance that still gets registered is a hole. A self-cycle that still elaborates is a hole.
- **No silent corruption**: Trial operations (unification, matching) must not leave persistent side effects on failure. Every `ch.unifier.Unify` call in a trial context must have corresponding save/restore.
- **Test what matters**: Tests must verify behavior, not pass trivially. A test that checks for an error code but doesn't verify the error prevents the bad state is incomplete.

## Review Checklist

### 1. Unifier Safety

Search for all `ch.unifier.Unify` call sites. For each, classify:

- **Committed**: The unification result is always wanted (e.g., `subsCheck`, `checkPattern`). No rollback needed.
- **Trial**: The unification is speculative (matching, overlap check, evidence resolution). **Must** have `saveUnifierState`/`restoreUnifierState` around it.
- **Semantic**: The unification checks a shape (computation, arrow, thunk). Error handling should use `addSemanticUnifyError` to preserve root cause for non-trivial failures.

Report any `Unify` call in trial context without save/restore.

### 2. Error Path Integrity

For every validation check that produces an error:

- Does the error **prevent** the invalid entity from being registered/elaborated?
- Or does it merely report and continue? If continue: is there a justification (error recovery for better diagnostics)?
- Specifically check: `processInstanceHeader` returns `nil` on overlap/self-cycle/arity-mismatch? `processClassDecl` rejects invalid classes?

### 3. Structural Correctness of Comparisons

Search for `types.Pretty` used in equality comparisons (not just display). Flag any use that should be `types.Equal` or trial unification.

Pattern to search:
```
types.Pretty(x) != types.Pretty(y)
types.Pretty(x) == types.Pretty(y)
```

### 4. Error Code Coverage

For each error code in `internal/errs/error.go`:

- Does at least one test trigger it?
- List uncovered codes.

Use: `grep -o 'Err[A-Z][a-zA-Z]*' internal/errs/error.go | sort -u` to enumerate codes, then cross-reference with test files.

### 5. Test Quality

For each test that calls `checkSourceExpectCode`:

- Does the test verify the **effect** of the error (e.g., that the invalid instance was NOT registered)?
- Or does it only verify the error code?
- Are there parametric/compound variants beyond the trivial case?

For each test that calls `checkSource` (expect success):

- Is the source non-trivial enough to exercise the feature?
- Would a subtle bug (e.g., wrong substitution, missing context) still pass this test?

### 6. Spec Consistency

Verify that `spec/v0.5-abstraction.md` and `docs/grammar-reference.md` match the prelude (`internal/stdlib/prelude.go`):

- All instances in prelude are documented in spec
- All classes in prelude are documented in spec
- Instance lists are in sync (no phantom entries in spec that aren't implemented)

## Output

Report findings in severity order:

1. **CRITICAL**: Correctness violations (unguarded trial unification, error without prevention, structural comparison bugs)
2. **MODERATE**: Design consistency issues (inconsistent error handling patterns, missing test variants)
3. **MINOR**: Style and maintenance (dead code, unused variants, documentation drift)

For each finding, include:
- File and line
- What's wrong
- Suggested fix (or "already correct, just noting")

## Notes

This command is specific to the Gomputation type checker. The patterns it checks are informed by common bugs in type system implementations:
- Skolem escape through unguarded unification
- Instance incoherence through overlap detection that doesn't prevent registration
- Evidence corruption through partial unification without rollback
- Type equality conflation with string comparison
