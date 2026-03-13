package stdlib

import (
	"strings"
	"testing"
)

func TestCoreSourceContainsEssentials(t *testing.T) {
	for _, want := range []string{
		"class IxMonad",
		"instance IxMonad Computation",
		"type Lift",
		"type Effect",
		"then :=",
	} {
		if !strings.Contains(CoreSource, want) {
			t.Errorf("CoreSource missing %q", want)
		}
	}
}

func TestCoreSourceDoesNotContainPrelude(t *testing.T) {
	for _, unwanted := range []string{
		"data Bool",
		"data Maybe",
		"data List",
		"data Ordering",
		"class Eq a",
		"Ord a {",
		"class Functor",
	} {
		if strings.Contains(CoreSource, unwanted) {
			t.Errorf("CoreSource should not contain %q", unwanted)
		}
	}
}

func TestPreludeSourceContainsStdlib(t *testing.T) {
	for _, want := range []string{
		"data Bool",
		"data Maybe",
		"data List",
		"data Ordering",
		"class Eq a",
		"Ord a {",
		"class Functor",
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
		"class IxMonad",
		"instance IxMonad Computation",
		"type Lift",
		"type Effect",
	} {
		if strings.Contains(PreludeSource, unwanted) {
			t.Errorf("PreludeSource should not contain %q", unwanted)
		}
	}
}
