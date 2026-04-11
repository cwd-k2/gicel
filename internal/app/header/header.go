// Package header parses GICEL file header directives.
//
// Source files may contain leading `-- gicel: ...` comment lines that
// declare structural options such as module dependencies and recursion.
// These are recognized by both the CLI and the LSP server.
//
// Allowed structural options:
//   - --module Name=path     (module dependency)
//   - --recursion            (enable fix/rec)
//   - --packs prelude,state  (stdlib packs to load)
//
// Resource limits (--timeout, --max-steps, etc.) and output flags (--json,
// --explain) are NOT allowed — they are caller-side concerns.
// Unknown flags produce warnings for forward compatibility.
package header

import (
	"fmt"
	"strings"
)

// Directives are parsed from the leading comment block of a GICEL file.
type Directives struct {
	Modules   []Module
	Recursion bool
	Packs     string   // comma-separated pack names ("" = not specified in header)
	Warnings  []string // unknown or invalid directives
}

// Module is a single --module Name=path from a file header.
type Module struct {
	Name string
	Path string
}

// disallowed flags are recognized but not permitted in headers.
var disallowedFlags = map[string]bool{
	"--timeout": true, "--max-steps": true,
	"--max-depth": true, "--max-nesting": true, "--max-alloc": true,
	"--json": true, "--explain": true, "--explain-all": true,
	"--verbose": true, "--no-color": true, "--entry": true,
}

// Parse scans leading `-- gicel: ...` comment lines for directives.
// Stops at the first non-comment, non-blank line. Block comments ({- -})
// are not recognized as headers. Unknown flags produce warnings (not errors)
// for forward compatibility.
func Parse(source string) Directives {
	var hd Directives
	for line := range strings.SplitSeq(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#!") {
			continue
		}
		if !strings.HasPrefix(trimmed, "--") {
			break
		}
		after, ok := strings.CutPrefix(trimmed, "-- gicel:")
		if !ok {
			continue
		}
		parseDirective(strings.TrimSpace(after), &hd)
	}
	return hd
}

func parseDirective(args string, hd *Directives) {
	fields := strings.Fields(args)
	for i := 0; i < len(fields); i++ {
		switch {
		case fields[i] == "--module":
			if i+1 < len(fields) {
				i++
				if eqIdx := strings.IndexByte(fields[i], '='); eqIdx > 0 {
					hd.Modules = append(hd.Modules, Module{
						Name: fields[i][:eqIdx],
						Path: fields[i][eqIdx+1:],
					})
				} else {
					hd.Warnings = append(hd.Warnings,
						fmt.Sprintf("invalid --module: expected Name=path, got %q", fields[i]))
				}
			} else {
				hd.Warnings = append(hd.Warnings, "--module requires an argument (Name=path)")
			}
		case fields[i] == "--recursion":
			hd.Recursion = true
		case fields[i] == "--packs":
			if i+1 < len(fields) {
				i++
				hd.Packs = fields[i]
			} else {
				hd.Warnings = append(hd.Warnings, "--packs requires an argument (e.g., prelude,console)")
			}
		case disallowedFlags[fields[i]]:
			hd.Warnings = append(hd.Warnings,
				fmt.Sprintf("%s is not allowed in file headers (it is a CLI-only option)", fields[i]))
		case strings.HasPrefix(fields[i], "--"):
			hd.Warnings = append(hd.Warnings,
				fmt.Sprintf("unknown directive: %s", fields[i]))
		}
	}
}
