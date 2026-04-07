// Custom flag types used by the CLI: -e (expression), --max-alloc (byte size),
// --module (repeatable name=path).

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// exprFlag tracks the -e flag value and warns on duplicates.
type exprFlag struct {
	value string
	count int
}

func (e *exprFlag) String() string { return e.value }

func (e *exprFlag) Set(val string) error {
	e.value = val
	e.count++
	return nil
}

// byteSizeFlag parses byte sizes with optional suffixes: KiB, MiB, GiB, KB, MB, GB.
// Plain integers are treated as raw bytes.

// byteSizeFlag parses byte sizes with optional suffixes: KiB, MiB, GiB, KB, MB, GB.
// Plain integers are treated as raw bytes.
type byteSizeFlag struct {
	value int64
}

func (b *byteSizeFlag) String() string { return strconv.FormatInt(b.value, 10) }

func (b *byteSizeFlag) Set(val string) error {
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
		{"GB", 1_000_000_000}, {"MB", 1_000_000}, {"KB", 1_000},
	}
	for _, s := range suffixes {
		trimmed := strings.TrimSpace(val)
		if strings.HasSuffix(trimmed, s.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(trimmed, s.suffix))
			n, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid number before %s: %q", s.suffix, numStr)
			}
			if n < 0 {
				return fmt.Errorf("byte size must be non-negative: %q", val)
			}
			b.value = n * s.mult
			return nil
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil {
		return fmt.Errorf("expected byte count or size with suffix (e.g. 100MiB): %q", val)
	}
	if n < 0 {
		return fmt.Errorf("byte size must be non-negative: %q", val)
	}
	b.value = n
	return nil
}

// moduleFlags is a repeatable flag for --module Name=path pairs.

// moduleFlags is a repeatable flag for --module Name=path pairs.
type moduleFlags []string

func (m *moduleFlags) String() string { return strings.Join(*m, ", ") }

func (m *moduleFlags) Set(val string) error {
	if !strings.Contains(val, "=") {
		return fmt.Errorf("expected Name=path, got %q", val)
	}
	*m = append(*m, val)
	return nil
}

// readSource loads GICEL source from -e string, stdin ("-"), or a file argument.
