// Example: session — binary session types via Atkey indexed monads.
//
// Protocol compliance is enforced by the type checker. The host provides
// send/recv/close primitives; the GICEL program composes them into a
// protocol-compliant session. Type errors reject protocol violations
// at compile time.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

const source = `
import Prelude

data Mult := Unrestricted | Affine | Linear
data Send s := MkSend
data Recv s := MkRecv
data End := MkEnd

-- Session operations (host-provided)
send :: \s. Computation { ch: Send s @Linear } { ch: s @Linear } Int
send := assumption

recv :: \s. Computation { ch: Recv s @Linear } { ch: s @Linear } Int
recv := assumption

close :: Computation { ch: End @Linear } {} ()
close := assumption

-- Ping-pong protocol: send a number, receive a response, close.
main :: Computation { ch: Send (Recv End) @Linear } {} Int
main := do {
  send;
  response <- recv;
  close;
  pure response
}
`

func main() {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Channel state for the session (simulates the other endpoint).
	var lastSent int

	eng.RegisterPrim("send", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		lastSent = 42
		fmt.Printf("send: %d\n", lastSent)
		return gicel.ToValue(lastSent), capEnv, nil
	})

	eng.RegisterPrim("recv", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		response := lastSent * 2
		fmt.Printf("recv: %d\n", response)
		return gicel.ToValue(response), capEnv, nil
	})

	eng.RegisterPrim("close", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		fmt.Println("close: session ended")
		return gicel.ToValue(nil), capEnv, nil
	})

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		log.Fatal("compile error: ", err)
	}

	// Provide the initial capability: ch in state Send (Recv End).
	caps := map[string]any{"ch": nil}

	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		log.Fatal("runtime error: ", err)
	}
	fmt.Printf("result: %v\n", result.Value)
	// Output:
	// send: 42
	// recv: 84
	// close: session ended
	// result: 84
}
