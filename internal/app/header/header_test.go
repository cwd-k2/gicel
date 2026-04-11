// Header parse tests — directive extraction from file headers.
// Does NOT cover: recursive resolution (resolve_test.go).

package header

import "testing"

func TestParse_SingleModule(t *testing.T) {
	hd := Parse("-- gicel: --module Lib=./lib/Lib.gicel\nimport Prelude\n")
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(hd.Modules))
	}
	if hd.Modules[0].Name != "Lib" || hd.Modules[0].Path != "./lib/Lib.gicel" {
		t.Fatalf("unexpected module: %+v", hd.Modules[0])
	}
}

func TestParse_MultipleModules(t *testing.T) {
	src := "-- gicel: --module A=./a.gicel\n-- gicel: --module B=./b.gicel\nimport Prelude\n"
	hd := Parse(src)
	if len(hd.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(hd.Modules))
	}
	if hd.Modules[0].Name != "A" {
		t.Fatalf("expected A, got %s", hd.Modules[0].Name)
	}
	if hd.Modules[1].Name != "B" {
		t.Fatalf("expected B, got %s", hd.Modules[1].Name)
	}
}

func TestParse_Recursion(t *testing.T) {
	hd := Parse("-- gicel: --recursion\nmain := 42\n")
	if !hd.Recursion {
		t.Fatal("expected Recursion=true")
	}
}

func TestParse_Empty(t *testing.T) {
	hd := Parse("import Prelude\nmain := 42\n")
	if len(hd.Modules) != 0 {
		t.Fatalf("expected 0 modules, got %d", len(hd.Modules))
	}
	if hd.Recursion {
		t.Fatal("expected Recursion=false")
	}
}

func TestParse_MixedComments(t *testing.T) {
	src := "-- This is a regular comment\n-- gicel: --module Lib=./lib.gicel\n-- Another comment\nimport Prelude\n"
	hd := Parse(src)
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(hd.Modules))
	}
}

func TestParse_StopsAtCode(t *testing.T) {
	src := "-- gicel: --module A=./a.gicel\nimport Prelude\n-- gicel: --module B=./b.gicel\n"
	hd := Parse(src)
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module (B should be ignored), got %d", len(hd.Modules))
	}
}

func TestParse_Shebang(t *testing.T) {
	src := "#!/usr/bin/env gicel run\n-- gicel: --module Lib=./lib.gicel\nimport Prelude\n"
	hd := Parse(src)
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module after shebang, got %d", len(hd.Modules))
	}
}

func TestParse_CombinedDirectives(t *testing.T) {
	hd := Parse("-- gicel: --module Lib=./lib.gicel --recursion\nmain := 42\n")
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(hd.Modules))
	}
	if !hd.Recursion {
		t.Fatal("expected Recursion=true")
	}
}

func TestParse_UnknownDirectiveWarning(t *testing.T) {
	hd := Parse("-- gicel: --future-flag\nmain := 42\n")
	if len(hd.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(hd.Warnings))
	}
}

func TestParse_PacksDirective(t *testing.T) {
	hd := Parse("-- gicel: --packs prelude,console\nmain := 42\n")
	if len(hd.Warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %v", len(hd.Warnings), hd.Warnings)
	}
	if hd.Packs != "prelude,console" {
		t.Fatalf("expected Packs=prelude,console, got %q", hd.Packs)
	}
}

func TestParse_DisallowedFlagWarning(t *testing.T) {
	hd := Parse("-- gicel: --timeout 5s\nmain := 42\n")
	if len(hd.Warnings) != 1 {
		t.Fatalf("expected 1 warning for --timeout, got %d", len(hd.Warnings))
	}
}

func TestParse_InvalidModuleSyntaxWarning(t *testing.T) {
	hd := Parse("-- gicel: --module NoEqualsSign\nmain := 42\n")
	if len(hd.Warnings) != 1 {
		t.Fatalf("expected 1 warning for invalid --module, got %d", len(hd.Warnings))
	}
	if len(hd.Modules) != 0 {
		t.Fatalf("expected 0 modules for invalid syntax, got %d", len(hd.Modules))
	}
}
