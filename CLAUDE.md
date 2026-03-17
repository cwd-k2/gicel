# GICEL

## Commands

```sh
go test ./...                          # test
go build -o bin/gicel ./cmd/gicel/     # build CLI
go run ./cmd/gicel/ run <file>.gicel   # run GICEL program
go run ./examples/go/<name>/           # run Go example
goimports -w .                         # format Go
prettier --write docs/ spec/           # format docs
```

### Multi-file programs (CLI)

```sh
# --module Name=path registers a module before compiling the main file.
# Repeatable; modules are registered in command-line order.
bin/gicel run \
  --module Geometry=Geometry.gicel \
  --module Color=Color.gicel \
  main.gicel

bin/gicel check \
  --module Geometry=Geometry.gicel \
  main.gicel
```

Working example at `examples/cli/multi-module/`:

```sh
cd examples/cli/multi-module
../../../bin/gicel run \
  --module Geometry=Geometry.gicel \
  --module Color=Color.gicel \
  --module MathLib=MathLib.gicel \
  main.gicel
# → (3, "red", 6)
```

## Rules

- Build output goes to `bin/` (gitignored).
- Format Go with `goimports`, docs with `prettier`.
- Commit per logical group or phase completion.
- Do not run test agents in background (memory exhaustion incident, 2024-03-14).
