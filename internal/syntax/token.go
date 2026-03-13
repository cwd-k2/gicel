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

	// Keywords (11)
	TokCase
	TokDo
	TokData
	TokType
	TokForall
	TokInfixl
	TokInfixr
	TokInfixn
	TokClass
	TokInstance
	TokImport

	// Identifiers
	TokLower // lowercase-start
	TokUpper // uppercase-start

	// Operators
	TokOp       // operator sequence
	TokBangHash // !# (record projection)

	// Literals
	TokIntLit
	TokStrLit  // "string"
	TokRuneLit // 'c'
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
	"do":       TokDo,
	"data":     TokData,
	"type":     TokType,
	"forall":   TokForall,
	"infixl":   TokInfixl,
	"infixr":   TokInfixr,
	"infixn":   TokInfixn,
	"class":    TokClass,
	"instance": TokInstance,
	"import":   TokImport,
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
	case TokLower:
		return "lower"
	case TokUpper:
		return "upper"
	case TokOp:
		return "op"
	case TokBangHash:
		return "!#"
	case TokIntLit:
		return "int"
	case TokStrLit:
		return "string"
	case TokRuneLit:
		return "rune"
	default:
		return "keyword"
	}
}
