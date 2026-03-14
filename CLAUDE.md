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

## Rules

- Build output goes to `bin/` (gitignored).
- Format Go with `goimports`, docs with `prettier`.
- Commit per logical group or phase completion.
- Do not run test agents in background (memory exhaustion incident, 2024-03-14).
