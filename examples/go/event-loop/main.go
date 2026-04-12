// Example: event-loop — bidirectional communication between Go and GICEL.
//
// Demonstrates a long-lived GICEL execution that receives commands from Go
// via a channel, dispatches them to a GICEL function using the Applier, and
// sends results back. Budget counters are reset between events so each
// handler invocation runs within its own resource envelope.
//
// Architecture:
//
//	Go goroutine              GICEL VM goroutine
//	────────────              ──────────────────
//	bus.In <- cmd        →    _eventLoop blocks on channel
//	                          apply(dispatch, cmd, state) → new state
//	<-bus.Out            ←    sends result back
//
// The GICEL module defines a pure dispatch function. The host primitive
// _eventLoop manages the event loop, state, channel I/O, and budget reset.
// Everything compiles in a single Runtime — no cross-Runtime closure issues.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// Bus carries commands and responses between Go and the GICEL VM.
type Bus struct {
	In  chan string
	Out chan Response
}

// Response is the result of a dispatched command.
type Response struct {
	Value int64
	Err   error
}

// User-defined handlers as a GICEL module. Pure functions — no effects.
const userModule = `
import Prelude

onInc :: Int -> Int
onInc := \n. n + 1

onDec :: Int -> Int
onDec := \n. n - 1

dispatch :: String -> Int -> Int
dispatch := \cmd n.
  if cmd == "inc"
    then onInc n
  else if cmd == "dec"
    then onDec n
  else n
`

// The main source imports the user module and runs the event loop.
// _eventLoop receives User.dispatch as a first-class function and
// drives it from Go. No --recursion needed — the loop is in Go.
const mainSource = `
import Prelude
import User (dispatch)

_eventLoop :: (String -> Int -> Int) -> Computation {} {} Int
_eventLoop := assumption

main := _eventLoop dispatch
`

func main() {
	bus := &Bus{
		In:  make(chan string),
		Out: make(chan Response),
	}

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Register the user module so it shares the same global namespace.
	if err := eng.RegisterModule("User", userModule); err != nil {
		log.Fatal("register module: ", err)
	}

	// _eventLoop: the core primitive. Blocks on bus.In, applies the GICEL
	// dispatch function via Applier, resets budget between events, sends
	// results back on bus.Out.
	eng.RegisterPrim("_eventLoop", func(
		ctx context.Context, ce gicel.CapEnv,
		args []gicel.Value, apply gicel.Applier,
	) (gicel.Value, gicel.CapEnv, error) {
		dispatch := args[0] // User.dispatch :: String -> Int -> Int
		counter := int64(0)

		for {
			select {
			case cmd := <-bus.In:
				// Per-event budget: reset counters so each dispatch gets
				// a fresh step/alloc envelope.
				gicel.ResetBudgetCounters(ctx)

				result, newCe, err := apply.ApplyN(
					dispatch,
					[]gicel.Value{gicel.ToValue(cmd), gicel.ToValue(counter)},
					ce,
				)
				ce = newCe

				if err != nil {
					bus.Out <- Response{Err: err}
					continue
				}

				counter = gicel.MustHost[int64](result)
				bus.Out <- Response{Value: counter}

			case <-ctx.Done():
				return gicel.ToValue(counter), ce, nil
			}
		}
	})

	// Compile everything in a single Runtime.
	rt, err := eng.NewRuntime(context.Background(), mainSource)
	if err != nil {
		log.Fatal("compile: ", err)
	}

	// Start the event loop in a background goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := rt.RunWith(ctx, nil)
		done <- err
	}()

	// --- Send commands from Go ---

	send := func(cmd string) {
		bus.In <- cmd
		resp := <-bus.Out
		if resp.Err != nil {
			fmt.Printf("  %s → error: %v\n", cmd, resp.Err)
		} else {
			fmt.Printf("  %s → %d\n", cmd, resp.Value)
		}
	}

	fmt.Println("--- bidirectional event loop ---")
	send("inc") // 0 → 1
	send("inc") // 1 → 2
	send("inc") // 2 → 3
	send("dec") // 3 → 2
	send("get") // 2 → 2 (else branch)
	send("inc") // 2 → 3

	// Shut down.
	cancel()
	<-done

	fmt.Println("--- done ---")
}
