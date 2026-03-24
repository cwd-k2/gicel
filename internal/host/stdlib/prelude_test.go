package stdlib

import (
	"strings"
	"testing"
)

func TestCoreSourceContainsEssentials(t *testing.T) {
	for _, want := range []string{
		"form IxMonad",
		"impl IxMonad Computation",
		"type Lift",
		"type Effect",
		"seq :=",
	} {
		if !strings.Contains(CoreSource, want) {
			t.Errorf("CoreSource missing %q", want)
		}
	}
}

func TestCoreSourceDoesNotContainPrelude(t *testing.T) {
	for _, unwanted := range []string{
		"form Bool",
		"form Maybe",
		"form List",
		"form Ordering",
		"form Eq",
		"form Functor",
	} {
		if strings.Contains(CoreSource, unwanted) {
			t.Errorf("CoreSource should not contain %q", unwanted)
		}
	}
}

func TestPreludeSourceContainsStdlib(t *testing.T) {
	for _, want := range []string{
		"form Bool",
		"form Maybe",
		"form List",
		"form Ordering",
		"form Eq",
		"form Functor",
		"id :=",
		"fst :=",
		"snd :=",
	} {
		if !strings.Contains(PreludeSource, want) {
			t.Errorf("PreludeSource missing %q", want)
		}
	}
}

func TestPreludeSourceDoesNotContainCore(t *testing.T) {
	for _, unwanted := range []string{
		"form IxMonad",
		"impl IxMonad Computation",
		"type Lift",
		"type Effect",
	} {
		if strings.Contains(PreludeSource, unwanted) {
			t.Errorf("PreludeSource should not contain %q", unwanted)
		}
	}
}
