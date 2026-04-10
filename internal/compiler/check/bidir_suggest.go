// Diagnostic suggestions — name similarity hints for unbound variables and constructors.
// Does NOT cover: name resolution (bidir_lookup.go), instantiation (bidir_inst.go).
package check

import (
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// suggestVar returns hint(s) for an unbound variable by searching the context
// for similar names. Candidates are filtered by category: variable names are
// only matched against variables, operators only against operators.
func (ch *Checker) suggestVar(name string) []diagnostic.Hint {
	nameIsIdent := isIdentName(name)
	seen := make(map[string]bool)
	var candidates []string
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok && !seen[v.Name] && v.Name != "" && !env.IsPrivateName(v.Name) {
			// Only suggest same-category names (ident↔ident, op↔op).
			if isIdentName(v.Name) == nameIsIdent {
				seen[v.Name] = true
				candidates = append(candidates, v.Name)
			}
		}
		return true
	})
	return suggestHints(name, candidates)
}

// isIdentName returns true if the name starts with a letter or underscore
// (i.e., is a variable/function name, not an operator symbol).
func isIdentName(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

// suggestCon returns hint(s) for an unknown constructor by searching the registry.
func (ch *Checker) suggestCon(name string) []diagnostic.Hint {
	var candidates []string
	for c := range ch.reg.AllConTypes() {
		candidates = append(candidates, c)
	}
	return suggestHints(name, candidates)
}

// importHints maps well-known names to the module that exports them.
// Used to suggest "import M" when a name is unbound but would be
// available with the right import.
var importHints = map[string]string{
	"+": "Prelude", "-": "Prelude", "*": "Prelude", "/": "Prelude",
	"==": "Prelude", "/=": "Prelude", "<": "Prelude", ">": "Prelude",
	"<=": "Prelude", ">=": "Prelude", "&&": "Prelude", "||": "Prelude",
	".": "Prelude", "$": "Prelude", "<>": "Prelude", "++": "Prelude",
	">>": "Prelude", ">>=": "Prelude", "compare": "Prelude",
	"show": "Prelude", "read": "Prelude", "id": "Prelude",
	"const": "Prelude", "flip": "Prelude", "map": "Prelude",
	"filter": "Prelude", "foldl": "Prelude", "foldr": "Prelude",
	"head": "Prelude", "tail": "Prelude", "length": "Prelude",
	"reverse": "Prelude", "concat": "Prelude", "fmap": "Prelude",
	"pure": "Prelude", "ap": "Prelude",
	"get": "Effect.State", "put": "Effect.State", "modify": "Effect.State",
	"evalState": "Effect.State", "runState": "Effect.State", "execState": "Effect.State",
	"failWith": "Effect.Fail", "try": "Effect.Fail",
	"log":     "Effect.IO",
	"putLine": "Console",
}

// suggestImport returns a hint suggesting the appropriate import if name
// is a well-known identifier or operator from a standard module.
func suggestImport(name string) []diagnostic.Hint {
	mod, ok := importHints[name]
	if !ok {
		return nil
	}
	return []diagnostic.Hint{{Message: "did you mean: import " + mod + "?"}}
}

func suggestHints(name string, candidates []string) []diagnostic.Hint {
	matches := diagnostic.Suggest(name, candidates, 2, 3)
	if len(matches) == 0 {
		return nil
	}
	quoted := make([]string, len(matches))
	for i, m := range matches {
		quoted[i] = "'" + m + "'"
	}
	return []diagnostic.Hint{{Message: "did you mean " + strings.Join(quoted, ", ") + "?"}}
}
