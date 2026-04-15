// Example: event-loop-b — GICEL-owned event loop with channel primitives.
//
// Demonstrates a reusable channel primitive design:
//
//	sendTo   @#label value  — send a value to a named channel (GICEL → Go)
//	recvFrom @#label ()     — receive a value from a named channel (Go → GICEL)
//
// Channels are Go chan values stored in CapEnv by label.
// The primitives mirror getAt/putAt but operate on channel I/O
// instead of direct CapEnv mutation.
//
// Two labels = bidirectional:  @#commands (Go→GICEL),  @#results (GICEL→Go)
//
//	Go goroutine              GICEL VM goroutine
//	────────────              ──────────────────
//	cmdCh <- "inc"       →    recvFrom @#commands ()  → "inc"
//	                          dispatch "inc" state     → newState
//	                          sendTo @#results newState
//	<-resCh              ←    loop newState
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cwd-k2/gicel"
)

// --- Channel primitive implementations ---
// These are reusable across any embedding scenario.

// recvFromImpl: blocks on a Go channel stored in CapEnv.
// args: [label]  (no value argument — matches getAt pattern)
// Resets budget counters — each receive marks a new event boundary.
func recvFromImpl(
	ctx context.Context, ce gicel.CapEnv,
	args []gicel.Value, _ gicel.Applier,
) (gicel.Value, gicel.CapEnv, error) {
	label, ok := gicel.TryHost[string](args[0])
	if !ok {
		return nil, ce, fmt.Errorf("expected label argument")
	}
	ch, err := chGet(ce, label)
	if err != nil {
		return nil, ce, err
	}
	select {
	case val := <-ch:
		gicel.ResetBudgetCounters(ctx)
		return val, ce, nil
	case <-ctx.Done():
		return nil, ce, fmt.Errorf("recvFrom @#%s: cancelled", label)
	}
}

// sendToImpl: writes a value to a Go channel stored in CapEnv.
// args: [label, value]  (matches putAt pattern)
func sendToImpl(
	ctx context.Context, ce gicel.CapEnv,
	args []gicel.Value, _ gicel.Applier,
) (gicel.Value, gicel.CapEnv, error) {
	label, ok := gicel.TryHost[string](args[0])
	if !ok {
		return nil, ce, fmt.Errorf("expected label argument")
	}
	ch, err := chGet(ce, label)
	if err != nil {
		return nil, ce, err
	}
	select {
	case ch <- args[1]:
		return gicel.ToValue(nil), ce, nil
	case <-ctx.Done():
		return nil, ce, fmt.Errorf("sendTo @#%s: cancelled", label)
	}
}

func chGet(ce gicel.CapEnv, label string) (chan gicel.Value, error) {
	raw, ok := ce.Get(label)
	if !ok {
		return nil, fmt.Errorf("channel %s: not found in capabilities", label)
	}
	ch, ok := raw.(chan gicel.Value)
	if !ok {
		return nil, fmt.Errorf("channel %s: capability is not a channel", label)
	}
	return ch, nil
}

// --- User module and main source ---

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

// GICEL owns the loop, state, and dispatch logic.
// The channel primitives are thin I/O pipes — no logic.
const mainSource = `
import Prelude
import User (dispatch)

recvFrom :: \(ch: Label) a (r: Row). Effect { ch: a | r } a
recvFrom := assumption

sendTo :: \(ch: Label) a (r: Row). a -> Effect { ch: a | r } ()
sendTo := assumption

main := fix (\loop state. do {
  cmd <- recvFrom @#commands;
  newState <- pure (dispatch cmd state);
  sendTo @#results newState;
  loop newState
}) 0
`

func main() {
	cmdCh := make(chan gicel.Value)
	resCh := make(chan gicel.Value)

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()

	if err := eng.RegisterModule("User", userModule); err != nil {
		log.Fatal("register module: ", err)
	}

	eng.RegisterPrim("recvFrom", recvFromImpl)
	eng.RegisterPrim("sendTo", sendToImpl)

	rt, err := eng.NewRuntime(context.Background(), mainSource)
	if err != nil {
		log.Fatal("compile: ", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		close(ready)
		_, err := rt.RunWith(ctx, &gicel.RunOptions{
			Caps: map[string]any{
				"#commands": cmdCh,
				"#results":  resCh,
			},
		})
		done <- err
	}()
	<-ready

	// --- Bidirectional communication ---

	send := func(cmd string) {
		select {
		case cmdCh <- gicel.ToValue(cmd):
		case err := <-done:
			log.Fatal("VM error: ", err)
		}
		select {
		case result := <-resCh:
			fmt.Printf("  %s → %s\n", cmd, result)
		case err := <-done:
			log.Fatal("VM error: ", err)
		}
	}

	fmt.Println("--- B pattern: channel primitives ---")
	send("inc") // 0 → 1
	send("inc") // 1 → 2
	send("inc") // 2 → 3
	send("dec") // 3 → 2
	send("get") // 2 → 2
	send("inc") // 2 → 3

	cancel()
	<-done

	fmt.Println("--- done ---")
}
