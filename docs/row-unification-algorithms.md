# Row Unification Algorithms

Deep domain research for the Gomputation specification (v0.2, Section 16: Row Equality and Unification Direction).

## Table of Contents

1. Foundations: Remy's Row Polymorphism
2. Canonical Row Representation
3. Row Unification Algorithm in Detail
4. Row Unification in Effect Systems
5. Integration with Type Unification
6. Scoped Labels and Duplicate Handling
7. Implementation Considerations for Go
8. Formal Properties
9. Comparison of Language Approaches
10. Recommendations for Gomputation

---

## 1. Foundations: Remy's Row Polymorphism

### 1.1 Historical Context

Row polymorphism originates with Mitchell Wand (1987), who introduced row variables to express the subtype relationship between records through parametric polymorphism rather than subtyping. Didier Remy (1989, 1992) extended this work substantially, establishing the framework that most modern row-polymorphic systems build on.

The key insight: instead of treating records as monolithic type constructors, decompose them into labeled sequences (rows) that can be abstracted over with ordinary type variables of a distinguished kind.

### 1.2 Remy's Flag Representation

Remy's original formulation assigns every label in the universe a **presence flag**:

```
Flag ::= Pre(T)    -- label is present with type T
       | Abs        -- label is absent
```

A row type is then a total function from the (countably infinite) label universe to flags:

```
Row = Label -> Flag
```

A record type `{ x : Int, y : Bool }` is represented as the infinite mapping:

```
x -> Pre(Int)
y -> Pre(Bool)
z -> Abs
w -> Abs
...  (all other labels -> Abs)
```

This representation has a critical property: **label order is irrelevant by construction**. The row `{ x : Int, y : Bool }` and `{ y : Bool, x : Int }` are literally the same function.

### 1.3 Row Variables and Their Semantics

In Remy's system, a row variable `r` ranges over rows -- that is, over mappings from labels to flags. An open record type like `{ x : Int | r }` is a row where:

- `x` is mapped to `Pre(Int)`
- all other labels are determined by `r`

The variable `r` does not range over "sub-records" but over the entire remaining mapping. Instantiating `r` to a row that maps `y` to `Pre(Bool)` and everything else to `Abs` yields `{ x : Int, y : Bool }`.

### 1.4 The Free Extension Problem

The flag representation solves a problem that plagued earlier record systems: **free extension**. The type of a function that accesses field `x` from a record should not fix the other fields. In Remy's system:

```
getX : forall r. { x : Pre(Int) | r } -> Int
```

Here `r` determines the flags of all labels except `x`. Any record with a present `x : Int` field, regardless of what other fields it has, is a valid argument.

### 1.5 Finite Representation

The infinite-map semantics is implementable because Remy requires that only finitely many labels deviate from a "default" determined by the row variable (or by `Abs` in a closed row). The finite representation is:

```
Row ::= RowVar(v)
      | RowEmpty                                 -- all labels Abs
      | RowExtend(label, type, row_tail)          -- one label Pre, rest determined by tail
```

This is exactly the grammar used by Gomputation's spec draft:

```
R ::= {} | { l : T | r }
```

where `{}` corresponds to `RowEmpty` and `{ l : T | r }` corresponds to `RowExtend(l, T, r)`.

### 1.6 Kind Discipline

Rows are classified by a distinct kind. In Remy's system:

```
Row : Kind
```

Row variables have kind `Row`. Type variables have kind `Type`. The two cannot be confused. This is exactly the kinding used by the Gomputation spec (Section 6.1).

---

## 2. Canonical Row Representation

### 2.1 The Problem of Structural Equality

Two row expressions can be syntactically different but semantically identical:

```
{ db : DB[Opened], log : Logger[Ready] }
{ log : Logger[Ready], db : DB[Opened] }
```

For unification to work correctly, the algorithm must recognize these as equal. There are two main approaches:

1. **Normalization to canonical form** before comparison
2. **Structural rewriting** during unification

### 2.2 Sorted Label Representation

The simplest canonical form for implementation: represent a row as a **sorted list of (label, type) pairs** plus an **optional tail variable**:

```
CanonicalRow = (SortedFields, Tail)
  where SortedFields = [(Label, Type)]  -- sorted by Label
        Tail          = None | RowVar(v)
```

Normalization:

```
normalize(RowEmpty) = ([], None)
normalize(RowVar(v)) = ([], Some(v))
normalize(RowExtend(l, t, r)) =
  let (fields, tail) = normalize(r)
  in  (insert_sorted(l, t, fields), tail)
```

After normalization:

- `{ db : DB[Opened], log : Logger[Ready] }` becomes `([(db, DB[Opened]), (log, Logger[Ready])], None)`
- `{ log : Logger[Ready], db : DB[Opened] }` becomes `([(db, DB[Opened]), (log, Logger[Ready])], None)`

These are identical.

### 2.3 Flattened Representation

For practical implementation, the "flattened" representation is a map plus tail:

```
FlatRow = { fields: Map<Label, Type>, tail: Option<TypeVar> }
```

This is the representation described by Max Bernstein's implementation and used in most practical type checkers. The map provides O(1) lookup (with a hash map) or O(log n) lookup (with a tree map), and label order is irrelevant by construction.

### 2.4 Normalization Algorithm

```
flatten(row: RowExpr) -> FlatRow:
    fields = {}
    current = row
    while true:
        match current:
            RowEmpty:
                return FlatRow { fields, tail: None }
            RowVar(v):
                v' = find(v)            -- follow union-find
                if v' is solved:
                    current = v'.solution
                    continue
                return FlatRow { fields, tail: Some(v') }
            RowExtend(l, t, rest):
                if l in fields:
                    error "duplicate label: l"
                fields[l] = t
                current = rest
```

The `find(v)` step follows the union-find structure for type/row variables to their current representative, which is critical for incremental unification.

### 2.5 Label Uniqueness Check

In systems that forbid duplicate labels (as Gomputation does), the flattening step serves double duty as a well-formedness check. If a label appears twice during flattening, the row is ill-formed. This check happens naturally during normalization.

---

## 3. Row Unification Algorithm in Detail

### 3.1 Overview

Row unification is an extension of standard first-order unification (Robinson, 1965). The key additions:

1. **Row-specific structural decomposition**: rows decompose by labels, not by position
2. **Tail solving**: open rows produce constraints on tail variables
3. **Permutation invariance**: label order must not affect the result
4. **Label uniqueness enforcement**: solutions must not introduce duplicate labels

### 3.2 The Algorithm: Five Cases

Given two rows `R1` and `R2` to unify, first flatten both:

```
flat1 = flatten(R1) = (fields1, tail1)
flat2 = flatten(R2) = (fields2, tail2)
```

Compute label sets:

```
shared   = keys(fields1) intersect keys(fields2)
only_in1 = keys(fields1) - keys(fields2)
only_in2 = keys(fields2) - keys(fields1)
```

Then dispatch on the shape of both rows:

#### Case 1: Two Closed Rows (tail1 = None, tail2 = None)

Both rows are fully known. They unify iff they have exactly the same labels, and each shared label's type unifies:

```
unify_rows_closed_closed(fields1, fields2):
    if keys(fields1) != keys(fields2):
        error "row mismatch: labels differ"
    for l in keys(fields1):
        unify(fields1[l], fields2[l])
```

**Example**:
```
{ db : DB[Opened], log : Logger[Ready] }  ~  { log : Logger[Ready], db : DB[Opened] }
=> unify(DB[Opened], DB[Opened])    -- ok
=> unify(Logger[Ready], Logger[Ready])  -- ok
=> success
```

**Failure example**:
```
{ db : DB[Opened] }  ~  { db : DB[Opened], log : Logger[Ready] }
=> keys differ: {db} != {db, log}
=> fail: "row mismatch: expected label 'log'"
```

#### Case 2: Open Row vs Closed Row (tail1 = Some(r), tail2 = None)

The open row must match the closed row. Shared labels unify pairwise. Labels in the closed row but not the open row are solved into the tail variable. Labels in the open row but not the closed row cause failure.

```
unify_rows_open_closed(fields1, tail1_var, fields2):
    -- (1) Labels in open row but not in closed row: impossible
    if only_in1 is not empty:
        error "row mismatch: closed row lacks labels {only_in1}"

    -- (2) Unify shared labels
    for l in shared:
        unify(fields1[l], fields2[l])

    -- (3) Solve tail variable to remaining fields
    remaining = { l: fields2[l] | l in only_in2 }
    r_solution = make_closed_row(remaining)

    -- (4) Occurs check
    if tail1_var occurs in r_solution:
        error "infinite row type"

    -- (5) Solve
    solve(tail1_var, r_solution)
```

**Example**:
```
{ db : DB[Closed] | r1 }  ~  { db : DB[Closed], log : Logger[Ready] }

flatten:
  flat1 = ({db: DB[Closed]}, Some(r1))
  flat2 = ({db: DB[Closed], log: Logger[Ready]}, None)

shared = {db}
only_in1 = {}
only_in2 = {log}

step (2): unify(DB[Closed], DB[Closed])  -- ok
step (3): remaining = {log: Logger[Ready]}
step (5): r1 := { log : Logger[Ready] }
```

#### Case 3: Closed Row vs Open Row (tail1 = None, tail2 = Some(r))

Symmetric to Case 2. Swap arguments and proceed.

#### Case 4: Two Open Rows (tail1 = Some(r1), tail2 = Some(r2))

This is the most subtle case. Both rows have unknown tails. The result introduces a **fresh row variable** representing the common unknown remainder.

```
unify_rows_open_open(fields1, r1, fields2, r2):
    -- (1) Unify shared labels
    for l in shared:
        unify(fields1[l], fields2[l])

    -- (2) Create fresh row variable for common remainder
    r_fresh = fresh_row_var()

    -- (3) Solve r1 to (labels only in row2) + r_fresh
    remaining_for_r1 = { l: fields2[l] | l in only_in2 }
    solution_for_r1 = extend_row(remaining_for_r1, r_fresh)

    -- (4) Solve r2 to (labels only in row1) + r_fresh
    remaining_for_r2 = { l: fields1[l] | l in only_in1 }
    solution_for_r2 = extend_row(remaining_for_r2, r_fresh)

    -- (5) Occurs checks
    if r1 occurs in solution_for_r1:
        error "infinite row type"
    if r2 occurs in solution_for_r2:
        error "infinite row type"

    -- (6) Check that r1's solution doesn't duplicate labels known in row1
    --     and r2's solution doesn't duplicate labels known in row2
    --     (This is automatic if we check: only_in2 disjoint from keys(fields1)
    --      and only_in1 disjoint from keys(fields2), which they are by construction.)

    -- (7) Solve
    solve(r1, solution_for_r1)
    solve(r2, solution_for_r2)
```

**Example**:
```
{ db : DB[Opened] | r1 }  ~  { db : DB[Opened], log : Logger[Ready] | r2 }

flatten:
  flat1 = ({db: DB[Opened]}, Some(r1))
  flat2 = ({db: DB[Opened], log: Logger[Ready]}, Some(r2))

shared = {db}
only_in1 = {}
only_in2 = {log}

step (1): unify(DB[Opened], DB[Opened])  -- ok
step (2): r_fresh = r3 (fresh)
step (3): solution_for_r1 = { log : Logger[Ready] | r3 }
step (4): solution_for_r2 = { | r3 } = r3
step (7): r1 := { log : Logger[Ready] | r3 }
          r2 := r3
```

After this, the two original rows both denote:
```
{ db : DB[Opened], log : Logger[Ready] | r3 }
```

#### Case 5: Two Row Variables (fields1 = {}, fields2 = {})

When both sides are bare row variables with no known fields, standard variable-variable unification applies:

```
unify_row_vars(r1, r2):
    if r1 == r2:
        return    -- already unified
    solve(r1, RowVar(r2))   -- or merge in union-find
```

### 3.3 Label Conflict: Same Label, Different Types

When both rows contain the same label but with different types, the unification of those types decides the outcome:

```
{ db : DB[Closed] | r1 }  ~  { db : DB[Opened] | r2 }

step: unify(DB[Closed], DB[Opened])
=> if DB[Closed] and DB[Opened] are distinct constructors: FAIL
   "cannot unify DB[Closed] with DB[Opened] at label 'db'"
```

This is exactly the behavior specified in the Gomputation spec (Section 16.2).

### 3.4 The Rewrite-Row Approach (Leijen)

Daan Leijen's "Extensible records with scoped labels" uses an alternative formulation based on **row rewriting** rather than flattening. The unification rule (uni-row) works as follows:

When unifying `{ l : T | r }` with some row `s`:

1. **Rewrite** `s` into the form `{ l : T' | s' }` using equational rules
2. Unify `T` with `T'`
3. Unify `r` with `s'`

The rewriting step searches through the row `s` for a field with label `l`, then "bubbles" it to the head:

```
rewrite_row(label, row):
    match row:
        RowEmpty:
            error "label not found: label"
        RowVar(v):
            -- Cannot rewrite further; create fresh variables
            t_fresh = fresh_type_var()
            r_fresh = fresh_row_var()
            solve(v, RowExtend(label, t_fresh, r_fresh))
            return (t_fresh, r_fresh)
        RowExtend(l', t', rest):
            if l' == label:
                return (t', rest)
            else:
                (found_type, remaining) = rewrite_row(label, rest)
                return (found_type, RowExtend(l', t', remaining))
```

The rewrite approach avoids explicit flattening but does the same logical work. It is particularly natural for linked-list row representations and is the approach used in tomprimozic's reference implementation.

### 3.5 The Flatten-Then-Diff Approach

The flattening approach (used by Bernstein, Thunderseethe, and most practical implementations) is:

```
unify_rows(R1, R2):
    flat1 = flatten(R1)
    flat2 = flatten(R2)
    -- dispatch on (flat1.tail, flat2.tail) as in Section 3.2
```

This approach is generally more efficient because:
- It avoids repeated traversal of the row structure
- It naturally produces sorted/canonical output
- Label lookup is O(1) or O(log n) rather than O(n)

### 3.6 Occurs Check for Row Variables

The occurs check prevents construction of infinite row types. It is identical in spirit to the standard occurs check for type variables but applied to row variables:

```
occurs_check(row_var, type_or_row):
    match type_or_row:
        RowVar(v):
            v' = find(v)
            if v' == row_var:
                error "infinite row type: row_var occurs in its own solution"
        RowExtend(l, t, rest):
            occurs_check(row_var, t)       -- row var could appear in a type
            occurs_check(row_var, rest)     -- row var could appear in the tail
        -- other type constructors: recurse into subterms
```

An important subtlety: the occurs check must look inside **type** positions too, not only row positions, because a row variable could in principle appear inside a type that appears as a field value (though this is uncommon in capability-row systems where field types are typically concrete).

### 3.7 Complete Pseudocode

Putting it all together, the full unification procedure for a system with row types:

```
unify(T1, T2):
    T1 = resolve(T1)    -- follow union-find / substitution
    T2 = resolve(T2)

    match (T1, T2):
        -- Standard cases
        (TypeVar(v1), _):
            if T1 == T2: return
            occurs_check(v1, T2)
            solve(v1, T2)

        (_, TypeVar(v2)):
            occurs_check(v2, T1)
            solve(v2, T1)

        (Arrow(a1, b1), Arrow(a2, b2)):
            unify(a1, a2)
            unify(b1, b2)

        (ForAll(a1, body1), ForAll(a2, body2)):
            -- handled by instantiation, not direct unification
            ...

        (TyCon(c1, args1), TyCon(c2, args2)):
            if c1 != c2: error "type mismatch"
            if len(args1) != len(args2): error "arity mismatch"
            for (a1, a2) in zip(args1, args2):
                unify(a1, a2)

        (Computation(pre1, post1, ret1), Computation(pre2, post2, ret2)):
            unify_rows(pre1, pre2)
            unify_rows(post1, post2)
            unify(ret1, ret2)

        -- Row cases (if rows can appear at type level)
        (Row(_), Row(_)):
            unify_rows(T1, T2)

        _:
            error "type mismatch"


unify_rows(R1, R2):
    flat1 = flatten(R1)
    flat2 = flatten(R2)

    shared   = keys(flat1.fields) & keys(flat2.fields)
    only_in1 = keys(flat1.fields) - keys(flat2.fields)
    only_in2 = keys(flat2.fields) - keys(flat1.fields)

    -- Unify types at shared labels
    for l in shared:
        unify(flat1.fields[l], flat2.fields[l])

    match (flat1.tail, flat2.tail):

        (None, None):
            -- Case 1: both closed
            if only_in1 is not empty or only_in2 is not empty:
                error "row label mismatch"

        (Some(r1), None):
            -- Case 2: open ~ closed
            if only_in1 is not empty:
                error "closed row lacks labels: {only_in1}"
            solution = make_closed_row(only_in2, flat2.fields)
            occurs_check(r1, solution)
            solve(r1, solution)

        (None, Some(r2)):
            -- Case 3: closed ~ open (symmetric)
            if only_in2 is not empty:
                error "closed row lacks labels: {only_in2}"
            solution = make_closed_row(only_in1, flat1.fields)
            occurs_check(r2, solution)
            solve(r2, solution)

        (Some(r1), Some(r2)):
            -- Case 4: open ~ open
            if r1 == r2 and only_in1 is empty and only_in2 is empty:
                return   -- same tail, no extra fields: nothing to do
            r_fresh = fresh_row_var()
            sol1 = extend_row(select(flat2.fields, only_in2), r_fresh)
            sol2 = extend_row(select(flat1.fields, only_in1), r_fresh)
            occurs_check(r1, sol1)
            occurs_check(r2, sol2)
            solve(r1, sol1)
            solve(r2, sol2)
```

### 3.8 Worked Examples

#### Example 1: Closed-Closed Match

```
unify_rows({ db : DB[Opened], log : Logger[Ready] },
           { log : Logger[Ready], db : DB[Opened] })

flatten both:
  flat1 = ({db: DB[Opened], log: Logger[Ready]}, None)
  flat2 = ({db: DB[Opened], log: Logger[Ready]}, None)

shared = {db, log}, only_in1 = {}, only_in2 = {}

unify(DB[Opened], DB[Opened])       => ok
unify(Logger[Ready], Logger[Ready]) => ok

Case 1: both closed, no extra labels => success
Result: no new constraints
```

#### Example 2: Open-Closed Solve

```
unify_rows({ db : DB[Closed] | r1 },
           { db : DB[Closed], log : Logger[Ready] })

flatten:
  flat1 = ({db: DB[Closed]}, Some(r1))
  flat2 = ({db: DB[Closed], log: Logger[Ready]}, None)

shared = {db}, only_in1 = {}, only_in2 = {log}

unify(DB[Closed], DB[Closed])  => ok

Case 2: open ~ closed
  only_in1 empty => ok
  solution = { log : Logger[Ready] }
  occurs_check(r1, { log : Logger[Ready] })  => ok
  solve(r1, { log : Logger[Ready] })

Result: r1 := { log : Logger[Ready] }
```

#### Example 3: Open-Open with Disjoint Labels

```
unify_rows({ db : DB[Opened] | r1 },
           { log : Logger[Ready] | r2 })

flatten:
  flat1 = ({db: DB[Opened]}, Some(r1))
  flat2 = ({log: Logger[Ready]}, Some(r2))

shared = {}, only_in1 = {db}, only_in2 = {log}

Case 4: open ~ open
  r_fresh = r3
  sol1 = { log : Logger[Ready] | r3 }
  sol2 = { db : DB[Opened] | r3 }
  solve(r1, { log : Logger[Ready] | r3 })
  solve(r2, { db : DB[Opened] | r3 })

Result:
  r1 := { log : Logger[Ready] | r3 }
  r2 := { db : DB[Opened] | r3 }
  Both rows now denote: { db : DB[Opened], log : Logger[Ready] | r3 }
```

#### Example 4: Typestate Conflict (Failure)

```
unify_rows({ db : DB[Closed] | r1 },
           { db : DB[Opened] | r2 })

flatten:
  flat1 = ({db: DB[Closed]}, Some(r1))
  flat2 = ({db: DB[Opened]}, Some(r2))

shared = {db}

unify(DB[Closed], DB[Opened])  => FAIL
  "cannot unify DB[Closed] with DB[Opened] at label 'db'"
```

#### Example 5: bind Sequencing

Consider:
```
dbOpen  : forall r. Comp { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
dbQuery : forall r. Query -> Comp { db : DB[Opened] | r } { db : DB[Opened] | r } Rows
```

Typing `bind dbOpen (\_ -> dbQuery q)`:

```
bind requires:
  c1 : Comp r1 r2 a
  k  : a -> Comp r2 r3 b

Instantiate dbOpen:
  Comp { db : DB[Closed] | r_a } { db : DB[Opened] | r_a } Unit

Instantiate dbQuery q:
  Comp { db : DB[Opened] | r_b } { db : DB[Opened] | r_b } Rows

Unify post of dbOpen with pre of dbQuery:
  { db : DB[Opened] | r_a }  ~  { db : DB[Opened] | r_b }

  flatten:
    flat1 = ({db: DB[Opened]}, Some(r_a))
    flat2 = ({db: DB[Opened]}, Some(r_b))

  shared = {db}, only_in1 = {}, only_in2 = {}

  unify(DB[Opened], DB[Opened])  => ok

  Case 4: open ~ open, same tail variable? No, different.
    r_fresh = r_c
    sol_a = { | r_c } = r_c
    sol_b = { | r_c } = r_c
    solve(r_a, r_c)
    solve(r_b, r_c)

Result type of bind:
  Comp { db : DB[Closed] | r_c } { db : DB[Opened] | r_c } Rows
```

The row variable `r_c` threads through, preserving the unknown capability context.

---

## 4. Row Unification in Effect Systems

### 4.1 Koka: Scoped Labels and Duplicate Effects

Koka uses row polymorphism for its effect system, with a design based on Leijen's "Extensible records with scoped labels" (2005). Key properties:

**Duplicate labels are allowed.** An effect row `<exc, exc>` is legal and distinct from `<exc>`. This models nested exception handlers naturally.

**Unification is deterministic and terminating.** By allowing duplicates, Koka avoids "lacks" constraints (`r\l`) that would be needed to enforce uniqueness. The unification algorithm is essentially standard Robinson unification with an additional row-rewriting rule.

**Effect unification.** Koka defines an operation `e =~ l | e'` that finds a label `l` in effect row `e` and returns the remainder `e'`. This is the rewrite-row operation applied to effects:

```
<l, l2, ..., ln | r>  =~  l  |  <l2, ..., ln | r>     -- found at head
<l1, l2, ..., ln | r> =~  l  |  <l1 | e'>              -- recurse: <l2,...,ln|r> =~ l | e'
```

When the effect is a bare variable `r`, unification creates fresh variables:

```
r  =~  l  |  r'     where r' is fresh, r := <l | r'>
```

**No constraint language.** Because Koka allows duplicates, it does not need lacks constraints, absence flags, or other constraint extensions. The proofs of soundness and completeness for HM carry over directly.

### 4.2 Links: Row Types for Effects and Databases

Links uses Hindley-Milner inference with row variables, following Remy's approach more closely than Koka. Links enforces label uniqueness for records and uses row variables for effect tracking.

Links' approach:
- Standard HM inference with row extension
- Row variables are kinded
- Unification follows the flatten-and-diff pattern
- Effects are tracked as row types on function arrows

### 4.3 PureScript: Row Types with Constraints

PureScript takes a distinct approach:

**Rows are unordered collections of named types, allowing duplicates.** Duplicate labels have their types collected in order, as if in a non-empty list.

**Row unification uses a separate unification table.** Row variables are solved to `ClosedRow` values, alongside a set of partial row combinations that are resolved as more information arrives during unification.

**Row constraints are expressed via type classes:**
- `Union` computes the union of two rows (left-biased)
- `Cons` asserts that a row can be obtained from another by inserting a label/type pair

**The challenge:** Row combination is commutative, so `(x: Int) + (f: Bool, y: Int) = (f: Bool, x: Int, y: Int)` and `(f: Bool, y: Int) + (x: Int) = (f: Bool, x: Int, y: Int)` are equal despite looking different syntactically. PureScript's unifier must recognize this.

### 4.4 Differences: Record Rows vs Effect Rows

| Aspect | Record Rows | Effect Rows | Capability Rows (Gomputation) |
|--------|-------------|-------------|-------------------------------|
| Duplicate labels | Usually forbidden | Often allowed (Koka) | Forbidden (spec Section 7.3) |
| Label semantics | Field name | Effect name | Capability identifier + state |
| Typical operation | Select, extend, restrict | Perform, handle | Transition (state update) |
| Tail interpretation | "other fields" | "other effects" | "other capabilities" |
| Constraint needs | Lacks (if unique) | None (if duplicates) | Lacks or freshness check |

Gomputation's capability rows are closest to record rows in that they enforce label uniqueness, but their semantics (typestate transition) is specific to the project.

---

## 5. Integration with Type Unification

### 5.1 Two Approaches

There are two main strategies for integrating row unification with ordinary type unification:

**Approach A: Direct unification (on-the-fly)**

Row unification is embedded directly into the standard unification procedure. When `unify` encounters two `Computation` types, it calls `unify_rows` on the pre and post rows. This is the simpler approach and the one used by most implementations.

```
unify(Computation(pre1, post1, ret1), Computation(pre2, post2, ret2)):
    unify_rows(pre1, pre2)
    unify_rows(post1, post2)
    unify(ret1, ret2)
```

**Approach B: Constraint generation**

The type checker generates constraints (including row constraints), then a separate solver resolves all constraints. This is more modular but introduces a constraint language:

```
Constraint ::= T1 ~ T2           -- type equality
             | R1 ~r R2          -- row equality
             | r \ l             -- "r lacks label l" (if needed)
```

The solver then processes constraints, possibly rewriting row constraints into simpler forms.

### 5.2 Recommendation for Gomputation

**Use direct unification (Approach A).** The Gomputation spec is designed around a small core with explicit row polymorphism and rank-1 inference. Direct unification is sufficient, simpler to implement, and avoids the need for a separate constraint language.

The constraint approach becomes necessary when:
- There are complex entailment relationships between rows
- Qualified types or type classes interact with rows
- Higher-rank polymorphism requires constraint collection before solving

None of these are active in the current Gomputation spec.

### 5.3 Interaction with Algorithm W

In the standard Algorithm W for Hindley-Milner inference, row unification integrates at these points:

1. **Variable lookup**: no special treatment needed.
2. **Lambda/application**: standard; row types flow through function types normally.
3. **Let-generalization**: row variables are generalized alongside type variables. A row variable `r` in the type of a let-bound function becomes `forall r. ...` if `r` does not appear free in the environment.
4. **Computation sequencing (bind)**: the post-row of the first computation and the pre-row of the continuation must unify. This is the primary site where row unification fires.

```
infer(env, Bind(c1, k)):
    (s1, Computation(pre1, post1, a)) = infer(env, c1)
    env' = apply(s1, env)
    (s2, t_k) = infer(env', k)
    -- t_k should be: a' -> Computation(pre2, post2, b)
    -- Unify a with a', and post1 with pre2
    s3 = unify(apply(s2, post1), pre2)
    -- Result:
    return (s3 . s2 . s1, Computation(apply(s3, pre1), apply(s3, post2), apply(s3, b)))
```

### 5.4 Substitution Application to Rows

When a substitution is applied, row variables must be resolved just like type variables:

```
apply_subst(subst, RowVar(v)):
    if v in subst:
        return apply_subst(subst, subst[v])
    else:
        return RowVar(v)

apply_subst(subst, RowExtend(l, t, rest)):
    return RowExtend(l, apply_subst(subst, t), apply_subst(subst, rest))

apply_subst(subst, RowEmpty):
    return RowEmpty
```

---

## 6. Scoped Labels and Duplicate Handling

### 6.1 Leijen's Scoped Labels

Daan Leijen's system allows duplicate labels in rows. When a label is extended onto a row that already contains it, the new binding **shadows** the old one rather than replacing it:

```
extend(x, Int, { x : Bool, y : Char }) = { x : Int, x : Bool, y : Char }
```

The first `x` is "in scope"; the second is shadowed. Restriction removes the outermost binding, revealing the inner one.

This design has two major consequences for unification:

1. **No lacks constraints needed.** Because duplicates are allowed, extending a row with label `l` never conflicts, so the type system does not need `r\l` predicates.

2. **Principal unification exists.** The absence of lacks constraints means the standard HM unification framework applies without extension.

### 6.2 Why Gomputation Chooses Unique Labels

The Gomputation spec (Section 7.3) requires label uniqueness. This is the right choice for capability environments because:

1. **Capability identity.** A row entry `db : DB[Opened]` represents a specific capability in a specific state. Duplicate `db` entries would create ambiguity: which database connection is being referenced?

2. **Typestate transitions.** Replacing `db : DB[Closed]` with `db : DB[Opened]` in a row must be unambiguous. With duplicates, a transition might shadow an old state rather than replacing it, leading to stale capability references.

3. **Error clarity.** When row unification fails, the error "label 'db' has type DB[Closed] but expected DB[Opened]" is clear. With duplicates, the error would need to specify which occurrence of `db` conflicted.

4. **Host boundary.** The host registers capabilities by name. Two capabilities with the same name would require a disambiguation mechanism that the spec does not (and probably should not) provide.

### 6.3 What Uniqueness Costs

Unique labels require one of:

1. **Lacks constraints** (`r\l`): the row variable `r` is constrained to not contain label `l`. This is what Remy's system uses.

2. **Freshness checks during unification**: when solving a row variable, check that the solution does not introduce a duplicate. This is simpler to implement.

For Gomputation, option (2) is preferable. The freshness check happens naturally during `flatten`:

```
flatten(RowExtend(l, t, rest)):
    (fields, tail) = flatten(rest)
    if l in fields:
        error "duplicate label: l"
    fields[l] = t
    return (fields, tail)
```

And during tail solving in Case 4 (open-open), the algorithm must verify:

```
-- When solving r1 := { labels_from_row2 | r_fresh }
-- Ensure labels_from_row2 are disjoint from fields already in row1
-- This is guaranteed by construction: labels_from_row2 = only_in2
-- which is defined as keys(fields2) - keys(fields1)
```

So the check is O(1) additional work beyond what the algorithm already computes.

### 6.4 Alternative: Lacks Constraints

If Gomputation ever needs explicit lacks constraints (for row subtraction, for instance), they would look like:

```
Gamma |- r : Row, r \ db
```

meaning "r is a row that does not contain label db." This enables typing:

```
closeAndDrop : forall r. r \ tmp => Comp { tmp : File[Open] | r } { r } Unit
```

This is not needed in the current spec but is a natural extension point. The unification algorithm would need to track constraint sets alongside substitutions.

---

## 7. Implementation Considerations for Go

### 7.1 Type Representation

```go
// Core type AST
type Type interface {
    typeNode()
}

type TypeVar struct {
    ID int
}

type Arrow struct {
    Param  Type
    Result Type
}

type Computation struct {
    Pre    Row
    Post   Row
    Result Type
}

type TyCon struct {
    Name string
    Args []Type
}

// Row representation
type Row interface {
    rowNode()
}

type RowEmpty struct{}

type RowExtend struct {
    Label string
    Type  Type
    Tail  Row
}

type RowVar struct {
    ID int
}
```

### 7.2 Flattened Row

```go
// FlatRow is the canonical, order-independent representation of a row.
type FlatRow struct {
    // Fields maps labels to their types, sorted by label for deterministic output.
    Fields map[string]Type
    // Tail is nil for a closed row, or a *RowVar for an open row.
    Tail   *RowVar
}

// Flatten resolves a Row expression into its canonical flattened form.
// Returns an error if duplicate labels are detected.
func (u *Unifier) Flatten(r Row) (*FlatRow, error) {
    fields := make(map[string]Type)
    current := r
    for {
        switch v := current.(type) {
        case *RowEmpty:
            return &FlatRow{Fields: fields, Tail: nil}, nil
        case *RowVar:
            resolved := u.FindRow(v)
            if solution, ok := u.rowSolutions[resolved.ID]; ok {
                current = solution
                continue
            }
            return &FlatRow{Fields: fields, Tail: resolved}, nil
        case *RowExtend:
            if _, exists := fields[v.Label]; exists {
                return nil, fmt.Errorf("duplicate label %q in row", v.Label)
            }
            fields[v.Label] = v.Type
            current = v.Tail
        default:
            return nil, fmt.Errorf("unexpected row node: %T", v)
        }
    }
}
```

### 7.3 Union-Find for Row Variables

Row variables and type variables should share a union-find infrastructure but with separate ID spaces (or tagged IDs):

```go
type Unifier struct {
    // Union-find parent pointers
    typeParent map[int]int
    typeRank   map[int]int
    rowParent  map[int]int
    rowRank    map[int]int

    // Solutions (substitution)
    typeSolutions map[int]Type
    rowSolutions  map[int]Row

    // Fresh variable counter
    nextID int
}

func (u *Unifier) FindRow(v *RowVar) *RowVar {
    root := v.ID
    for u.rowParent[root] != root {
        // Path compression
        u.rowParent[root] = u.rowParent[u.rowParent[root]]
        root = u.rowParent[root]
    }
    return &RowVar{ID: root}
}

func (u *Unifier) SolveRow(v *RowVar, solution Row) error {
    root := u.FindRow(v)
    if err := u.OccursCheckRow(root.ID, solution); err != nil {
        return err
    }
    u.rowSolutions[root.ID] = solution
    return nil
}
```

### 7.4 The Main Unification Entry Points

```go
func (u *Unifier) Unify(t1, t2 Type) error {
    t1 = u.Resolve(t1)
    t2 = u.Resolve(t2)

    switch a := t1.(type) {
    case *TypeVar:
        return u.unifyVar(a, t2)
    default:
        if b, ok := t2.(*TypeVar); ok {
            return u.unifyVar(b, t1)
        }
    }

    switch a := t1.(type) {
    case *Arrow:
        b, ok := t2.(*Arrow)
        if !ok {
            return u.mismatch(t1, t2)
        }
        if err := u.Unify(a.Param, b.Param); err != nil {
            return err
        }
        return u.Unify(a.Result, b.Result)

    case *Computation:
        b, ok := t2.(*Computation)
        if !ok {
            return u.mismatch(t1, t2)
        }
        if err := u.UnifyRows(a.Pre, b.Pre); err != nil {
            return fmt.Errorf("in pre-row: %w", err)
        }
        if err := u.UnifyRows(a.Post, b.Post); err != nil {
            return fmt.Errorf("in post-row: %w", err)
        }
        return u.Unify(a.Result, b.Result)

    // ... other type constructors
    }
    return u.mismatch(t1, t2)
}
```

### 7.5 Row Unification Implementation

```go
func (u *Unifier) UnifyRows(r1, r2 Row) error {
    flat1, err := u.Flatten(r1)
    if err != nil {
        return err
    }
    flat2, err := u.Flatten(r2)
    if err != nil {
        return err
    }

    // Compute label sets
    shared := make([]string, 0)
    onlyIn1 := make([]string, 0)
    onlyIn2 := make([]string, 0)

    for l := range flat1.Fields {
        if _, ok := flat2.Fields[l]; ok {
            shared = append(shared, l)
        } else {
            onlyIn1 = append(onlyIn1, l)
        }
    }
    for l := range flat2.Fields {
        if _, ok := flat1.Fields[l]; !ok {
            onlyIn2 = append(onlyIn2, l)
        }
    }

    // Sort for deterministic error messages
    sort.Strings(shared)
    sort.Strings(onlyIn1)
    sort.Strings(onlyIn2)

    // Unify types at shared labels
    for _, l := range shared {
        if err := u.Unify(flat1.Fields[l], flat2.Fields[l]); err != nil {
            return fmt.Errorf("at label %q: %w", l, err)
        }
    }

    // Dispatch on tail shapes
    switch {
    case flat1.Tail == nil && flat2.Tail == nil:
        // Case 1: both closed
        if len(onlyIn1) > 0 || len(onlyIn2) > 0 {
            return u.rowLabelMismatch(onlyIn1, onlyIn2)
        }
        return nil

    case flat1.Tail != nil && flat2.Tail == nil:
        // Case 2: open ~ closed
        if len(onlyIn1) > 0 {
            return fmt.Errorf("closed row lacks labels: %v", onlyIn1)
        }
        return u.solveTailToClosed(flat1.Tail, flat2.Fields, onlyIn2)

    case flat1.Tail == nil && flat2.Tail != nil:
        // Case 3: closed ~ open (symmetric)
        if len(onlyIn2) > 0 {
            return fmt.Errorf("closed row lacks labels: %v", onlyIn2)
        }
        return u.solveTailToClosed(flat2.Tail, flat1.Fields, onlyIn1)

    default:
        // Case 4: open ~ open
        return u.unifyOpenOpen(flat1.Tail, flat2.Tail,
            flat1.Fields, flat2.Fields, onlyIn1, onlyIn2)
    }
}

func (u *Unifier) solveTailToClosed(
    tailVar *RowVar,
    sourceFields map[string]Type,
    labels []string,
) error {
    var solution Row = &RowEmpty{}
    for _, l := range labels {
        solution = &RowExtend{Label: l, Type: sourceFields[l], Tail: solution}
    }
    return u.SolveRow(tailVar, solution)
}

func (u *Unifier) unifyOpenOpen(
    r1, r2 *RowVar,
    fields1, fields2 map[string]Type,
    onlyIn1, onlyIn2 []string,
) error {
    r1 = u.FindRow(r1)
    r2 = u.FindRow(r2)

    // If same variable with no extra fields: nothing to do
    if r1.ID == r2.ID && len(onlyIn1) == 0 && len(onlyIn2) == 0 {
        return nil
    }

    rFresh := u.FreshRowVar()

    // Build solution for r1: fields only in row2 + fresh tail
    var sol1 Row = rFresh
    for _, l := range onlyIn2 {
        sol1 = &RowExtend{Label: l, Type: fields2[l], Tail: sol1}
    }

    // Build solution for r2: fields only in row1 + fresh tail
    var sol2 Row = rFresh
    for _, l := range onlyIn1 {
        sol2 = &RowExtend{Label: l, Type: fields1[l], Tail: sol2}
    }

    if err := u.SolveRow(r1, sol1); err != nil {
        return err
    }
    return u.SolveRow(r2, sol2)
}
```

### 7.6 Error Reporting

Good error messages require tracking **context** through unification:

```go
type UnificationError struct {
    Context  string   // e.g., "in pre-row of bind"
    Label    string   // e.g., "db"
    Expected Type     // e.g., DB[Closed]
    Got      Type     // e.g., DB[Opened]
}

func (e *UnificationError) Error() string {
    return fmt.Sprintf("%s: at label %q, expected %s but got %s",
        e.Context, e.Label, e.Expected, e.Got)
}
```

Row-specific error categories:

| Error | Meaning | Example |
|-------|---------|---------|
| Label mismatch | Closed rows have different label sets | `{db}` vs `{db, log}` |
| Type conflict at label | Same label, incompatible types | `db: DB[Closed]` vs `db: DB[Opened]` |
| Infinite row | Occurs check failure | `r ~ {x : Int \| r}` |
| Duplicate label | Label appears twice after solving | Ill-formed row construction |
| Closed row lacks label | Open row has labels the closed row doesn't | `{db, log \| r}` vs `{db}` |

### 7.7 Efficient Normalization

For small rows (typical in capability environments: 2-10 labels), a sorted slice is competitive with or superior to a hash map:

```go
type SortedField struct {
    Label string
    Type  Type
}

type FlatRowSlice struct {
    Fields []SortedField  // sorted by Label
    Tail   *RowVar
}
```

Benefits:
- No hash overhead for small N
- Deterministic iteration order (sorted)
- Cache-friendly memory layout
- Simple equality check by traversal

The crossover point where a hash map becomes faster depends on the workload, but for capability rows with fewer than ~20 labels, the sorted-slice approach is likely faster in Go.

---

## 8. Formal Properties

### 8.1 Soundness

**Theorem (Soundness of Row Unification).** If `unify_rows(R1, R2)` produces a substitution `theta`, then `theta(R1) = theta(R2)` under the intended row equality (permutation of labels, alpha-equivalence of variables).

*Sketch.* Each case of the algorithm produces a substitution that equates the two rows:
- Case 1 (closed-closed): no substitution needed; label sets are equal and field types are unified.
- Case 2/3 (open-closed): the tail variable is solved to exactly the missing fields, making the open row equal to the closed row.
- Case 4 (open-open): both tail variables are solved such that both rows denote the same set of known fields plus a common fresh tail.

The soundness of field-type unification follows from the soundness of ordinary type unification.

### 8.2 Completeness

**Theorem (Completeness).** If there exists a substitution `sigma` such that `sigma(R1) = sigma(R2)`, then `unify_rows(R1, R2)` succeeds and produces a most general unifier `theta` such that `sigma = rho . theta` for some substitution `rho`.

*Sketch.* The algorithm is syntax-directed and considers all possible label distributions between the two rows. The fresh variable in Case 4 precisely captures the unknown shared remainder. The most-general-unifier property follows from the same argument as Robinson's unification, extended to handle the commutative nature of row labels.

**Caveat.** Completeness depends on the system not requiring lacks constraints. If label uniqueness is enforced at solve time (as recommended for Gomputation), then unification is still complete for consistent inputs, but may reject solutions that would introduce duplicate labels. This is intentional: such solutions would be ill-formed.

### 8.3 Principal Types

**Theorem.** In HM extended with row polymorphism (unique labels, permutation equality), every well-typed term has a principal type scheme.

This follows from the fact that:
1. Row unification produces most general unifiers (completeness)
2. Row variables are generalized at let-bindings just like type variables
3. The kind distinction between `Type` and `Row` is respected

This result holds for the unique-label discipline with freshness checks. It also holds for the duplicate-label discipline used by Koka (where the argument is even simpler because no uniqueness constraints exist).

### 8.4 Decidability

Row unification is decidable. The argument:
- The occurs check prevents infinite types
- Each unification step either fails, produces no new variables, or produces strictly smaller subproblems
- The fresh variable in Case 4 does not increase the problem size because it replaces two variables with one fresh variable plus two solutions
- Termination follows from the same measure as standard first-order unification: the total number of unsolved variables strictly decreases

### 8.5 Complexity

Row unification has the same asymptotic complexity as standard first-order unification when using union-find with path compression and union by rank:

- **Near-linear** in the size of the types/rows being unified (inverse Ackermann factor)
- The flattening step is O(n) where n is the number of labels
- Label intersection/difference is O(n log n) with sorted representations or O(n) with hash sets
- Overall: O(n * alpha(n)) where alpha is the inverse Ackermann function

In practice, for capability rows with a small number of labels, the constant factors dominate and the algorithm is effectively O(n).

---

## 9. Comparison of Language Approaches

| Feature | Remy (ML) | Koka | Links | PureScript | Elm | Gomputation |
|---------|-----------|------|-------|------------|-----|-------------|
| **Row kind** | Record | Effect | Record + Effect | Record | Record | Capability |
| **Duplicate labels** | No (flags) | Yes (scoped) | No | Yes (ordered) | No | No |
| **Label order** | Irrelevant | Irrelevant | Irrelevant | Irrelevant | Irrelevant | Irrelevant |
| **Tail variables** | Yes | Yes | Yes | Yes | Yes | Yes |
| **Lacks constraints** | Yes (via Abs flag) | No | Yes | Via type classes | Implicit | No (freshness check) |
| **Unification strategy** | Direct + flags | Direct + rewrite | Direct | Constraint-based | Direct | Direct (recommended) |
| **Row operations** | Select, extend, restrict | Perform, handle | Select, extend | Union, Cons | Select, extend | Transition (replace) |
| **Principal types** | Yes | Yes | Yes | Yes (with constraints) | Yes (intended) | Yes |
| **Known soundness issues** | No | No | No | No | Yes (Issue #656) | N/A |
| **Inference** | HM | HM | HM | HM + classes | HM | HM (rank-1) |

### Notable Implementation Approaches

| Implementation | Representation | Unification Style | Reference |
|---------------|---------------|-------------------|-----------|
| tomprimozic/type-systems | Linked list | Rewrite-row (Leijen) | OCaml, ~200 LOC |
| Bernstein (blog) | Flat map + tail | Flatten-then-diff | Python, tutorial |
| Thunderseethe | Closed rows + combinations | Diff-and-unify | Rust, production |
| Koka compiler | Row head/tail | Rewrite-row | Haskell, production |
| PureScript compiler | Separate unification table | Constraint solving | Haskell, production |

---

## 10. Recommendations for Gomputation

### 10.1 Representation

Use the **flattened map representation** (`FlatRow` with sorted fields + optional tail variable). Reasons:

- Natural fit for Go's `map[string]Type`
- Permutation invariance by construction
- Duplicate detection is automatic during flattening
- Efficient label intersection/difference via map operations
- Deterministic output ordering via sorted keys

For the AST, keep the linked-list `RowExtend` representation (it is natural for parsing and for the spec grammar). The flattening happens inside the unifier.

### 10.2 Unification Strategy

Use the **flatten-then-diff** algorithm (Section 3.2), not the rewrite-row approach. Reasons:

- Clearer mapping to Go's imperative style
- Better error messages (all label mismatches are visible at once, not discovered one at a time)
- More efficient for capability rows where the number of labels is small but comparison is frequent
- Avoids the need for equational rewriting rules in the type language

### 10.3 Variable Management

Use a **union-find** with separate ID spaces for type variables and row variables. This is standard and efficient.

Row variables and type variables should share the same `nextID` counter to avoid ID collisions, but have separate parent/rank/solution maps:

```go
type Unifier struct {
    nextID        int
    typeUF        *UnionFind
    rowUF         *UnionFind
    typeSolutions map[int]Type
    rowSolutions  map[int]Row
}
```

### 10.4 Label Uniqueness Enforcement

Enforce uniqueness via freshness checks during flattening and during tail solving. Do not introduce a lacks-constraint language. The freshness check is simple, efficient, and sufficient for the current spec.

If a future extension requires row subtraction (dropping a capability from a row), reassess whether lacks constraints are needed at that point.

### 10.5 Error Reporting Strategy

Report errors with:
- The label at which the conflict occurred
- The two conflicting types at that label
- The context (pre-row vs post-row, which bind site, which primitive)
- If a closed row is missing labels, list them explicitly

Sort label names in error messages for determinism.

### 10.6 Occurs Check

Implement the occurs check as a recursive traversal that checks whether a row variable appears in a type or row expression. The check is needed:
- Before solving a row variable
- Inside type positions (field values) as well as row positions (tails)

For capability rows where field types are typically concrete (like `DB[Opened]`), the occurs check will almost never traverse deeply. But it must be present for soundness.

### 10.7 Integration with Inference

Integrate row unification into the standard `unify` procedure. The `Computation` type constructor triggers row unification on its first two arguments. No separate constraint language is needed for the current spec.

Row variables should be generalized at let-bindings alongside type variables. The generalization check is the same: a variable is generalizable if it does not appear free in the typing environment.

---

## References

### Primary Papers

1. Didier Remy, "Type Inference for Records in a Natural Extension of ML" (1989). https://www.cs.cmu.edu/~aldrich/courses/819/row.pdf
2. Didier Remy, "Typing Record Concatenation for Free" (1992). POPL.
3. Mitchell Wand, "Type Inference for Record Concatenation and Multiple Inheritance" (1987). LICS.
4. Daan Leijen, "Extensible Records with Scoped Labels" (2005). https://www.microsoft.com/en-us/research/wp-content/uploads/2016/02/scopedlabels.pdf
5. Daan Leijen, "Koka: Programming with Row-Polymorphic Effect Types" (2014). https://arxiv.org/abs/1406.2061
6. Francois Pottier and Didier Remy, "The Essence of ML Type Inference" (2005). https://pauillac.inria.fr/~fpottier/publis/emlti-final.pdf

### Implementation References

7. Max Bernstein, "Adding Row Polymorphism to Damas-Hindley-Milner". https://bernsteinbear.com/blog/row-poly/
8. Joakim Ahnfelt-Ronne, "Row Polymorphism Crash Course". https://ahnfelt.medium.com/row-polymorphism-crash-course-587f1e7b7c47
9. Thunderseethe, "Rowing Afloat Datatype Boats". https://thunderseethe.dev/posts/row-types/
10. tomprimozic/type-systems, extensible_rows implementation. https://github.com/tomprimozic/type-systems/tree/master/extensible_rows
11. Cambridge L28 Lecture Notes, "Row Polymorphism". https://www.cl.cam.ac.uk/teaching/1415/L28/rows.pdf

### Implementations and Documentation

12. PureScript documentation, Types. https://github.com/purescript/documentation/blob/master/language/Types.md
13. Links programming language. https://links-lang.org/
14. Koka language. https://github.com/koka-lang/koka
15. OCaml Issue #10840, Soundness bug: polymorphism and row types. https://github.com/ocaml/ocaml/issues/10840
16. Elm Issue #656, Unsound type inference for extensible records. https://github.com/elm/compiler/issues/656

### Go Union-Find Libraries

17. Google Mangle union-find. https://pkg.go.dev/github.com/google/mangle/unionfind
18. theodesp/unionfind (weighted, path-compressed). https://github.com/theodesp/unionfind
