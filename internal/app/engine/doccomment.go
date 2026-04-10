package engine

import (
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// ExtractDocComment scans backwards from declStart in the source text
// and collects consecutive -- comment lines as a doc string. Stops at
// the first non-comment, non-blank line or at the beginning of the file.
// Returns the doc string with -- prefixes stripped, or "" if none found.
func ExtractDocComment(source string, declStart span.Pos) string {
	pos := int(declStart)

	// Skip backwards over horizontal whitespace before the declaration.
	for pos > 0 && (source[pos-1] == ' ' || source[pos-1] == '\t') {
		pos--
	}
	// We should now be at the start of the declaration line or at a newline.
	// Move past the newline to the end of the previous line.
	if pos > 0 && source[pos-1] == '\n' {
		pos--
	} else {
		return "" // declaration starts at the beginning of the file or mid-line
	}

	var lines []string
	for pos > 0 {
		// Find the start of this line.
		lineStart := pos
		for lineStart > 0 && source[lineStart-1] != '\n' {
			lineStart--
		}
		line := source[lineStart:pos]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "--") {
			// Strip the -- prefix and optional single space.
			content := strings.TrimPrefix(trimmed, "--")
			content = strings.TrimPrefix(content, " ")
			lines = append(lines, content)
		} else if trimmed == "" {
			// Empty line stops doc comment collection.
			break
		} else {
			break
		}

		// Move to the end of the previous line.
		pos = lineStart
		if pos > 0 && source[pos-1] == '\n' {
			pos--
		} else {
			break
		}
	}

	if len(lines) == 0 {
		return ""
	}

	// Reverse collected lines (they were collected bottom-up).
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n")
}
