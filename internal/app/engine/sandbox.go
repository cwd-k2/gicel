package engine

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// InternalPanicError wraps a recovered panic with its stack trace.
// Error() returns a short message for API consumers; Stack contains
// the full goroutine trace for diagnostics.
type InternalPanicError struct {
	Value any
	Stack []byte
}

func (e *InternalPanicError) Error() string {
	return fmt.Sprintf("gicel: internal panic: %v", e.Value)
}

// Sandbox defaults — intentionally more conservative than Engine defaults.
const (
	sandboxDefaultTimeout       = 5 * time.Second
	sandboxDefaultSteps         = 100_000
	sandboxDefaultDepth         = 100
	sandboxDefaultNesting       = 256
	sandboxDefaultAlloc         = 10 * 1024 * 1024 // 10 MiB
	sandboxDefaultMaxSourceSize = 10 * 1024 * 1024 // 10 MiB
)

// SandboxConfig configures a sandboxed execution.
type SandboxConfig struct {
	Packs           []registry.Pack       // stdlib packs to load (default: none)
	Entry           string                // entry point binding (default: DefaultEntryPoint)
	Timeout         time.Duration         // execution timeout (default: 5s)
	MaxSteps        int                   // step limit (default: 100_000)
	MaxDepth        int                   // depth limit (default: 100)
	MaxNesting      int                   // structural nesting limit (default: 256)
	MaxAlloc        int64                 // allocation byte limit (default: 10 MiB)
	MaxSourceSize   int                   // maximum source size in bytes (default: 10 MiB)
	MaxTFSteps      int                   // type family reduction step limit (default: 50_000)
	MaxSolverSteps  int                   // constraint solver step limit (default: 100_000)
	MaxResolveDepth int                   // instance resolution depth limit (default: 64)
	Caps            map[string]any        // initial capability environment (nil for empty)
	Bindings        map[string]eval.Value // host-provided value bindings (nil for none)

	// Context is the parent context for cancellation propagation.
	// When non-nil, the timeout derives from this context (WithTimeout).
	// When nil, context.Background() is used.
	Context context.Context

	// Explain receives semantic evaluation events. Nil disables explain.
	Explain eval.ExplainHook

	// ExplainDepth controls stdlib suppression (default: ExplainUser).
	ExplainDepth ExplainDepth
}

// RunSandbox compiles and executes a GICEL program in a single call
// with conservative resource limits. The timeout covers main source
// compilation and evaluation. Pack application runs before the timeout
// context is established (packs do not receive a context).
func RunSandbox(source string, cfg *SandboxConfig) (result *RunResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			err = &InternalPanicError{Value: r, Stack: buf[:n]}
		}
	}()

	if cfg == nil {
		cfg = &SandboxConfig{}
	}

	maxSourceSize := cfg.MaxSourceSize
	if maxSourceSize <= 0 {
		maxSourceSize = sandboxDefaultMaxSourceSize
	}
	if len(source) > maxSourceSize {
		return nil, fmt.Errorf("source size %d bytes exceeds maximum %d bytes", len(source), maxSourceSize)
	}

	entry := cfg.Entry
	if entry == "" {
		entry = DefaultEntryPoint
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = sandboxDefaultTimeout
	}
	maxSteps := cfg.MaxSteps
	if maxSteps <= 0 {
		maxSteps = sandboxDefaultSteps
	}
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = sandboxDefaultDepth
	}
	maxNesting := cfg.MaxNesting
	if maxNesting <= 0 {
		maxNesting = sandboxDefaultNesting
	}
	maxAlloc := cfg.MaxAlloc
	if maxAlloc <= 0 {
		maxAlloc = sandboxDefaultAlloc
	}

	parent := cfg.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	eng := NewEngine()
	eng.SetCompileContext(ctx)
	eng.SetStepLimit(maxSteps)
	eng.SetDepthLimit(maxDepth)
	eng.SetNestingLimit(maxNesting)
	eng.SetAllocLimit(maxAlloc)
	if cfg.MaxTFSteps > 0 {
		eng.SetMaxTFSteps(cfg.MaxTFSteps)
	}
	if cfg.MaxSolverSteps > 0 {
		eng.SetMaxSolverSteps(cfg.MaxSolverSteps)
	}
	if cfg.MaxResolveDepth > 0 {
		eng.SetMaxResolveDepth(cfg.MaxResolveDepth)
	}

	for _, p := range cfg.Packs {
		if err := p(eng); err != nil {
			return nil, err
		}
	}

	eng.SetEntryPoint(entry)
	rt, err := eng.NewRuntime(ctx, source)
	if err != nil {
		return nil, err
	}

	return rt.RunWith(ctx, &RunOptions{
		Entry:        entry,
		Caps:         cfg.Caps,
		Bindings:     cfg.Bindings,
		Explain:      cfg.Explain,
		ExplainDepth: cfg.ExplainDepth,
	})
}
