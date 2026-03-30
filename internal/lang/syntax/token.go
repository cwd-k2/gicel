package syntax

import "github.com/cwd-k2/gicel/internal/infra/span"

// TokenKind identifies the type of a token.
type TokenKind int

const (
	TokEOF TokenKind = iota

	// Delimiters
	TokLParen    // (
	TokRParen    // )
	TokLBrace    // {
	TokRBrace    // }
	TokLBracket  // [
	TokRBracket  // ]
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
	TokTilde      // ~
	TokDashPipe   // -|

	// Keywords (14)
	TokAs
	TokAssumption
	TokCase
	TokDo
	TokForm
	TokType
	TokInfixl
	TokInfixr
	TokInfixn
	TokImpl
	TokImport
	TokIf
	TokThen
	TokElse

	// Identifiers
	TokLower // lowercase-start
	TokUpper // uppercase-start

	// Operators
	TokOp      // operator sequence
	TokDotHash // .# (record projection)

	// Literals
	TokIntLit
	TokDoubleLit // 3.14, 1e10, 1.05e+10
	TokStrLit    // "string"
	TokRuneLit   // 'c'
	TokLabelLit  // #label (type-level label literal)
)

// Token is a single lexical unit.
type Token struct {
	Kind          TokenKind
	Text          string
	S             span.Span
	NewlineBefore bool // true if a newline was skipped before this token
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
	case TokLBracket:
		return "["
	case TokRBracket:
		return "]"
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
	case TokTilde:
		return "~"
	case TokDashPipe:
		return "-|"
	case TokLower:
		return "lower"
	case TokUpper:
		return "upper"
	case TokOp:
		return "op"
	case TokDotHash:
		return ".#"
	case TokIntLit:
		return "int"
	case TokDoubleLit:
		return "double"
	case TokStrLit:
		return "string"
	case TokRuneLit:
		return "rune"
	case TokLabelLit:
		return "label"
	default:
		return "keyword"
	}
}
