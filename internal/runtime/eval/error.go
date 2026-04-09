package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// RuntimeError represents an error during evaluation.
type RuntimeError struct {
	Message   string
	Detail    Value        // the original value passed to fail (nil if unavailable)
	FailLabel string       // label for named fail effects ("" = anonymous failWith)
	Span      span.Span    // internal: byte offsets
	Source    *span.Source // originating source (populated by evaluator)
	Line      int          // 1-based line number (populated by Runtime)
	Col       int          // 1-based column number (populated by Runtime)
}

func (e *RuntimeError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%d:%d: runtime error: %s", e.Line, e.Col, e.Message)
	}
	return "runtime error: " + e.Message
}
