//go:build probe

// Pool overflow probe tests — verify that the compiler raises a typed
// PoolOverflowError when its u16-bounded pools (constant, string, match
// desc, record desc, merge desc, proto) reach the maxPoolSize limit.
//
// These tests pin the panic *type* rather than the message, so that any
// recover site (precompileVM) can rely on `*PoolOverflowError` as the
// stable contract for "compile-time pool exhausted".
//
// Does NOT cover: end-to-end recover path (engine package), per-pool
// content-distinct uniqueness (the pools intern values, so distinct values
// are required to actually fill them).

package vm

import (
	"errors"
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// expectPoolOverflow recovers a panic, verifies it is a *PoolOverflowError
// with the expected pool name, and reports a failure otherwise. Returns
// true if a matching panic was caught.
func expectPoolOverflow(t *testing.T, expectPool string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected *PoolOverflowError panic, got none")
		}
		var poe *PoolOverflowError
		if !errors.As(asError(r), &poe) {
			t.Fatalf("expected *PoolOverflowError, got %T: %v", r, r)
		}
		if poe.Pool != expectPool {
			t.Errorf("expected pool=%q, got %q", expectPool, poe.Pool)
		}
	}()
	fn()
}

// asError converts a panic value (interface{}) to error if it implements it.
func asError(v interface{}) error {
	if e, ok := v.(error); ok {
		return e
	}
	return nil
}

func TestProbeC3_ConstantPoolOverflow(t *testing.T) {
	c := NewCompiler(nil, nil)
	c.enterFrame()
	expectPoolOverflow(t, "constant", func() {
		// Add maxPoolSize+1 distinct constants. Use HostVal[int64] which
		// compares by value, so each i produces a fresh interned slot.
		for i := 0; i <= maxPoolSize; i++ {
			c.addConstant(eval.IntVal(int64(i)))
		}
	})
}

func TestProbeC3_StringPoolOverflow(t *testing.T) {
	c := NewCompiler(nil, nil)
	c.enterFrame()
	expectPoolOverflow(t, "string", func() {
		// Distinct strings — interning by value means we need fresh content.
		for i := 0; i <= maxPoolSize; i++ {
			c.addString("s" + intToStr(i))
		}
	})
}

func TestProbeC3_MatchDescPoolOverflow(t *testing.T) {
	c := NewCompiler(nil, nil)
	c.enterFrame()
	expectPoolOverflow(t, "match desc", func() {
		// MatchDesc has no equality interning — every Add increments the pool.
		for i := 0; i <= maxPoolSize; i++ {
			c.addMatchDesc(MatchDesc{})
		}
	})
}

func TestProbeC3_RecordDescPoolOverflow(t *testing.T) {
	c := NewCompiler(nil, nil)
	c.enterFrame()
	expectPoolOverflow(t, "record desc", func() {
		for i := 0; i <= maxPoolSize; i++ {
			c.addRecordDesc(RecordDesc{})
		}
	})
}

func TestProbeC3_MergeDescPoolOverflow(t *testing.T) {
	c := NewCompiler(nil, nil)
	c.enterFrame()
	expectPoolOverflow(t, "merge desc", func() {
		for i := 0; i <= maxPoolSize; i++ {
			c.addMergeDesc(MergeDesc{})
		}
	})
}

func TestProbeC3_ProtoPoolOverflow(t *testing.T) {
	c := NewCompiler(nil, nil)
	c.enterFrame()
	expectPoolOverflow(t, "proto", func() {
		for i := 0; i <= maxPoolSize; i++ {
			c.addProto(&Proto{})
		}
	})
}

// intToStr is a tiny helper to avoid pulling in strconv just for tests.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
