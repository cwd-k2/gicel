package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// RuntimeError represents an error during evaluation.
type RuntimeError struct {
	Message string
	Span    span.Span    // internal: byte offsets
	Source  *span.Source // originating source (populated by evaluator)
	Line    int          // 1-based line number (populated by Runtime)
	Col     int          // 1-based column number (populated by Runtime)
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("runtime error: %s", e.Message)
}
