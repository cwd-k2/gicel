package lsp

import "github.com/cwd-k2/gicel/internal/app/header"

// Re-export for internal use. The canonical implementation is in
// internal/app/header, shared with the CLI.

type HeaderDirectives = header.Directives
type ModuleDirective = header.Module

var ParseHeader = header.Parse
