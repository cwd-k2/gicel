package main

import "testing"

func TestByteSizeFlag(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1024", 1024},
		{"0", 0},
		{"1KiB", 1024},
		{"1MiB", 1 << 20},
		{"1GiB", 1 << 30},
		{"100MiB", 100 << 20},
		{"2GiB", 2 << 30},
		{"1KB", 1000},
		{"1MB", 1_000_000},
		{"1GB", 1_000_000_000},
		{" 100 MiB ", 100 << 20},
		{"50 GiB", 50 << 30},
	}
	for _, tt := range tests {
		var f byteSizeFlag
		if err := f.Set(tt.input); err != nil {
			t.Errorf("Set(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if f.value != tt.want {
			t.Errorf("Set(%q) = %d, want %d", tt.input, f.value, tt.want)
		}
	}
}

func TestByteSizeFlagErrors(t *testing.T) {
	bad := []string{"abc", "MiB", "1.5MiB", "1TiB", "-1MiB"}
	for _, input := range bad {
		var f byteSizeFlag
		if err := f.Set(input); err == nil {
			t.Errorf("Set(%q): expected error, got value %d", input, f.value)
		}
	}
}

func TestByteSizeFlagString(t *testing.T) {
	f := byteSizeFlag{value: 104857600}
	if s := f.String(); s != "104857600" {
		t.Errorf("String() = %q, want \"104857600\"", s)
	}
}

// TestPackCompleteness verifies that packMap and allPackOrder are in sync.
// Every key in packMap must appear in allPackOrder and vice versa.
func TestPackCompleteness(t *testing.T) {
	if len(packMap) != len(allPackOrder) {
		t.Fatalf("packMap has %d entries but allPackOrder has %d", len(packMap), len(allPackOrder))
	}
	for _, name := range allPackOrder {
		if _, ok := packMap[name]; !ok {
			t.Errorf("allPackOrder contains %q but packMap does not", name)
		}
	}
	for name := range packMap {
		found := false
		for _, n := range allPackOrder {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("packMap contains %q but allPackOrder does not", name)
		}
	}
}
