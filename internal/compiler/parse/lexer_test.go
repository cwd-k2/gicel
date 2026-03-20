// Lexer tests — operator token boundary guards.
// Does NOT cover: string/comment/numeric scanning (lexer_probe_test.go).

package parse

import (
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// --- =: guard ---

func TestLexer_EqColonEqReportsError(t *testing.T) {
	tokens, es := lexWithErrors("=:=")
	if tokens[0].Kind != TokOp || tokens[0].Text != "=:=" {
		t.Errorf("expected TokOp(\"=:=\"), got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if !es.HasErrors() {
		t.Fatal("expected error for =:= containing reserved =:")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrReservedInOp && strings.Contains(e.Message, "=:") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ErrReservedInOp mentioning =:, got: %s", es.Format())
	}
}

func TestLexer_EqColonStillWorks(t *testing.T) {
	tokens := lex("=:")
	if tokens[0].Kind != TokEqColon {
		t.Errorf("expected TokEqColon for '=:', got %v", tokens[0].Kind)
	}
	tokens2 := lex("=: x")
	if tokens2[0].Kind != TokEqColon {
		t.Errorf("expected TokEqColon for '=: x', got %v", tokens2[0].Kind)
	}
}

// --- => guard ---

func TestLexer_FatArrowStillWorks(t *testing.T) {
	tokens := lex("=>")
	if tokens[0].Kind != TokFatArrow {
		t.Errorf("expected TokFatArrow for '=>', got %v", tokens[0].Kind)
	}
	tokens2 := lex("=> x")
	if tokens2[0].Kind != TokFatArrow {
		t.Errorf("expected TokFatArrow for '=> x', got %v", tokens2[0].Kind)
	}
}

func TestLexer_FatArrowExtendedSingleOperator(t *testing.T) {
	tokens := lex("=>=")
	if tokens[0].Kind != TokOp || tokens[0].Text != "=>=" {
		t.Errorf("expected TokOp(\"=>=\"), got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// --- -> guard ---

func TestLexer_ArrowStillWorks(t *testing.T) {
	tokens := lex("->")
	if tokens[0].Kind != TokArrow {
		t.Errorf("expected TokArrow for '->', got %v", tokens[0].Kind)
	}
	tokens2 := lex("-> x")
	if tokens2[0].Kind != TokArrow {
		t.Errorf("expected TokArrow for '-> x', got %v", tokens2[0].Kind)
	}
}

func TestLexer_ArrowExtendedSingleOperator(t *testing.T) {
	tokens := lex("->>")
	if tokens[0].Kind != TokOp || tokens[0].Text != "->>" {
		t.Errorf("expected TokOp(\"->>\"), got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// --- <- guard ---

func TestLexer_LArrowStillWorks(t *testing.T) {
	tokens := lex("<-")
	if tokens[0].Kind != TokLArrow {
		t.Errorf("expected TokLArrow for '<-', got %v", tokens[0].Kind)
	}
	tokens2 := lex("<- x")
	if tokens2[0].Kind != TokLArrow {
		t.Errorf("expected TokLArrow for '<- x', got %v", tokens2[0].Kind)
	}
}

func TestLexer_LArrowExtendedSingleOperator(t *testing.T) {
	tokens := lex("<->")
	if tokens[0].Kind != TokOp || tokens[0].Text != "<->" {
		t.Errorf("expected TokOp(\"<->\"), got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// --- := guard ---

func TestLexer_ColonEqReportsError(t *testing.T) {
	tokens, es := lexWithErrors(":==")
	if tokens[0].Kind != TokOp || tokens[0].Text != ":==" {
		t.Errorf("expected TokOp(\":==\"), got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if !es.HasErrors() {
		t.Fatal("expected error for :== containing reserved :=")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrReservedInOp && strings.Contains(e.Message, ":=") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ErrReservedInOp mentioning :=, got: %s", es.Format())
	}
}

func TestLexer_ColonEqStillWorks(t *testing.T) {
	tokens := lex(":=")
	if tokens[0].Kind != TokColonEq {
		t.Errorf("expected TokColonEq for ':=', got %v", tokens[0].Kind)
	}
	tokens2 := lex(":= x")
	if tokens2[0].Kind != TokColonEq {
		t.Errorf("expected TokColonEq for ':= x', got %v", tokens2[0].Kind)
	}
}
