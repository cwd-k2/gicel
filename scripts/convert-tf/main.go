// convert-tf converts type family =: equations to case => format in Go test files.
// Usage: go run ./scripts/convert-tf/ file1.go file2.go ...
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	for _, path := range os.Args[1:] {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		result := convertFile(string(data))
		if result != string(data) {
			os.WriteFile(path, []byte(result), 0644)
			fmt.Fprintf(os.Stderr, "converted: %s\n", path)
		}
	}
}

func convertFile(src string) string {
	var result strings.Builder
	i := 0
	for i < len(src) {
		idx := strings.Index(src[i:], "`")
		if idx < 0 {
			result.WriteString(src[i:])
			break
		}
		result.WriteString(src[i : i+idx+1])
		i += idx + 1
		end := strings.Index(src[i:], "`")
		if end < 0 {
			result.WriteString(src[i:])
			break
		}
		content := src[i : i+end]
		result.WriteString(convertGICEL(content))
		result.WriteByte('`')
		i += end + 1
	}
	return result.String()
}

// Type family declaration pattern:
// type Name (params) :: Kind := {
//   Name Pat =: RHS;
//   ...
// }
var (
	// Match type family declaration with params and ::
	tfDeclRe = regexp.MustCompile(`(?m)^(\s*)type\s+(\w+)\s+(.+?)\s*::\s*(.+?)\s*:=\s*\{`)
	// Match equation line: Name Pat =: RHS
	eqLineRe = regexp.MustCompile(`(?m)^(\s*)(\w+)\s+(.*?)\s*=:\s*(.*)$`)
)

func convertGICEL(src string) string {
	// Find all type family declarations
	// For each: extract name, params, kind, and convert body

	// Step 1: Convert equation lines (inside type family bodies)
	// Strip family name from equation, change =: to =>
	src = eqLineRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := eqLineRe.FindStringSubmatch(m)
		if len(groups) < 5 {
			return m
		}
		indent := groups[1]
		// groups[2] is the family name (stripped)
		patterns := strings.TrimSpace(groups[3])
		rhs := strings.TrimSpace(groups[4])
		return indent + patterns + " => " + rhs
	})

	// Step 2: Convert declaration line
	// type Name (params) :: Kind := { → type Name :: Kind := \(params). case param {
	src = tfDeclRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := tfDeclRe.FindStringSubmatch(m)
		if len(groups) < 5 {
			return m
		}
		indent := groups[1]
		name := groups[2]
		params := strings.TrimSpace(groups[3])
		kind := strings.TrimSpace(groups[4])

		// Extract first param name for case scrutinee
		firstParam := extractFirstParamName(params)

		// Build new declaration
		return fmt.Sprintf("%stype %s :: %s := \\%s. case %s {", indent, name, kind, params, firstParam)
	})

	return src
}

func extractFirstParamName(params string) string {
	params = strings.TrimSpace(params)
	if params == "" {
		return "_"
	}
	// (name: Kind) → name
	if strings.HasPrefix(params, "(") {
		inner := strings.TrimPrefix(params, "(")
		if idx := strings.Index(inner, ":"); idx >= 0 {
			return strings.TrimSpace(inner[:idx])
		}
		if idx := strings.Index(inner, ")"); idx >= 0 {
			return strings.TrimSpace(inner[:idx])
		}
	}
	// bare name
	parts := strings.Fields(params)
	if len(parts) > 0 {
		return parts[0]
	}
	return "_"
}
