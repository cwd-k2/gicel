package check

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// DeclKind classifies declaration entries for hover recording.
type DeclKind uint8

const (
	DeclAlias DeclKind = iota // type alias declaration
	DeclImpl                  // impl (instance) declaration
)

// HoverRecorder receives hover-relevant events during type checking.
// When non-nil in CheckConfig, each method is called at the appropriate
// point. When nil, no recording occurs.
type HoverRecorder interface {
	// --- Recording: called during inference/checking ---

	// RecordType is called for each inferred/checked expression with
	// the raw (pre-zonk) type.
	RecordType(sp span.Span, ty types.Type)

	// RecordOperator is called for operator tokens with their resolved
	// type, name, and defining module.
	RecordOperator(sp span.Span, name, module string, ty types.Type)

	// RecordVarDoc is called for variable references so the
	// implementation can attach documentation and module provenance
	// to the span.
	RecordVarDoc(sp span.Span, name, module string)

	// RecordDecl is called for declaration hover entries (type aliases,
	// impl declarations).
	RecordDecl(sp span.Span, kind DeclKind, name string, ty types.Type)

	// --- Lifecycle: called at phase boundaries ---

	// Rezonk must be called after all Record* calls for a given phase.
	// Applied during generalization (with temporary meta solutions active)
	// and after checking completes (with final solutions).
	Rezonk(zonk func(types.Type) types.Type)
}
