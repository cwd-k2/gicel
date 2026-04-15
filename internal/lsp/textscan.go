// Lexical text scanning utilities for LSP position-based queries.
// These are pure functions operating on source text with no server state dependency.
package lsp

import (
	"unicode"
	"unicode/utf8"
)

// extractQualifiedPrefix returns the module name if the cursor is immediately
// after "Module." (an uppercase identifier followed by a dot). Returns "" if
// no qualified prefix is detected. offset is the byte position of the cursor.
// Uses rune-level scanning consistent with identifierAtOffset.
func extractQualifiedPrefix(text string, offset int) string {
	if offset <= 0 || offset > len(text) {
		return ""
	}
	// Cursor is right after the dot (trigger character).
	if text[offset-1] != '.' {
		return ""
	}
	// Walk backwards from the dot to find the module name.
	end := offset - 1
	start := scanBackward(text, end, isIdentRune)
	if start >= end {
		return ""
	}
	name := text[start:end]
	// Module names start with an uppercase letter.
	firstRune, _ := utf8.DecodeRuneInString(name)
	if !unicode.IsUpper(firstRune) {
		return ""
	}
	return name
}

// identifierAtOffset extracts the identifier or operator at the given
// byte offset by scanning forwards and backwards. Uses rune-level
// decoding to match the scanner's identifier definition.
func identifierAtOffset(source string, offset int) string {
	if offset < 0 || offset >= len(source) {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(source[offset:])
	if r == utf8.RuneError {
		return ""
	}

	var pred func(rune) bool
	switch {
	case isIdentRune(r):
		pred = isIdentRune
	case isOperatorRune(r):
		pred = isOperatorRune
	default:
		return ""
	}
	start := scanBackward(source, offset, pred)
	end := scanForward(source, offset, pred)
	return source[start:end]
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\''
}

func isOperatorRune(r rune) bool {
	return r != '_' && r != '\'' && r != '(' && r != ')' &&
		!unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) &&
		r > ' ' && r != ',' && r != ';' && r != '{' && r != '}' && r != '[' && r != ']'
}

// scanBackward returns the start byte offset after scanning backwards from pos
// while pred holds for each decoded rune.
func scanBackward(text string, pos int, pred func(rune) bool) int {
	for pos > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:pos])
		if !pred(r) {
			break
		}
		pos -= size
	}
	return pos
}

// scanForward returns the end byte offset after scanning forwards from pos
// while pred holds for each decoded rune.
func scanForward(text string, pos int, pred func(rune) bool) int {
	for pos < len(text) {
		r, size := utf8.DecodeRuneInString(text[pos:])
		if !pred(r) {
			break
		}
		pos += size
	}
	return pos
}
