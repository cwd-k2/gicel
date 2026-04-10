package env

// IsPrivateName reports whether a name is module-private.
// Private: '_' prefix (user convention) or compiler-generated identifier
// containing '$'. The '$' character is not part of the surface identifier
// grammar (the lexer rejects it), so its presence in a name is an injective
// signal of compiler generation — not a heuristic. Operator names
// (e.g., <$>, $, +>) are never private even if they contain '$'.
func IsPrivateName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if name[0] == '_' {
		return true
	}
	// Compiler-generated names contain '$' in identifier context.
	// Operators (all non-alphanumeric) are exempt.
	if IsOperatorName(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] == '$' {
			return true
		}
	}
	return false
}

// IsOperatorName returns true if the name is an operator (all symbol characters).
func IsOperatorName(name string) bool {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return false
		}
	}
	return true
}
