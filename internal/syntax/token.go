package syntax

import "github.com/cwd-k2/gomputation/internal/span"

// TokenKind identifies the type of a token.
type TokenKind int

const (
	TokEOF TokenKind = iota
	TokNewline

	// Delimiters
	TokLParen    // (
	TokRParen    // )
	TokLBrace    // {
	TokRBrace    // }
	TokComma     // ,
	TokSemicolon // ;
	TokPipe      // |
	TokAt        // @

	// Punctuation
	TokArrow      // ->
	TokLArrow     // <-
	TokFatArrow   // =>
	TokColonColon // ::
	TokColonEq    // :=
	TokColon      // :
	TokDot        // .
	TokBackslash  // \
	TokUnderscore // _
	TokEq         // =

	// Keywords (11)
	TokCase
	TokOf
	TokDo
	TokData
	TokType
	TokForall
	TokInfixl
	TokInfixr
	TokInfixn
	TokClass
	TokInstance

	// Identifiers
	TokLower // lowercase-start
	TokUpper // uppercase-start

	// Operators
	TokOp // operator sequence

	// Numeric (fixity declarations only)
	TokIntLit
)

// Token is a single lexical unit.
type Token struct {
	Kind          TokenKind
	Text          string
	S             span.Span
	NewlineBefore bool // true if a newline was skipped before this token
}

var keywords = map[string]TokenKind{
	"case":     TokCase,
	"of":       TokOf,
	"do":       TokDo,
	"data":     TokData,
	"type":     TokType,
	"forall":   TokForall,
	"infixl":   TokInfixl,
	"infixr":   TokInfixr,
	"infixn":   TokInfixn,
	"class":    TokClass,
	"instance": TokInstance,
}

// String returns a human-readable token kind name.
func (k TokenKind) String() string {
	switch k {
	case TokEOF:
		return "EOF"
	case TokLParen:
		return "("
	case TokRParen:
		return ")"
	case TokLBrace:
		return "{"
	case TokRBrace:
		return "}"
	case TokComma:
		return ","
	case TokSemicolon:
		return ";"
	case TokPipe:
		return "|"
	case TokAt:
		return "@"
	case TokArrow:
		return "->"
	case TokLArrow:
		return "<-"
	case TokFatArrow:
		return "=>"
	case TokColonColon:
		return "::"
	case TokColonEq:
		return ":="
	case TokColon:
		return ":"
	case TokDot:
		return "."
	case TokBackslash:
		return "\\"
	case TokUnderscore:
		return "_"
	case TokEq:
		return "="
	case TokLower:
		return "lower"
	case TokUpper:
		return "upper"
	case TokOp:
		return "op"
	case TokIntLit:
		return "int"
	default:
		return "keyword"
	}
}
