package lsp

import "testing"

func TestParseHeader_SingleModule(t *testing.T) {
	hd := ParseHeader("-- gicel: --module Lib=./lib/Lib.gicel\nimport Prelude\n")
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(hd.Modules))
	}
	if hd.Modules[0].Name != "Lib" || hd.Modules[0].Path != "./lib/Lib.gicel" {
		t.Fatalf("unexpected module: %+v", hd.Modules[0])
	}
}

func TestParseHeader_MultipleModules(t *testing.T) {
	src := `-- gicel: --module A=./a.gicel
-- gicel: --module B=./b.gicel
import Prelude
`
	hd := ParseHeader(src)
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

func TestParseHeader_Recursion(t *testing.T) {
	hd := ParseHeader("-- gicel: --recursion\nmain := 42\n")
	if !hd.Recursion {
		t.Fatal("expected Recursion=true")
	}
}

func TestParseHeader_Empty(t *testing.T) {
	hd := ParseHeader("import Prelude\nmain := 42\n")
	if len(hd.Modules) != 0 {
		t.Fatalf("expected 0 modules, got %d", len(hd.Modules))
	}
	if hd.Recursion {
		t.Fatal("expected Recursion=false")
	}
}

func TestParseHeader_MixedComments(t *testing.T) {
	src := `-- This is a regular comment
-- gicel: --module Lib=./lib.gicel
-- Another comment
import Prelude
`
	hd := ParseHeader(src)
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(hd.Modules))
	}
}

func TestParseHeader_StopsAtCode(t *testing.T) {
	src := `-- gicel: --module A=./a.gicel
import Prelude
-- gicel: --module B=./b.gicel
`
	hd := ParseHeader(src)
	// B should not be parsed — it appears after the first code line.
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module (B should be ignored), got %d", len(hd.Modules))
	}
}

func TestParseHeader_Shebang(t *testing.T) {
	src := `#!/usr/bin/env gicel run
-- gicel: --module Lib=./lib.gicel
import Prelude
`
	hd := ParseHeader(src)
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module after shebang, got %d", len(hd.Modules))
	}
}

func TestParseHeader_CombinedDirectives(t *testing.T) {
	hd := ParseHeader("-- gicel: --module Lib=./lib.gicel --recursion\nmain := 42\n")
	if len(hd.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(hd.Modules))
	}
	if !hd.Recursion {
		t.Fatal("expected Recursion=true")
	}
}
