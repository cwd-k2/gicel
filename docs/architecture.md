# Architecture

Package dependency diagram for the GICEL compiler and runtime.

_Last updated: v0.29.0 (2026-04-11)._

## Layer Model

```
Layer 6  gicel (root)            public Go API facade
Layer 5  app/engine              orchestration
Layer 4  host/stdlib registry    Go integration
Layer 3  compiler/check parse optimize   compilation
Layer 2  lang/ir types syntax    IR, type, and AST representation
Layer 0  infra/span diagnostic budget    shared infrastructure
```

Dependencies flow downward. `lang/syntax` imports `lang/types` (lateral within Layer 2). Execution goes through `runtime/vm` (bytecode VM); `runtime/eval` provides shared value types.

## Dependency DAG

```
gicel (root)   ──→ app/engine
               ──→ host/{stdlib,registry}
               ──→ runtime/eval
               ──→ lang/{ir,types}
               ──→ infra/budget

app/engine ──→ compiler/{check,parse,optimize}
             ──→ host/{stdlib,registry}
             ──→ runtime/{eval,vm}
             ──→ lang/{ir,syntax,types}
             ──→ infra/{budget,diagnostic,span}

host/stdlib ──→ host/registry
            ──→ runtime/eval
            ──→ lang/{ir,types}
            ──→ infra/budget

host/registry ──→ runtime/eval
              ──→ lang/ir

runtime/vm ──→ runtime/eval
           ──→ lang/{ir,types}
           ──→ infra/{budget,span}

runtime/eval ──→ lang/ir
             ──→ infra/span

compiler/check ──→ check/{solve,unify,family,exhaust,env,modscope}
               ──→ lang/{syntax,types,ir}
               ──→ infra/{budget,diagnostic,span}

  check/solve ──→ check/env
              ──→ lang/{types,ir}
              ──→ infra/{diagnostic,span}

  check/modscope ──→ check/env
                 ──→ lang/{syntax,types}
                 ──→ infra/{diagnostic,span}

  check/env ──→ lang/types
            ──→ infra/span

  check/family ──→ check/{unify,env}
               ──→ lang/types
               ──→ infra/{budget,diagnostic,span}

  check/exhaust ──→ check/{env,unify}
                ──→ lang/{ir,types}
                ──→ infra/{diagnostic,span}

  check/unify ──→ lang/types
              ──→ infra/{budget,span}

compiler/parse ──→ lang/{syntax,types}
               ──→ infra/{span,diagnostic}

compiler/optimize ──→ lang/ir

lang/ir ──→ lang/types
        ──→ infra/span

lang/types ──→ infra/span

lang/syntax ──→ lang/{ir,types}
            ──→ infra/span

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

| Package       | Responsibility                                             |
| ------------- | ---------------------------------------------------------- |
| `lang/syntax` | AST node types, token definitions, source-level helpers    |
| `lang/types`  | Type (unified across universe levels), row types, evidence |
| `lang/ir`     | Core IR (19 formers), program structure, walkers           |

### compiler — source to Core IR

| Package                   | Responsibility                                           |
| ------------------------- | -------------------------------------------------------- |
| `compiler/parse`          | Pratt-parser from source to AST                          |
| `compiler/check`          | Bidirectional type checking, elaboration to Core IR      |
| `compiler/check/solve`    | OutsideIn(X) constraint solving, worklist, inert set     |
| `compiler/check/unify`    | Type unification, meta-variable solving                  |
| `compiler/check/family`   | Type family reduction                                    |
| `compiler/check/exhaust`  | Pattern exhaustiveness checking (Maranget)               |
| `compiler/check/env`      | Typing context entries, module exports, naming utilities |
| `compiler/check/modscope` | Module import resolution, qualified name scoping         |
| `compiler/optimize`       | Core IR simplification and fusion                        |

### runtime — Core IR execution

| Package        | Responsibility                                                                 |
| -------------- | ------------------------------------------------------------------------------ |
| `runtime/eval` | Shared value types (Closure, ConVal, ThunkVal, PrimVal, etc.) and CapEnv       |
| `runtime/vm`   | Bytecode compiler (Core IR → bytecode) and VM with TCO and allocation tracking |

### host — Go integration

| Package         | Responsibility                                         |
| --------------- | ------------------------------------------------------ |
| `host/registry` | Registration interface (`Registrar`, `Pack`)           |
| `host/stdlib`   | Standard library packs (Prelude, Effects, Collections) |

### app — orchestration

| Package      | Responsibility                                  |
| ------------ | ----------------------------------------------- |
| `app/engine` | Compilation pipeline, runtime assembly, sandbox |

### root — public Go API

| Package        | Responsibility                                          |
| -------------- | ------------------------------------------------------- |
| `gicel` (root) | Public facade: Engine, Runtime, RunSandbox, Pack, Value |

## Invariants

- `lang/` has no imports from `compiler/`, `runtime/`, `host/`, or `app/`.
- `infra/` has no imports from any other `internal/` package.
- `compiler/parse` depends on `lang/{syntax,types}` and `infra/`.
- `runtime/eval` and `runtime/vm` have no imports from `compiler/` or `host/`.
- `host/registry` breaks the potential cycle between `host/stdlib` and `runtime/eval`.
- `app/engine` is the only internal package that imports from all lower layers.
- `gicel` (root) is the public entry point; it wraps `app/engine` and re-exports key types.
