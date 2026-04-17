package engine

import (
	"crypto/sha256"
	"hash"
	"unsafe"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Package notes — streaming fingerprint sinks.
//
// Historical context: fingerprint computation (moduleEnvFingerprint,
// runtimeFingerprint) used to accumulate every byte into a reusable
// *bytes.Buffer, then hand buf.Bytes() to sha256.Sum256. The buffer was
// a "universal sink" — convenient, but structurally a middleman: the
// producer (WriteTypeKey, writeModuleEnvSection) is already polymorphic
// over the types.KeyWriter interface, and sha256.Hash already implements
// io.Writer. An adapter that widens sha256.Hash to KeyWriter lets the
// producer stream directly into the hash state with no buffer.
//
// Two helpers live here:
//
//   keyHasher   — types.KeyWriter adapter around a streaming sha256.
//                 Used where many fields are written (env / runtime
//                 fingerprint).
//   hashString  — single-shot SHA-256 of a string without the
//                 `[]byte(s)` copy. Used where only one value is hashed
//                 (source → cache key).
//
// Both use unsafe.Slice(unsafe.StringData(s), len(s)) to avoid the
// implicit string-to-[]byte copy. This is LOAD-BEARING for the
// fingerprint hot path (see tmp/perf/investigation.md). Tests in
// hashkey_test.go pin the zero-alloc invariant; refactors that
// silently replace unsafe.Slice with []byte(s) will be caught there.

// keyHasher is a zero-allocation KeyWriter backed by a streaming
// SHA-256. It satisfies types.KeyWriter so that any producer written
// against that interface (e.g. WriteTypeKey, writeModuleEnvSection)
// can hash directly without an intermediate *bytes.Buffer.
//
// Constructed via newKeyHasher. The single allocation happens there;
// all subsequent methods reuse the struct's in-place buffers.
//
// Not safe for concurrent use — reflecting the caller invariant
// (Engine is single-goroutine).
//
// Why pointer receiver + struct-owned buffers: hash.Hash is an
// interface, so the compiler cannot prove that Write/Sum won't
// retain the []byte argument. Any []byte derived from a stack array
// therefore escapes. Holding the byte/digest scratch buffers as
// fields of a heap-allocated *keyHasher moves "escape" from per-call
// to per-hasher (amortized once per fingerprint).
type keyHasher struct {
	h       hash.Hash
	byteBuf [1]byte  // reused by WriteByte
	digest  [32]byte // reused by Sum
}

// Compile-time assertion: *keyHasher must satisfy types.KeyWriter.
// If the interface gains a method, this line fails first, directing
// the implementer here rather than into a chain of obscure errors.
var _ types.KeyWriter = (*keyHasher)(nil)

// newKeyHasher returns a fresh streaming SHA-256 sink.
func newKeyHasher() *keyHasher { return &keyHasher{h: sha256.New()} }

// Write satisfies io.Writer.
func (w *keyHasher) Write(p []byte) (int, error) { return w.h.Write(p) }

// WriteByte satisfies io.ByteWriter.
//
// Reuses w.byteBuf (a field of the heap-allocated keyHasher) so the
// slice passed to hash.Hash.Write does not allocate per call. A naive
// `w.h.Write([]byte{b})` literal escapes through the interface on
// every call — see the struct doc for the rationale.
func (w *keyHasher) WriteByte(b byte) error {
	w.byteBuf[0] = b
	_, err := w.h.Write(w.byteBuf[:])
	return err
}

// WriteString satisfies io.StringWriter with zero allocations.
//
// The unsafe.Slice here is LOAD-BEARING. A naive `[]byte(s)` copies
// s into a new byte slice — on the fingerprint hot path that rehashes
// Prelude sources (~20 KB each) on every fresh Engine, that copy
// dominates allocations. Since hash.Hash.Write is strictly read-only
// and Go strings are immutable, a zero-copy view is safe.
//
// TestKeyHasher_WriteStringZeroAlloc pins the invariant. Refactors
// that "clean up" the unsafe call will fail that test.
func (w *keyHasher) WriteString(s string) (int, error) {
	return w.h.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
}

// Sum returns the current SHA-256 digest as a fixed-size array.
//
// Reuses w.digest (a field of the heap-allocated keyHasher) as the
// scratch buffer for hash.Hash.Sum. Passing a stack array's slice
// would force it to escape through the Sum interface call, costing
// one allocation per Sum.
func (w *keyHasher) Sum() [32]byte {
	w.h.Sum(w.digest[:0])
	return w.digest
}

// hashString returns sha256.Sum256(s) without copying s into a []byte.
//
// The idiomatic `sha256.Sum256([]byte(s))` allocates a fresh slice
// whose length equals len(s). For a single ~20 KB module source
// hashed once per fresh Engine × thousands of compile iterations,
// this copy became the second-largest allocation source in
// BenchmarkEngineCompileLarge (see tmp/perf/investigation.md).
//
// unsafe.Slice(unsafe.StringData(s), len(s)) yields a read-only
// view into the string's underlying bytes. SHA-256 never mutates
// its input and Go strings are immutable, so the view is safe for
// the duration of the call.
//
// TestHashString_ZeroAlloc pins the invariant.
func hashString(s string) [32]byte {
	return sha256.Sum256(unsafe.Slice(unsafe.StringData(s), len(s)))
}
