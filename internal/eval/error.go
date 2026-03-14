package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/span"
)

// RuntimeError represents an error during evaluation.
type RuntimeError struct {
	Message string
	Span    span.Span
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("runtime error: %s", e.Message)
}
