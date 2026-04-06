// Package header parses GICEL file header directives.
//
// Source files may contain leading `-- gicel: ...` comment lines that
// declare structural options such as module dependencies and recursion.
// These are recognized by both the CLI and the LSP server.
package header

import "strings"

// Directives are parsed from the leading comment block of a GICEL file.
// Only structural options are recognized: --module and --recursion.
type Directives struct {
	Modules   []Module
	Recursion bool
}

// Module is a single --module Name=path from a file header.
type Module struct {
	Name string
	Path string
}

// Parse scans leading `-- gicel: ...` comment lines for directives.
// Stops at the first non-comment, non-blank line. Block comments ({- -})
// are not recognized as headers.
func Parse(source string) Directives {
	var hd Directives
	for _, line := range strings.Split(source, "\n") {
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
		switch fields[i] {
		case "--module":
			if i+1 < len(fields) {
				i++
				if eqIdx := strings.IndexByte(fields[i], '='); eqIdx > 0 {
					hd.Modules = append(hd.Modules, Module{
						Name: fields[i][:eqIdx],
						Path: fields[i][eqIdx+1:],
					})
				}
			}
		case "--recursion":
			hd.Recursion = true
		}
	}
}
