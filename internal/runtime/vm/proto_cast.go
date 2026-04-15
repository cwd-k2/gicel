package vm

import "github.com/cwd-k2/gicel/internal/runtime/eval"

// protoOf extracts the *Proto from an eval.Bytecode value.
// This is the single point of type assertion for the eval.Bytecode → *Proto
// conversion, which is guaranteed safe: *Proto is the only implementor of
// eval.Bytecode, created exclusively within this package.
func protoOf(b eval.Bytecode) *Proto {
	return b.(*Proto)
}
