// Agent-guide embed tests — DocTopics/Doc/DocDesc.

package gicel_test

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

func TestDocTopics_NonEmpty(t *testing.T) {
	topics := gicel.DocTopics()
	if len(topics) == 0 {
		t.Fatal("no agent-guide topics embedded")
	}
}

func TestDocTopics_SortedUnique(t *testing.T) {
	topics := gicel.DocTopics()
	seen := make(map[string]bool, len(topics))
	for i, n := range topics {
		if seen[n] {
			t.Errorf("duplicate topic %q", n)
		}
		seen[n] = true
		if i > 0 && topics[i-1] > n {
			t.Errorf("DocTopics not sorted: %q before %q", topics[i-1], n)
		}
	}
}

// TestDocTopics_ExcludesREADME guards the README skip in doc.go:26.
// README is reached by the empty/"index" topic, not by name.
func TestDocTopics_ExcludesREADME(t *testing.T) {
	for _, n := range gicel.DocTopics() {
		if strings.HasSuffix(n, "README") {
			t.Errorf("DocTopics should exclude README: got %q", n)
		}
	}
}

func TestDoc_IndexAndEmpty(t *testing.T) {
	index := gicel.Doc("index")
	if index == "" {
		t.Fatal("Doc(\"index\") returned empty")
	}
	if gicel.Doc("") != index {
		t.Fatal("Doc(\"\") should alias to index")
	}
}

func TestDoc_Missing(t *testing.T) {
	if s := gicel.Doc("nope.nothing"); s != "" {
		t.Fatalf("expected empty for unknown topic, got %d bytes", len(s))
	}
}

func TestDoc_AllTopicsResolve(t *testing.T) {
	for _, n := range gicel.DocTopics() {
		if gicel.Doc(n) == "" {
			t.Errorf("catalogued topic %q does not resolve", n)
		}
	}
}

// TestDocDesc_ExtractsHeading verifies that DocDesc returns the first
// "## " heading, stripping optional "N. " numbering prefixes.
func TestDocDesc_ExtractsHeading(t *testing.T) {
	desc := gicel.DocDesc("features.records")
	if desc == "" {
		t.Fatal("DocDesc returned empty for known topic")
	}
	// Numbering prefix "N. " must be stripped.
	if strings.HasPrefix(desc, "1. ") || strings.HasPrefix(desc, "2. ") {
		t.Errorf("DocDesc did not strip numbering: %q", desc)
	}
}

func TestDocDesc_Missing(t *testing.T) {
	if s := gicel.DocDesc("nope.nothing"); s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
}
