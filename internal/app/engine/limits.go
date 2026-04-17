package engine

import (
	"strconv"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// CompilerLimits holds limits that affect type-checked / compiled output.
// Changes to these fields invalidate both module and runtime caches.
// Each field participates in the module-environment fingerprint.
type CompilerLimits struct {
	nestingLimit    int
	maxTFSteps      int
	maxSolverSteps  int
	maxResolveDepth int
}

// writeKey serializes CompilerLimits into b for fingerprint computation.
// The byte format must remain stable to preserve cache compatibility.
func (cl *CompilerLimits) writeKey(b types.KeyWriter) {
	var numBuf [20]byte
	b.Write(strconv.AppendInt(numBuf[:0], int64(cl.nestingLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(cl.maxTFSteps), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(cl.maxSolverSteps), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(cl.maxResolveDepth), 10))
}

// RuntimeLimits holds limits that affect execution but not compilation.
// Changes to these fields invalidate only the runtime cache.
// Each field participates in the runtime-specific fingerprint section.
type RuntimeLimits struct {
	stepLimit  int
	depthLimit int
	allocLimit int64
}

// writeKey serializes RuntimeLimits into b for fingerprint computation.
// The byte format must remain stable to preserve cache compatibility.
func (rl *RuntimeLimits) writeKey(b types.KeyWriter) {
	var numBuf [20]byte
	b.WriteString("rl:")
	b.Write(strconv.AppendInt(numBuf[:0], int64(rl.stepLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], int64(rl.depthLimit), 10))
	b.WriteByte('/')
	b.Write(strconv.AppendInt(numBuf[:0], rl.allocLimit, 10))
	b.WriteByte(0)
}

// PipelineFlags groups pipeline configuration options that are part of
// the runtime fingerprint but not the module fingerprint. Struct placement
// documents the cache invalidation level.
type PipelineFlags struct {
	entryPoint      string
	denyAssumptions bool
	noInline        bool
	verifyIR        bool
}

// effectiveEntryPoint returns the entry point name, defaulting to DefaultEntryPoint.
func (pf *PipelineFlags) effectiveEntryPoint() string {
	if pf.entryPoint == "" {
		return DefaultEntryPoint
	}
	return pf.entryPoint
}

// writeKey serializes PipelineFlags into b for fingerprint computation.
// The byte format must remain stable to preserve cache compatibility.
func (pf *PipelineFlags) writeKey(b types.KeyWriter) {
	ep := pf.effectiveEntryPoint()
	b.WriteString("pf:")
	b.WriteString(ep)
	b.WriteByte('/')
	writeBool(b, pf.denyAssumptions)
	b.WriteByte('/')
	writeBool(b, pf.noInline)
	b.WriteByte('/')
	writeBool(b, pf.verifyIR)
	b.WriteByte(0)
}
