package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/reg"
)

// Sandbox defaults — intentionally more conservative than Engine defaults.
const (
	sandboxDefaultSteps = 100_000
	sandboxDefaultDepth = 100
	sandboxDefaultAlloc = 10 * 1024 * 1024 // 10 MiB
)

// SandboxConfig configures a sandboxed execution.
type SandboxConfig struct {
	Packs    []reg.Pack           // stdlib packs to load (default: none)
	Entry    string               // entry point binding (default: "main")
	Timeout  time.Duration        // execution timeout (default: 5s)
	MaxSteps int                  // step limit (default: 100_000)
	MaxDepth int                  // depth limit (default: 100)
	MaxAlloc int64                // allocation byte limit (default: 10 MiB)
	Caps     map[string]any       // initial capability environment (nil for empty)
	Bindings map[string]eval.Value // host-provided value bindings (nil for none)
}

// RunSandbox compiles and executes a GICEL program in a single call
// with conservative resource limits.
func RunSandbox(source string, cfg *SandboxConfig) (result *RunResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("gicel: internal panic: %v", r)
		}
	}()

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
