package header

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_SingleModule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib.gicel", "import Prelude\ndouble := \\x. x * 2\n")
	mainSrc := "-- gicel: --module Lib=./lib.gicel\nimport Prelude\nimport Lib\nmain := double 21\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	res, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(res.Modules))
	}
	if res.Modules[0].Name != "Lib" {
		t.Fatalf("expected Lib, got %s", res.Modules[0].Name)
	}
}

func TestResolve_TransitiveDependency(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "utils.gicel", "import Prelude\nwrap := \\s. \"[\" ++ s ++ \"]\"\n")
	writeFile(t, dir, "lib.gicel", "-- gicel: --module Utils=./utils.gicel\nimport Prelude\nimport Utils\ngreet := \\name. wrap name\n")
	mainSrc := "-- gicel: --module Lib=./lib.gicel\nimport Prelude\nimport Lib\nmain := greet \"world\"\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	res, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(res.Modules))
	}
	// Utils should come before Lib (dependency order).
	if res.Modules[0].Name != "Utils" {
		t.Fatalf("expected Utils first, got %s", res.Modules[0].Name)
	}
	if res.Modules[1].Name != "Lib" {
		t.Fatalf("expected Lib second, got %s", res.Modules[1].Name)
	}
}

func TestResolve_DiamondDependency(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "shared.gicel", "import Prelude\nhelper := \\x. x + 1\n")
	writeFile(t, dir, "a.gicel", "-- gicel: --module Shared=./shared.gicel\nimport Prelude\nimport Shared\nfromA := helper 1\n")
	writeFile(t, dir, "b.gicel", "-- gicel: --module Shared=./shared.gicel\nimport Prelude\nimport Shared\nfromB := helper 2\n")
	mainSrc := "-- gicel: --module A=./a.gicel\n-- gicel: --module B=./b.gicel\nimport Prelude\nimport A\nimport B\nmain := (fromA, fromB)\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	res, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err != nil {
		t.Fatal(err)
	}
	// Shared should appear once (deduplicated).
	names := make(map[string]bool)
	for _, m := range res.Modules {
		if names[m.Name] {
			t.Fatalf("duplicate module: %s", m.Name)
		}
		names[m.Name] = true
	}
	if !names["Shared"] || !names["A"] || !names["B"] {
		t.Fatalf("expected Shared, A, B; got %v", names)
	}
}

func TestResolve_CircularDependency(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.gicel", "-- gicel: --module B=./b.gicel\nimport B\nmain := 1\n")
	writeFile(t, dir, "b.gicel", "-- gicel: --module A=./a.gicel\nimport A\nmain := 2\n")
	mainSrc := "-- gicel: --module A=./a.gicel\nimport A\nmain := 1\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	_, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Fatalf("expected circular error, got: %v", err)
	}
}

func TestResolve_PathContainment(t *testing.T) {
	dir := t.TempDir()
	// Create a file outside the project root.
	outsideDir := t.TempDir()
	writeFile(t, outsideDir, "evil.gicel", "main := 1\n")

	relPath, _ := filepath.Rel(dir, filepath.Join(outsideDir, "evil.gicel"))
	mainSrc := "-- gicel: --module Evil=" + relPath + "\nimport Evil\nmain := 1\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	_, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err == nil {
		t.Fatal("expected error for path outside project root")
	}
	if !strings.Contains(err.Error(), "outside project root") {
		t.Fatalf("expected containment error, got: %v", err)
	}
}

func TestResolve_ConflictingPaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "v1.gicel", "main := 1\n")
	writeFile(t, dir, "v2.gicel", "main := 2\n")
	mainSrc := "-- gicel: --module Lib=./v1.gicel\n-- gicel: --module Lib=./v2.gicel\nimain := 1\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	_, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err == nil {
		t.Fatal("expected error for conflicting paths")
	}
	if !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("expected conflicting error, got: %v", err)
	}
}

func TestResolve_Recursion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "lib.gicel", "-- gicel: --recursion\nimport Prelude\nfac :: Int -> Int\nfac := fix (\\self n. if n == 0 then 1 else n * self (n - 1))\n")
	mainSrc := "-- gicel: --module Lib=./lib.gicel\nimport Prelude\nimport Lib\nmain := fac 5\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	res, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Recursion {
		t.Fatal("expected Recursion=true (propagated from lib)")
	}
}

func TestResolve_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	mainSrc := "-- gicel: --module Lib=./nonexistent.gicel\nmain := 1\n"
	writeFile(t, dir, "main.gicel", mainSrc)

	_, err := Resolve(mainSrc, filepath.Join(dir, "main.gicel"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
