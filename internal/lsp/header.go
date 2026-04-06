package lsp

import "strings"

// HeaderDirectives are parsed from the leading comment block of a GICEL file.
// Only structural options are recognized: --module and --recursion.
type HeaderDirectives struct {
	Modules   []ModuleDirective
	Recursion bool
}

// ModuleDirective is a single --module Name=path from a file header.
type ModuleDirective struct {
	Name string
	Path string
}

// ParseHeader scans leading `-- gicel: ...` comment lines for directives.
// Stops at the first non-comment, non-blank line. Block comments ({- -})
// are not recognized as headers.
func ParseHeader(source string) HeaderDirectives {
	var hd HeaderDirectives
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Shebang line on first line.
		if strings.HasPrefix(trimmed, "#!") {
			continue
		}
		if !strings.HasPrefix(trimmed, "--") {
			break // first non-comment line: end of header region
		}
		after, ok := strings.CutPrefix(trimmed, "-- gicel:")
		if !ok {
			continue // regular comment, stay in header region
		}
		parseDirective(strings.TrimSpace(after), &hd)
	}
	return hd
}

func parseDirective(args string, hd *HeaderDirectives) {
	fields := strings.Fields(args)
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--module":
			if i+1 < len(fields) {
				i++
				if eqIdx := strings.IndexByte(fields[i], '='); eqIdx > 0 {
					hd.Modules = append(hd.Modules, ModuleDirective{
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
