package stdlib

import (
	"context"
	"unicode"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- Rune classification primitives ---

func isAlphaImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(unicode.IsLetter(r)), ce, nil
}

func isDigitImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(unicode.IsDigit(r)), ce, nil
}

func isAlphaNumImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(unicode.IsLetter(r) || unicode.IsDigit(r)), ce, nil
}

func isSpaceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(unicode.IsSpace(r)), ce, nil
}

func isUpperImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(unicode.IsUpper(r)), ce, nil
}

func isLowerImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return boolVal(unicode.IsLower(r)), ce, nil
}

// --- Rune conversion primitives ---

func runeToIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(r)}, ce, nil
}

func intToRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "rune")
	if err != nil {
		return nil, ce, err
	}
	if n < 0 || n > 0x10FFFF || (n >= 0xD800 && n <= 0xDFFF) {
		return &eval.ConVal{Con: eval.MaybeNothing}, ce, nil
	}
	return &eval.ConVal{Con: eval.MaybeJust, Args: []eval.Value{&eval.HostVal{Inner: rune(n)}}}, ce, nil
}

func digitToIntImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	if r >= '0' && r <= '9' {
		return &eval.ConVal{Con: eval.MaybeJust, Args: []eval.Value{&eval.HostVal{Inner: int64(r - '0')}}}, ce, nil
	}
	if r >= 'a' && r <= 'f' {
		return &eval.ConVal{Con: eval.MaybeJust, Args: []eval.Value{&eval.HostVal{Inner: int64(r - 'a' + 10)}}}, ce, nil
	}
	if r >= 'A' && r <= 'F' {
		return &eval.ConVal{Con: eval.MaybeJust, Args: []eval.Value{&eval.HostVal{Inner: int64(r - 'A' + 10)}}}, ce, nil
	}
	return &eval.ConVal{Con: eval.MaybeNothing}, ce, nil
}

func showRuneImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRune(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: "'" + string(r) + "'"}, ce, nil
}
