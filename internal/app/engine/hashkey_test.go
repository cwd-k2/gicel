// hashkey tests — correctness and alloc invariants.
// Does NOT cover: cache key callers (see runtimecache_test.go).
package engine

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
)

// --- correctness ---

func TestHashString_MatchesSum256(t *testing.T) {
	cases := []string{
		"",
		"a",
		"hello world",
		strings.Repeat("x", 4096),
		strings.Repeat("λ✨", 1024), // multi-byte runes
	}
	for _, s := range cases {
		got := hashString(s)
		want := sha256.Sum256([]byte(s))
		if got != want {
			t.Errorf("hashString(%q) = %x, want %x", s, got, want)
		}
	}
}

func TestKeyHasher_MatchesBufferRoute(t *testing.T) {
	// For any sequence of Write/WriteByte/WriteString, streaming into
	// keyHasher must produce the same digest as accumulating in a
	// bytes.Buffer and hashing at the end. This is the invariant that
	// makes keyHasher a drop-in replacement for the *bytes.Buffer
	// middleman path.
	writeProgram := func() [][3]any {
		// (op, arg) pairs. op: 0=Write([]byte), 1=WriteByte(byte), 2=WriteString(string)
		return [][3]any{
			{2, "module=", nil},
			{2, "Prelude", nil},
			{1, byte(0), nil},
			{0, []byte{0x01, 0x02, 0x03}, nil},
			{2, "λ✨", nil},
			{1, byte('='), nil},
			{2, strings.Repeat("payload", 128), nil},
			{1, byte(0), nil},
		}
	}

	// via bytes.Buffer → Sum256
	var buf bytes.Buffer
	for _, op := range writeProgram() {
		switch op[0] {
		case 0:
			buf.Write(op[1].([]byte))
		case 1:
			buf.WriteByte(op[1].(byte))
		case 2:
			buf.WriteString(op[1].(string))
		}
	}
	want := sha256.Sum256(buf.Bytes())

	// via keyHasher
	h := newKeyHasher()
	for _, op := range writeProgram() {
		switch op[0] {
		case 0:
			_, _ = h.Write(op[1].([]byte))
		case 1:
			_ = h.WriteByte(op[1].(byte))
		case 2:
			_, _ = h.WriteString(op[1].(string))
		}
	}
	got := h.Sum()

	if got != want {
		t.Errorf("keyHasher digest differs from buffered route\n got:  %x\n want: %x", got, want)
	}
}

// --- alloc invariants (re-occurrence guards) ---
//
// These tests lock in the zero-alloc property of the unsafe.Slice and
// [1]byte patterns. If a future refactor replaces `unsafe.Slice(
// unsafe.StringData(s), len(s))` with the idiomatic `[]byte(s)`, or
// rewrites WriteByte to allocate, these tests fire with a clear
// message pointing to the root cause.
//
// Context: before this fix, CHANGELOG claimed "unsafe.StringData view
// for source hashing" but the optimization was silently lost in a
// later refactor, costing ~650 MB of [:]byte copies per
// BenchmarkEngineCompileLarge run. Tests like these would have caught
// it the day it regressed. See tmp/perf/investigation.md.

func TestHashString_ZeroAlloc(t *testing.T) {
	// A 4 KB string is large enough that a []byte(s) copy would be
	// unmistakable in AllocsPerRun. A zero-copy path must not alloc
	// regardless of input size.
	s := strings.Repeat("x", 4096)
	allocs := testing.AllocsPerRun(100, func() {
		_ = hashString(s)
	})
	if allocs != 0 {
		t.Fatalf("hashString allocated %v times per call; expected 0. "+
			"A []byte(s) copy may have been reintroduced — use "+
			"unsafe.Slice(unsafe.StringData(s), len(s)) instead.", allocs)
	}
}

func TestKeyHasher_WriteStringZeroAlloc(t *testing.T) {
	h := newKeyHasher()
	s := strings.Repeat("x", 4096)
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = h.WriteString(s)
	})
	if allocs != 0 {
		t.Fatalf("keyHasher.WriteString allocated %v times per call; "+
			"expected 0. The unsafe.Slice may have been replaced by "+
			"[]byte(s).", allocs)
	}
}

func TestKeyHasher_WriteByteZeroAlloc(t *testing.T) {
	h := newKeyHasher()
	allocs := testing.AllocsPerRun(100, func() {
		_ = h.WriteByte(0)
	})
	if allocs != 0 {
		t.Fatalf("keyHasher.WriteByte allocated %v times per call; "+
			"expected 0. A []byte{b} slice literal may have been "+
			"reintroduced — use a stack-local [1]byte instead.", allocs)
	}
}

func TestKeyHasher_SumZeroAlloc(t *testing.T) {
	h := newKeyHasher()
	_, _ = h.WriteString("warmup")
	allocs := testing.AllocsPerRun(100, func() {
		_ = h.Sum()
	})
	// One alloc is tolerated here: hash.Hash.Sum internally appends to
	// a nil slice if we pass nil, but we pass out[:0] precisely to
	// avoid that. Zero is the expected value.
	if allocs != 0 {
		t.Fatalf("keyHasher.Sum allocated %v times per call; expected 0. "+
			"h.Sum(out[:0]) should reuse the array-backed slice without "+
			"a heap allocation.", allocs)
	}
}
