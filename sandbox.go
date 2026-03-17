package gicel

import (
	"context"
	"time"
)

// Sandbox defaults — intentionally more conservative than Engine defaults.
const (
	sandboxDefaultSteps = 100_000
	sandboxDefaultDepth = 100
	sandboxDefaultAlloc = 10 * 1024 * 1024 // 10 MiB
)

// SandboxConfig configures a sandboxed execution.
// All fields are optional; nil config uses conservative defaults.
type SandboxConfig struct {
	Packs    []Pack           // stdlib packs to load (default: none)
	Entry    string           // entry point binding (default: "main")
	Timeout  time.Duration    // execution timeout (default: 5s)
	MaxSteps int              // step limit (default: 100_000)
	MaxDepth int              // depth limit (default: 100)
	MaxAlloc int64            // allocation byte limit (default: 10 MiB)
	Caps     map[string]any   // initial capability environment (nil for empty)
	Bindings map[string]Value // host-provided value bindings (nil for none)
}

// RunSandbox compiles and executes a GICEL program in a single call
// with conservative resource limits. Designed for AI agent use cases.
func RunSandbox(source string, cfg *SandboxConfig) (*RunResult, error) {
	if cfg == nil {
		cfg = &SandboxConfig{}
	}

	entry := cfg.Entry
	if entry == "" {
		entry = "main"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	maxSteps := cfg.MaxSteps
	if maxSteps <= 0 {
		maxSteps = sandboxDefaultSteps
	}
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = sandboxDefaultDepth
	}
	maxAlloc := cfg.MaxAlloc
	if maxAlloc <= 0 {
		maxAlloc = sandboxDefaultAlloc
	}

	eng := NewEngine()
	eng.SetStepLimit(maxSteps)
	eng.SetDepthLimit(maxDepth)
	eng.SetAllocLimit(maxAlloc)

	for _, p := range cfg.Packs {
		if err := p(eng); err != nil {
			return nil, err
		}
	}

	// Start the timeout clock before compilation so that compile + execute
	// total is bounded by the configured timeout.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rt, err := eng.NewRuntime(source)
	if err != nil {
		return nil, err
	}

	return rt.RunWith(ctx, &RunOptions{
		Entry:    entry,
		Caps:     cfg.Caps,
		Bindings: cfg.Bindings,
	})
}
