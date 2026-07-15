package ulid

import (
	"strings"
	"testing"
	"time"
)

func TestFromEntropyIsDeterministicAndCanonical(t *testing.T) {
	var zero [10]byte
	if got := FromEntropy(time.UnixMilli(0), zero); got != strings.Repeat("0", 26) {
		t.Fatalf("zero ULID = %q", got)
	}

	entropy := [10]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	when := time.UnixMilli(1_700_000_000_123)
	first := FromEntropy(when, entropy)
	second := FromEntropy(when, entropy)
	if first != second || len(first) != 26 {
		t.Fatalf("deterministic ULID first=%q second=%q", first, second)
	}
	entropy[9]++
	if changed := FromEntropy(when, entropy); changed == first {
		t.Fatalf("entropy change retained ULID %q", changed)
	}
	for _, char := range first {
		if !strings.ContainsRune(crockford, char) {
			t.Fatalf("ULID %q contains non-Crockford character %q", first, char)
		}
	}
}
