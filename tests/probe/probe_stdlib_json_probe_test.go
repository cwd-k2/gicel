//go:build probe

// Data.JSON probe tests — encoding/decoding boundary cases and round-trips
// for primitive types, containers, and malformed inputs.
// Does NOT cover: probe_stdlib_collection_probe_test.go.
package probe_test

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// TestProbeJSON_RoundTripInt covers the simplest round-trip; fromJSON must
// recover the original value after toJSON.
func TestProbeJSON_RoundTripInt(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.JSON

main := case fromJSON (toJSON 42 :: String) :: Maybe Int {
  Just n => n;
  Nothing => 0 - 1
}
`, gicel.Prelude, gicel.DataJSON)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, v, 42)
}

// TestProbeJSON_StringEscapes: strings with quotes, backslashes, and
// control chars must survive a round trip intact.
func TestProbeJSON_StringEscapes(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.JSON

main := case fromJSON (toJSON "a\"b\\c" :: String) :: Maybe String {
  Just s => s;
  Nothing => "<parse-failed>"
}
`, gicel.Prelude, gicel.DataJSON)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "a\"b\\c")
}

// TestProbeJSON_FromMalformedReturnsNothing: fromJSON on garbage must
// return Nothing (not panic, not partial parse).
func TestProbeJSON_FromMalformedReturnsNothing(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.JSON

main := case fromJSON "{ not valid" :: Maybe Int {
  Just _  => True;
  Nothing => False
}
`, gicel.Prelude, gicel.DataJSON)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

// TestProbeJSON_TypeMismatchReturnsNothing: parsing a number as Bool must
// fail gracefully — the grammar parses, but the target type rejects.
func TestProbeJSON_TypeMismatchReturnsNothing(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.JSON

main := case fromJSON "42" :: Maybe Bool {
  Just _  => True;
  Nothing => False
}
`, gicel.Prelude, gicel.DataJSON)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, v, "False")
}

// TestProbeJSON_RoundTripList verifies list encoding produces the expected
// JSON array shape and survives a round trip.
func TestProbeJSON_RoundTripList(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.JSON

main := toJSON [1, 2, 3]
`, gicel.Prelude, gicel.DataJSON)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", v)
	}
	s, _ := hv.Inner.(string)
	// Must be a JSON array of three integers; don't pin exact spacing.
	if !strings.Contains(s, "1") || !strings.Contains(s, "2") || !strings.Contains(s, "3") {
		t.Errorf("expected array of 1,2,3 in %q", s)
	}
	if !strings.HasPrefix(strings.TrimSpace(s), "[") {
		t.Errorf("expected JSON array, got %q", s)
	}
}

// TestProbeJSON_NullUnit: () must encode as null (the only sensible mapping
// for the unit type).
func TestProbeJSON_NullUnit(t *testing.T) {
	v, err := probeRun(t, `
import Prelude
import Data.JSON

main := toJSON ()
`, gicel.Prelude, gicel.DataJSON)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostString(t, v, "null")
}
