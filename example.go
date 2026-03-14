package gicel

import (
	"embed"
	"sort"
	"strings"
)

//go:embed examples/gicel/*.gicel
var exampleFS embed.FS

const exampleDir = "examples/gicel"

// Examples returns the list of available GICEL example names.
func Examples() []string {
	entries, err := exampleFS.ReadDir(exampleDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".gicel"))
	}
	sort.Strings(names)
	return names
}

// Example returns the source code of the named GICEL example.
// Returns empty string if the example is not found.
func Example(name string) string {
	data, err := exampleFS.ReadFile(exampleDir + "/" + name + ".gicel")
	if err != nil {
		return ""
	}
	return string(data)
}
