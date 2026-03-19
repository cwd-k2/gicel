package gicel

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed examples/gicel
var exampleFS embed.FS

const exampleDir = "examples/gicel"

// Examples returns the list of available GICEL example names.
// Subdirectory structure is flattened with "." separators:
// examples/gicel/basics/hello.gicel → "basics.hello"
func Examples() []string {
	var names []string
	fs.WalkDir(exampleFS, exampleDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".gicel") {
			return nil
		}
		rel, _ := strings.CutPrefix(p, exampleDir+"/")
		name := strings.TrimSuffix(rel, ".gicel")
		name = strings.ReplaceAll(name, "/", ".")
		names = append(names, name)
		return nil
	})
	sort.Strings(names)
	return names
}

// Example returns the source code of the named GICEL example.
// Dot-separated names resolve to subdirectory paths:
// "basics.hello" → examples/gicel/basics/hello.gicel
// Returns empty string if the example is not found.
func Example(name string) string {
	// Convert dot-separated name to file path.
	filePath := strings.ReplaceAll(name, ".", "/") + ".gicel"
	// Reject path traversal.
	if strings.Contains(filePath, "..") {
		return ""
	}
	data, err := exampleFS.ReadFile(exampleDir + "/" + filePath)
	if err != nil {
		return ""
	}
	return string(data)
}
