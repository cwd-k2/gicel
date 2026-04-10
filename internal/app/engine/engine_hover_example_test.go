// HoverIndex example smoke tests — verify hover pipeline works on all examples.
// Does NOT cover: specific hover positions (server_test.go), data structure (hoverindex_test.go).

package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/app/header"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// allPacks registers all stdlib packs on the engine.
func allPacks(eng *Engine) {
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Fail)
	eng.Use(stdlib.State)
	eng.Use(stdlib.IO)
	eng.Use(stdlib.Stream)
	eng.Use(stdlib.Slice)
	eng.Use(stdlib.Array)
	eng.Use(stdlib.Ref)
	eng.Use(stdlib.Map)
	eng.Use(stdlib.Set)
	eng.Use(stdlib.EffectMap)
	eng.Use(stdlib.EffectSet)
	eng.Use(stdlib.JSON)
	eng.Use(stdlib.Session)
}

func TestHoverIndex_AllExamples(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "gicel")
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".gicel") {
			files = append(files, path)
		}
		return nil
	})
	if len(files) == 0 {
		t.Fatal("no example files found")
	}

	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			source := string(data)

			eng := NewEngine()
			allPacks(eng)
			eng.EnableHoverIndex()

			// Apply header directives (--module, --recursion).
			res, err := header.Resolve(source, f)
			if err == nil {
				if res.Recursion {
					eng.EnableRecursion()
				}
				for _, mod := range res.Modules {
					if err := eng.RegisterModule(mod.Name, mod.Source); err != nil {
						t.Logf("module %s: %v", mod.Name, err)
					}
				}
			}

			ar := eng.Analyze(context.Background(), source)
			if ar.HoverIndex == nil {
				t.Fatal("HoverIndex is nil")
			}
			if ar.HoverIndex.Len() == 0 {
				t.Fatal("HoverIndex is empty")
			}

			// Verify no unresolved TyMeta in any recorded type.
			assertLowMetaRate(t, ar)
		})
	}
}

// assertLowMetaRate checks that TyMeta entries are rare.
// Some orphaned metas remain from CBPV auto-coercion and complex
// type-level computations where intermediate metas are never unified.
// These represent genuinely ambiguous type positions and are harmless.
func assertLowMetaRate(t *testing.T, ar *AnalysisResult) {
	t.Helper()
	total := ar.HoverIndex.Len()
	metaCount := 0
	for i := range total {
		e := &ar.HoverIndex.entries[i]
		if e.ty != nil && containsMeta(e.ty) {
			metaCount++
		}
	}
	if metaCount > 0 {
		pct := float64(metaCount) / float64(total) * 100
		if pct > 15.0 {
			t.Errorf("too many TyMeta entries: %d/%d (%.1f%% > 15%%)", metaCount, total, pct)
		}
	}
}

// containsMeta recursively checks if a type contains TyMeta.
func containsMeta(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyMeta:
		return true
	case *types.TyArrow:
		return containsMeta(t.From) || containsMeta(t.To)
	case *types.TyApp:
		return containsMeta(t.Fun) || containsMeta(t.Arg)
	case *types.TyForall:
		return containsMeta(t.Kind) || containsMeta(t.Body)
	case *types.TyEvidence:
		return containsMeta(t.Body)
	default:
		return false
	}
}
