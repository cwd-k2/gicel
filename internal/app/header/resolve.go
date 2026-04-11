package header

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxResolveDepth limits recursive header resolution to prevent
// pathological chains or cycles.
const maxResolveDepth = 64

// ResolvedModule is a module discovered through header resolution,
// with its name, absolute path, and source text ready for registration.
type ResolvedModule struct {
	Name   string
	Path   string // absolute, symlink-resolved
	Source string
}

// ResolveResult holds the output of recursive header resolution.
type ResolveResult struct {
	Modules   []ResolvedModule // topological order (dependencies first)
	Recursion bool             // true if any file declared --recursion
	Packs     string           // from entry file's --packs ("" = not specified)
	Warnings  []string         // unknown or disallowed directives encountered
}

// Resolve recursively reads header directives starting from entrySource
// (located at entryPath), discovering transitive module dependencies.
//
// Paths in headers are resolved relative to the declaring file.
// All resolved paths are checked to be within rootDir (the entry file's
// directory). Paths outside rootDir are rejected for security.
//
// Same module name mapping to the same absolute path is deduplicated.
// Same module name mapping to different paths is an error.
func Resolve(entrySource, entryPath string) (*ResolveResult, error) {
	absEntry, err := filepath.Abs(entryPath)
	if err != nil {
		return nil, fmt.Errorf("resolve entry path: %w", err)
	}
	evalEntry, err := filepath.EvalSymlinks(absEntry)
	if err != nil {
		return nil, fmt.Errorf("resolve entry path: %w", err)
	}
	rootDir := filepath.Dir(evalEntry)

	r := &resolver{
		rootDir:  rootDir,
		seen:     make(map[string]string), // module name → absolute path
		visiting: make(map[string]bool),   // absolute path → currently visiting
		result:   &ResolveResult{},
	}

	if err := r.resolve(entrySource, absEntry, 0); err != nil {
		return nil, err
	}
	return r.result, nil
}

type resolver struct {
	rootDir  string
	seen     map[string]string // module name → resolved abs path
	visiting map[string]bool   // cycle detection by file path
	result   *ResolveResult
}

func (r *resolver) resolve(source, filePath string, depth int) error {
	if depth > maxResolveDepth {
		return fmt.Errorf("header resolution depth exceeded %d (possible cycle)", maxResolveDepth)
	}

	directives := Parse(source)
	if directives.Recursion {
		r.result.Recursion = true
	}
	// --packs from the entry file only (depth 0); dependency files don't set packs.
	if depth == 0 && directives.Packs != "" {
		r.result.Packs = directives.Packs
	}
	for _, w := range directives.Warnings {
		r.result.Warnings = append(r.result.Warnings, fmt.Sprintf("%s: %s", filepath.Base(filePath), w))
	}

	baseDir := filepath.Dir(filePath)

	for _, mod := range directives.Modules {
		modPath := mod.Path
		if !filepath.IsAbs(modPath) {
			modPath = filepath.Join(baseDir, modPath)
		}

		// Resolve symlinks and normalize.
		absPath, err := filepath.Abs(modPath)
		if err != nil {
			return fmt.Errorf("header module %s: %w", mod.Name, err)
		}
		evalPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("header module %s: file not found: %s", mod.Name, mod.Path)
			}
			return fmt.Errorf("header module %s: %w", mod.Name, err)
		}

		// Containment check: resolved path must be under rootDir.
		rel, err := filepath.Rel(r.rootDir, evalPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("header module %s: path %s is outside project root %s",
				mod.Name, mod.Path, r.rootDir)
		}

		// Dedup: same name, same path = ok. Same name, different path = error.
		if prev, ok := r.seen[mod.Name]; ok {
			if prev == evalPath {
				continue // diamond dependency, already resolved
			}
			return fmt.Errorf("header module %s: conflicting paths: %s vs %s",
				mod.Name, prev, evalPath)
		}

		// Cycle detection by file path.
		if r.visiting[evalPath] {
			return fmt.Errorf("circular header dependency involving %s", evalPath)
		}
		r.visiting[evalPath] = true

		// Read the module file.
		data, err := os.ReadFile(evalPath)
		if err != nil {
			return fmt.Errorf("header module %s: %w", mod.Name, err)
		}

		// Recursively resolve the module's own headers first (dependencies before dependents).
		if err := r.resolve(string(data), evalPath, depth+1); err != nil {
			return err
		}

		delete(r.visiting, evalPath)
		r.seen[mod.Name] = evalPath
		r.result.Modules = append(r.result.Modules, ResolvedModule{
			Name:   mod.Name,
			Path:   evalPath,
			Source: string(data),
		})
	}

	return nil
}
