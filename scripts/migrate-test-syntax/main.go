// migrate-test-syntax converts GICEL test source strings from old syntax to unified syntax.
// Usage: go run ./scripts/migrate-test-syntax/ internal/compiler/check/*_test.go
//
// Transformations applied to string literals:
// 1. "class Name params {" → "form Name := \params. {"
// 2. "instance [Ctx =>] Name types {" → "impl [Ctx =>] Name types := {"
// 3. "method :: Type" inside class bodies → "method: Type"
// 4. Keep top-level "name :: Type" unchanged (type annotations)
//
// The tool prints the modified file to stdout. Redirect to replace.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate-test-syntax <file>...")
		os.Exit(1)
	}
	for _, path := range os.Args[1:] {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
			continue
		}
		result := migrateFile(string(data))
		if result != string(data) {
			if err := os.WriteFile(path, []byte(result), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", path, err)
			} else {
				fmt.Fprintf(os.Stderr, "migrated: %s\n", path)
			}
		}
	}
}

// migrateFile processes a Go source file, finding raw string literals
// and applying GICEL syntax migrations within them.
func migrateFile(src string) string {
	// Find all backtick-quoted raw strings and transform their contents.
	var result strings.Builder
	i := 0
	for i < len(src) {
		// Look for backtick
		idx := strings.Index(src[i:], "`")
		if idx < 0 {
			result.WriteString(src[i:])
			break
		}
		result.WriteString(src[i : i+idx+1]) // write up to and including opening backtick
		i += idx + 1

		// Find closing backtick
		end := strings.Index(src[i:], "`")
		if end < 0 {
			result.WriteString(src[i:])
			break
		}

		content := src[i : i+end]
		migrated := migrateGICELSource(content)
		result.WriteString(migrated)
		result.WriteByte('`')
		i += end + 1
	}
	return result.String()
}

var (
	// type Name params := Body → type Name := \params. Body
	typeAliasRe = regexp.MustCompile(`(?m)^(\s*)type\s+(\w+)\s+([a-z][\w\s]*?)\s*:=\s*`)

	// class Name params { → form Name := \params. {
	classRe = regexp.MustCompile(`(?m)^(\s*)class\s+(\w+)\s+(.+?)\s*\{`)
	// class (Super) => Name params { → form Name := \params. Super => {
	classCtxRe = regexp.MustCompile(`(?m)^(\s*)class\s+\(([^)]+)\)\s*=>\s*(\w+)\s+(.+?)\s*\{`)
	// class Super => Name params { (without parens)
	classCtxNoPRe = regexp.MustCompile(`(?m)^(\s*)class\s+(\w+\s+\w+)\s*=>\s*(\w+)\s+(.+?)\s*\{`)
	// instance [Ctx =>] Name types { → impl [Ctx =>] Name types := {
	instanceRe = regexp.MustCompile(`(?m)^(\s*)instance\s+(.+?)\s*\{`)
	// method :: Type (inside braces, indented or after {/;) → method: Type
	methodRe   = regexp.MustCompile(`(?m)^(\s+)(\w+)\s+::\s+`)
	methodReIn = regexp.MustCompile(`([{;])\s*(\w+)\s+::\s+`)
)

func migrateGICELSource(src string) string {
	// type Name params := Body → type Name := \params. Body
	src = typeAliasRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := typeAliasRe.FindStringSubmatch(m)
		indent, name, params := groups[1], groups[2], strings.TrimSpace(groups[3])
		// Don't convert type families (they have :: before :=)
		// or types that already use \
		if strings.Contains(params, "::") || strings.Contains(params, "\\") {
			return m
		}
		return fmt.Sprintf("%stype %s := \\%s. ", indent, name, params)
	})

	// Order matters: context forms before simple class.

	// class (Ctx) => Name params {  →  form Name := \params. Ctx => {
	src = classCtxRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := classCtxRe.FindStringSubmatch(m)
		indent, ctx, name, params := groups[1], groups[2], groups[3], groups[4]
		params = strings.TrimSpace(params)
		return fmt.Sprintf("%sform %s := \\%s. %s => {", indent, name, params, ctx)
	})

	// class Ctx => Name params {  →  form Name := \params. Ctx => {
	src = classCtxNoPRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := classCtxNoPRe.FindStringSubmatch(m)
		indent, ctx, name, params := groups[1], groups[2], groups[3], groups[4]
		params = strings.TrimSpace(params)
		return fmt.Sprintf("%sform %s := \\%s. %s => {", indent, name, params, ctx)
	})

	// class Name params {  →  form Name := \params. {
	src = classRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := classRe.FindStringSubmatch(m)
		indent, name, params := groups[1], groups[2], groups[3]
		params = strings.TrimSpace(params)
		if params == "" {
			return fmt.Sprintf("%sform %s := {", indent, name)
		}
		return fmt.Sprintf("%sform %s := \\%s. {", indent, name, params)
	})

	// instance ... {  →  impl ... := {
	src = instanceRe.ReplaceAllStringFunc(src, func(m string) string {
		groups := instanceRe.FindStringSubmatch(m)
		indent, rest := groups[1], groups[2]
		rest = strings.TrimSpace(rest)
		// Don't add := if already present
		if strings.Contains(rest, ":=") {
			return fmt.Sprintf("%simpl %s {", indent, rest)
		}
		return fmt.Sprintf("%simpl %s := {", indent, rest)
	})

	// Also fix impl without := that weren't caught by instanceRe
	// Pattern: "impl Type {" at start of line → "impl Type := {"
	implFixRe := regexp.MustCompile(`(?m)(impl [^{]*[^=]) \{`)
	src = implFixRe.ReplaceAllStringFunc(src, func(m string) string {
		if strings.Contains(m, ":=") {
			return m
		}
		return strings.Replace(m, " {", " := {", 1)
	})

	// Convert method signatures inside braces.
	// Only convert "  name :: Type" (indented, lowercase name) to "  name: Type"
	// Don't touch "name :: Type" at start of line (top-level annotation).
	src = methodRe.ReplaceAllString(src, "${1}${2}: ")
	// Also handle inline: "{ name :: Type }" or "; name :: Type"
	src = methodReIn.ReplaceAllStringFunc(src, func(m string) string {
		groups := methodReIn.FindStringSubmatch(m)
		return groups[1] + " " + groups[2] + ": "
	})

	return src
}
