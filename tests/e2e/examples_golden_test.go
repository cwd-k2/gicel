// Examples golden tests — run every examples/gicel/**/*.gicel and compare
// stdout against a sidecar .golden file. The golden files capture the
// expected runtime output and are the primary regression net for the
// end-to-end pipeline (compile + execute + format).
//
// Workflow:
//
//	go test ./tests/e2e/                   # verify
//	go test ./tests/e2e/ -update           # regenerate golden files
//
// Examples that read stdin stay determinate because the test drives
// each run with /dev/null as stdin. The scripts/run-examples.sh script
// uses the same convention for the liveness check.

package e2e_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "update .golden files instead of comparing")

// examplesRoot is resolved relative to the test binary's working directory,
// which is the package directory (tests/e2e). Examples live at the repo root.
const examplesRoot = "../../examples/gicel"

// binaryPath points to the test-scope CLI binary built in TestMain.
// A fresh build per run guarantees the golden comparison reflects
// current code, not a stale binary from a previous session.
var binaryPath string

// perTestTimeout bounds any single example to prevent a runaway from
// hanging the test suite. Matches scripts/run-examples.sh.
const perTestTimeout = 20 * time.Second

func TestMain(m *testing.M) {
	flag.Parse()
	tmp, err := os.MkdirTemp("", "gicel-e2e-*")
	if err != nil {
		panic("TestMain: mkdir: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "gicel")
	cmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/gicel/")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("TestMain: build: " + err.Error())
	}
	os.Exit(m.Run())
}

// TestExamplesGolden runs each .gicel example and compares stdout to its
// .golden sidecar.
func TestExamplesGolden(t *testing.T) {
	files, err := collectExamples(examplesRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no examples discovered")
	}

	for _, path := range files {
		// Test names mirror the on-disk layout for easy filtering with -run.
		name := strings.TrimPrefix(path, examplesRoot+"/")
		name = strings.TrimSuffix(name, ".gicel")
		t.Run(name, func(t *testing.T) {
			checkExample(t, path)
		})
	}
}

func collectExamples(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".gicel") {
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

func checkExample(t *testing.T, path string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "run", path)
	// Determinate stdin — some examples (echo.gicel) read from stdin.
	cmd.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// run-examples.sh falls back to `check` when run fails (check-only
		// examples have no main). We do NOT golden check-only examples:
		// their run-time output is empty, not meaningful. Mark the example
		// as skip via a sentinel .skip file to opt out.
		if isCheckOnly(path) {
			t.Skipf("declared check-only via .skip sidecar")
		}
		t.Fatalf("run failed: %v\n--- stderr ---\n%s", err, stderr.String())
	}

	goldenPath := path + ".golden"
	got := stdout.Bytes()

	if *updateGolden {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("missing golden file %s — run `go test ./tests/e2e/ -update` to create", goldenPath)
		}
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output mismatch for %s\n--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			path, len(got), preview(got), len(want), preview(want))
	}
}

func isCheckOnly(path string) bool {
	_, err := os.Stat(path + ".skip")
	return err == nil
}

// preview truncates large outputs to keep diff messages readable.
func preview(b []byte) string {
	const max = 2048
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "\n... (truncated)"
}
