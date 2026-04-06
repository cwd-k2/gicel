package protocol

import (
	"runtime"
	"testing"
)

func TestURIToPath(t *testing.T) {
	got := URIToPath("file:///Users/test/main.gicel")
	want := "/Users/test/main.gicel"
	if got != want {
		t.Fatalf("URIToPath: got %q, want %q", got, want)
	}
}

func TestPathToURI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only test")
	}
	got := PathToURI("/Users/test/main.gicel")
	want := DocumentURI("file:///Users/test/main.gicel")
	if got != want {
		t.Fatalf("PathToURI: got %q, want %q", got, want)
	}
}

func TestURIRoundtrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only test")
	}
	path := "/tmp/gicel/test.gicel"
	uri := PathToURI(path)
	back := URIToPath(uri)
	if back != path {
		t.Fatalf("roundtrip failed: %q → %q → %q", path, uri, back)
	}
}
