// Example: event-loop-select — Go-side select with GICEL Variant dispatch.
//
// Go multiplexes multiple event sources (key, tick) using select,
// wraps each as a VariantVal, and sends it on a single channel.
// GICEL receives the Variant and pattern-matches with case.
//
// This demonstrates that Offer-style branching works over channels
// without session protocol machinery — just recvFrom + Variant + case.
//
//	keyCh ──┐
//	        ├──(Go select)──→ eventCh ──→ recvFrom @#events ──→ case { ... }
//	tickCh ─┘
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cwd-k2/gicel"
)

// --- Channel primitives (same as event-loop-b) ---

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
		return nil, ce, fmt.Errorf("recvFrom: cancelled")
	}
}

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
		return nil, ce, fmt.Errorf("sendTo: cancelled")
	}
}

func chGet(ce gicel.CapEnv, label string) (chan gicel.Value, error) {
	raw, ok := ce.Get(label)
	if !ok {
		return nil, fmt.Errorf("channel %s: not in capabilities", label)
	}
	ch, ok := raw.(chan gicel.Value)
	if !ok {
		return nil, fmt.Errorf("channel %s: not a channel", label)
	}
	return ch, nil
}

// --- GICEL source ---

// Event is a variant: key carries a String, tick carries an Int (count).
// GICEL receives the variant via recvFrom and dispatches with case.
const mainSource = `
import Prelude

recvFrom :: \(ch: Label) a (r: Row). Effect { ch: a | r } a
recvFrom := assumption

sendTo :: \(ch: Label) a (r: Row). a -> Effect { ch: a | r } ()
sendTo := assumption

form Event := KeyPress String | Tick Int

main := fix (\loop state. do {
  event <- recvFrom @#events;
  case event {
    KeyPress k => do {
      sendTo @#results (append "key: " k);
      loop state
    };
    Tick n => do {
      sendTo @#results (append "tick #" (show n));
      loop (state + 1)
    }
  }
}) 0
`

func main() {
	eventCh := make(chan gicel.Value)
	resultCh := make(chan gicel.Value)

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()

	eng.RegisterPrim("recvFrom", recvFromImpl)
	eng.RegisterPrim("sendTo", sendToImpl)

	rt, err := eng.NewRuntime(context.Background(), mainSource)
	if err != nil {
		log.Fatal("compile: ", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := rt.RunWith(ctx, &gicel.RunOptions{
			Caps: map[string]any{
				"#events":  eventCh,
				"#results": resultCh,
			},
		})
		done <- err
	}()

	// --- Go-side: multiple event sources merged via select ---

	keyCh := make(chan string, 4)
	tickCh := make(chan int, 4)

	// Multiplexer: Go select → VariantVal → single eventCh
	go func() {
		for {
			select {
			case k := <-keyCh:
				// KeyPress constructor: ConVal{Con: "KeyPress", Args: [string]}
				eventCh <- &gicel.ConVal{Con: "KeyPress", Args: []gicel.Value{gicel.ToValue(k)}}
			case n := <-tickCh:
				// Tick constructor: ConVal{Con: "Tick", Args: [int]}
				eventCh <- &gicel.ConVal{Con: "Tick", Args: []gicel.Value{gicel.ToValue(int64(n))}}
			case <-ctx.Done():
				return
			}
		}
	}()

	// --- Simulate events ---

	recv := func() {
		select {
		case result := <-resultCh:
			fmt.Printf("  → %s\n", result)
		case err := <-done:
			log.Fatal("VM error: ", err)
		}
	}

	fmt.Println("--- select-style event dispatch ---")

	keyCh <- "a"
	recv() // key: a

	keyCh <- "b"
	recv() // key: b

	tickCh <- 1
	recv() // tick #1

	keyCh <- "c"
	recv() // key: c

	tickCh <- 2
	recv() // tick #2

	// Simulate concurrent events with timing
	go func() {
		time.Sleep(10 * time.Millisecond)
		tickCh <- 3
	}()
	keyCh <- "d"
	recv() // key: d (arrives first)
	recv() // tick #3 (arrives after 10ms)

	cancel()
	<-done
	fmt.Println("--- done ---")
}
