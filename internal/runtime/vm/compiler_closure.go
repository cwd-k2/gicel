// Compiler — child proto builders and capture resolution.
// These functions operate across a parent frame and a freshly entered
// child frame; grouping them makes the dual-frame invariant explicit.
// Does NOT cover: IR node compilation (compiler_expr.go).

package vm

import (
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

func (c *Compiler) compileMergeChildProto(body ir.Core, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = true
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	c.compileExpr(body, false)
	c.emit(OpForceEffectful)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// --- child proto compilation ---

// compileChildProto compiles a nested function or thunk body.
// Capture resolution runs on the current (parent) frame before pushing the child.
func (c *Compiler) compileChildProto(paramName string, body ir.Core, isThunk bool, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = isThunk
	if paramName != "" {
		f.params = []string{paramName}
	}
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	if paramName != "" {
		c.allocLocal(paramName)
	}
	c.compileExpr(body, true)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// compileFixProto compiles a Fix body (self-referential closure or thunk).
// compileFixProto compiles a Fix body (self-referential closure or thunk).
func (c *Compiler) compileFixProto(selfName, paramName string, body ir.Core, isThunk bool, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = isThunk
	if paramName != "" {
		f.params = []string{paramName}
	}
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	selfSlot := c.allocLocal(selfName)
	f.fixSelfSlot = selfSlot
	if paramName != "" {
		c.allocLocal(paramName)
	}
	c.compileExpr(body, true)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// compileFixMultiProto compiles a Fix body with multiple parameters (flattened lambda chain).
// compileFixMultiProto compiles a Fix body with multiple parameters (flattened lambda chain).
func (c *Compiler) compileFixMultiProto(selfName string, params []string, body ir.Core, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = false
	f.params = params
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	selfSlot := c.allocLocalWithArity(selfName, len(params))
	f.fixSelfSlot = selfSlot
	for _, param := range params {
		c.allocLocal(param)
	}
	c.compileExpr(body, true)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// resolveCapturesFiltered resolves free variable names to parent-frame local slots,
// filtering out globals (which the child accesses via LOAD_GLOBAL).
// resolveCapturesFiltered resolves free variable names to parent-frame local slots,
// filtering out globals (which the child accesses via LOAD_GLOBAL).
func (c *Compiler) resolveCapturesFiltered(fv []string, fvIndices []int) ([]string, []int) {
	if len(fv) == 0 {
		return nil, nil
	}
	f := c.top() // current frame = parent of the about-to-be-created child
	var names []string
	var slots []int
	for i, name := range fv {
		if slot, ok := resolveLocalInFrame(f, name); ok {
			names = append(names, name)
			slots = append(slots, slot)
		} else if fvIndices != nil && i < len(fvIndices) && fvIndices[i] >= 0 {
			names = append(names, name)
			slots = append(slots, fvIndices[i])
		}
	}
	return names, slots
}

// resolveLocalInFrame searches a specific frame's locals by name.
// resolveLocalInFrame searches a specific frame's locals by name.
func resolveLocalInFrame(f *frame, name string) (int, bool) {
	for i := len(f.locals) - 1; i >= 0; i-- {
		if f.locals[i].name == name {
			return f.locals[i].slot, true
		}
	}
	return -1, false
}
