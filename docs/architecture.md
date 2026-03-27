# Architecture

Package dependency diagram for the GICEL compiler and runtime.

_Last updated: v0.17.2 (2026-03-25)._

## Layer Model

```
Layer 5  app/engine              orchestration
Layer 4  host/stdlib registry    Go integration
Layer 3  compiler/check parse optimize   compilation
Layer 2  lang/ir types           IR and type representation
Layer 1  lang/syntax             surface syntax
Layer 0  infra/span diagnostic budget    shared infrastructure
```

Dependencies flow downward. No package imports from a higher layer.

## Dependency DAG

```
app/engine ──→ compiler/{check,parse,optimize}
             ──→ host/{stdlib,registry}
             ──→ runtime/eval
             ──→ lang/{ir,syntax,types}
             ──→ infra/{budget,diagnostic,span}

host/stdlib ──→ host/registry
            ──→ runtime/eval
            ──→ lang/{ir,syntax}
            ──→ infra/budget

host/registry ──→ runtime/eval
              ──→ lang/ir

runtime/eval ──→ lang/{ir,syntax}
             ──→ infra/{budget,span}

compiler/check ──→ check/{solve,unify,family,exhaust,env,modscope}
               ──→ compiler/parse
               ──→ lang/{syntax,types,ir}
               ──→ infra/{budget,diagnostic,span}

  check/solve ──→ check/{unify,family}
              ──→ lang/types
              ──→ infra/{budget,diagnostic,span}

  check/modscope ──→ check/{env,exhaust,family}
                 ──→ lang/{syntax,types}
                 ──→ infra/{diagnostic,span}

  check/env ──→ check/{exhaust,family}
            ──→ lang/types
            ──→ infra/span

  check/family ──→ check/unify
               ──→ lang/types
               ──→ infra/{budget,diagnostic,span}

  check/exhaust ──→ check/unify
                ──→ lang/{ir,types}
                ──→ infra/{diagnostic,span}

  check/unify ──→ lang/types
              ──→ infra/budget

compiler/parse ──→ lang/syntax
               ──→ infra/{span,diagnostic}

compiler/optimize ──→ lang/ir

lang/ir ──→ lang/types
        ──→ infra/span

lang/types ──→ infra/span

lang/syntax ──→ infra/span

infra/diagnostic ──→ infra/span

infra/budget ──→ (isolated)
infra/span   ──→ (isolated)
```

## Package Responsibilities

### infra — shared infrastructure

| Package            | Responsibility                     |
| ------------------ | ---------------------------------- |
| `infra/span`       | Source positions and spans         |
| `infra/diagnostic` | Structured compiler diagnostics    |
| `infra/budget`     | Step, depth, and allocation limits |

### lang — language definition

| Package       | Responsibility                                          |
| ------------- | ------------------------------------------------------- |
| `lang/syntax` | AST node types, token definitions, source-level helpers |
| `lang/types`  | Type (unified across universe levels), row types, evidence |
| `lang/ir`     | Core IR (17 formers), program structure, walkers        |

### compiler — source to Core IR

| Package                   | Responsibility                                                  |
| ------------------------- | --------------------------------------------------------------- |
| `compiler/parse`          | Pratt-parser from source to AST                                 |
| `compiler/check`          | Bidirectional type checking, elaboration to Core IR             |
| `compiler/check/solve`    | OutsideIn(X) constraint solving, worklist, inert set            |
| `compiler/check/unify`    | Type unification, meta-variable solving                         |
| `compiler/check/family`   | Type family reduction                                           |
| `compiler/check/exhaust`  | Pattern exhaustiveness checking (Maranget)                      |
| `compiler/check/env`      | Module export types: aliases, classes, instances, type families |
| `compiler/check/modscope` | Module import resolution, qualified name scoping                |
| `compiler/optimize`       | Core IR simplification and fusion                               |

### runtime — Core IR execution

| Package        | Responsibility                                                      |
| -------------- | ------------------------------------------------------------------- |
| `runtime/eval` | Trampoline-based CBV evaluator, de Bruijn indexed array environment |

### host — Go integration

| Package         | Responsibility                                         |
| --------------- | ------------------------------------------------------ |
| `host/registry` | Registration interface (`Registrar`, `Pack`)           |
| `host/stdlib`   | Standard library packs (Prelude, Effects, Collections) |

### app — orchestration

| Package      | Responsibility                                  |
| ------------ | ----------------------------------------------- |
| `app/engine` | Compilation pipeline, runtime assembly, sandbox |

## Invariants

- `lang/` has no imports from `compiler/`, `runtime/`, `host/`, or `app/`.
- `infra/` has no imports from any other `internal/` package.
- `compiler/parse` depends only on `lang/syntax` and `infra/`.
- `runtime/eval` has no imports from `compiler/` or `host/`.
- `host/registry` breaks the potential cycle between `host/stdlib` and `runtime/eval`.
- `app/engine` is the only package that imports from all lower layers.
- The codebase is strictly single-threaded; no goroutines, channels, or mutexes.
