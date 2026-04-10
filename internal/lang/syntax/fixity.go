package syntax

// Fixity holds operator precedence and associativity.
// Defined in syntax (not parse) because fixity is a language-level
// grammar concept, not a parser-implementation concern. Consumed by
// the parser, module store, and pipeline.
type Fixity struct {
	Assoc Assoc
	Prec  int
}
