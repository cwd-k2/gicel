## Module System

Modules organize code into named units. Import declarations must appear before all other declarations.

### Open Import

Brings all exported names into scope unqualified:

```
import Prelude
import Effect.State
```

### Selective Import

Only listed names are imported. Instances are always imported (coherence requirement):

```
import Prelude (Maybe(..), List(..), map, filter, (+))
```

Import list entries:

| Entry       | Meaning                                          |
| ----------- | ------------------------------------------------ |
| `name`      | Value binding                                    |
| `(op)`      | Operator                                         |
| `Name`      | Type or class (bare -- no constructors/methods)  |
| `Name(..)`  | Type or class with all constructors/methods      |
| `Name(A,B)` | Type or class with specific constructors/methods |

### Qualified Import

All names accessible only through the alias:

```
import Data.Map as M

main := M.empty
```

Operators cannot be qualified (`M.+` is not valid). Qualified names use adjacency: `M.x` (no whitespace) is qualified; `M . x` (whitespace) is composition.

### Private Names

Value bindings starting with `_` are module-private and excluded from exports:

```
_helper :: Int -> Int
_helper := \x. x + 1

publicFn :: Int -> Int       -- exported
publicFn := \x. _helper x
```

### Multi-File Projects (CLI)

Register user modules with `--module Name=path`:

```sh
gicel run \
  --module Geometry=lib/Geometry.gicel \
  --module Color=lib/Color.gicel \
  main.gicel
```

### File Header Directives

Source files can declare module dependencies and compiler options in leading comments. This eliminates the need for CLI flags in simple projects:

```
-- gicel: --module Geometry=./lib/Geometry.gicel
-- gicel: --module Color=./lib/Color.gicel
-- gicel: --recursion
import Prelude
import Geometry
import Color

main := ...
```

**Rules:**

- Only `--module Name=path` and `--recursion` are recognized in headers.
- Paths are relative to the declaring file.
- Directives are resolved recursively: if Geometry.gicel declares its own `--module` dependencies, those are discovered automatically.
- CLI `--module` flags override header directives.
- All resolved paths must be within the entry file's directory (security constraint).
- The header region ends at the first non-comment, non-blank line.

Header directives work with both `gicel run` and `gicel check`.

### Multi-File Projects (Go API)

```go
eng.RegisterModule("Util", utilSource)
```

Modules are type-checked at registration. Circular imports are forbidden. Duplicate imports of the same module are an error.

### Ambiguity

When two imports bring the same name into scope, the compiler reports an ambiguity error. Use qualified or selective import to disambiguate.
