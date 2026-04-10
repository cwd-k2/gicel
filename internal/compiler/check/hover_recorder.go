package check

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// HoverRecorder receives hover-relevant events during type checking.
// When non-nil in CheckConfig, each method is called at the appropriate
// point. When nil, no recording occurs. Replaces the six individual
// callback fields (TypeRecorder, OperatorRecorder, VarDocRecorder,
// DeclRecorder, PostCheckHook, PostGeneralize) that previously served
// this purpose.
type HoverRecorder interface {
	// RecordType is called for each inferred/checked expression with
	// the raw (pre-zonk) type.
	RecordType(sp span.Span, ty types.Type)

	// RecordOperator is called for operator tokens with their resolved
	// type, name, and defining module.
	RecordOperator(sp span.Span, name, module string, ty types.Type)

	// RecordVarDoc is called for variable references so the
	// implementation can attach documentation to the span.
	RecordVarDoc(sp span.Span, name string)

	// RecordDecl is called for declaration hover entries (type aliases,
	// impl declarations). declType is "alias" or "impl".
	RecordDecl(sp span.Span, declType, name string, ty types.Type)

	// Rezonk is called at lifecycle points where the zonk function
	// should be applied to all previously recorded types. Called
	// during generalization (with temporary meta solutions active)
	// and after checking completes (with final solutions).
	Rezonk(zonk func(types.Type) types.Type)
}
