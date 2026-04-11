// HoverRecorder-fix interaction tests — verifies that Rezonk path compression
// does not contaminate outer-scope meta solutions.
// Does NOT cover: hover content accuracy, HoverIndex data structure.

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

func TestAnalyzeHoverFix(t *testing.T) {
	allSetup := func() *Engine {
		eng := NewEngine()
		eng.Use(stdlib.Prelude)
		eng.Use(stdlib.Fail)
		eng.Use(stdlib.State)
		eng.Use(stdlib.IO)
		eng.Use(stdlib.Stream)
		eng.Use(stdlib.Slice)
		eng.Use(stdlib.Map)
		eng.Use(stdlib.Set)
		eng.Use(stdlib.Array)
		eng.Use(stdlib.Ref)
		eng.Use(stdlib.EffectMap)
		eng.Use(stdlib.EffectSet)
		eng.Use(stdlib.JSON)
		eng.Use(stdlib.Session)
		eng.Use(stdlib.Math)
		eng.Use(stdlib.Sequence)
		eng.EnableRecursion()
		return eng
	}

	// fix inside block with Set.member (extracts graph.gicel dfs pattern)
	t.Run("fix_set_member_block", func(t *testing.T) {
		src := `import Prelude
import Data.Set as Set
dfs :: List Int -> List Int
dfs := \xs. {
  go := fix (\self stack visited result. case stack {
    Nil => reverse result;
    Cons node rest =>
      if Set.member node visited then self rest visited result
      else {
        visited2 := Set.insert node visited;
        self rest visited2 (Cons node result)
      }
  });
  go xs (Set.empty :: Set Int) Nil
}
main := dfs [1, 2, 3]`
		for _, hover := range []bool{false, true} {
			label := "all"
			if hover {
				label += "+hover"
			}
			t.Run(label, func(t *testing.T) {
				eng := allSetup()
				if hover {
					eng.EnableHoverIndex()
				}
				ar := eng.Analyze(context.Background(), src)
				if ar.Errors != nil && ar.Errors.HasErrors() {
					t.Errorf("%d errors", len(ar.Errors.Errs))
					for _, e := range ar.Errors.Errs {
						t.Logf("  %s", e.Message)
					}
				}
			})
		}
	})

	// fix at top-level with Set.member
	t.Run("fix_set_member_toplevel", func(t *testing.T) {
		src := `import Prelude
import Data.Set as Set
go := fix (\self stack visited result. case stack {
  Nil => reverse result;
  Cons node rest =>
    if Set.member node visited then self rest visited result
    else {
      visited2 := Set.insert node visited;
      self rest visited2 (Cons node result)
    }
})
main := go [1, 2, 3] (Set.empty :: Set Int) Nil`
		for _, hover := range []bool{false, true} {
			label := "all"
			if hover {
				label += "+hover"
			}
			t.Run(label, func(t *testing.T) {
				eng := allSetup()
				if hover {
					eng.EnableHoverIndex()
				}
				ar := eng.Analyze(context.Background(), src)
				if ar.Errors != nil && ar.Errors.HasErrors() {
					t.Errorf("%d errors", len(ar.Errors.Errs))
					for _, e := range ar.Errors.Errs {
						t.Logf("  %s", e.Message)
					}
				}
			})
		}
	})

	// fix without class constraints (no Set) — control case
	t.Run("fix_no_class", func(t *testing.T) {
		src := `import Prelude
f :: List Int -> List Int
f := \xs. {
  go := fix (\self acc remaining. case remaining {
    Nil => reverse acc;
    Cons x rest => self (Cons (x + 1) acc) rest
  });
  go Nil xs
}
main := f [1, 2, 3]`
		for _, hover := range []bool{false, true} {
			label := "all"
			if hover {
				label += "+hover"
			}
			t.Run(label, func(t *testing.T) {
				eng := allSetup()
				if hover {
					eng.EnableHoverIndex()
				}
				ar := eng.Analyze(context.Background(), src)
				if ar.Errors != nil && ar.Errors.HasErrors() {
					t.Errorf("%d errors", len(ar.Errors.Errs))
					for _, e := range ar.Errors.Errs {
						t.Logf("  %s", e.Message)
					}
				}
			})
		}
	})
}
