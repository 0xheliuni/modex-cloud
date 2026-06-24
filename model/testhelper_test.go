package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// openInMemory points the package-global DB at a fresh in-memory SQLite database
// via the shared InitForTest helper.
func openInMemory(t *testing.T) {
	t.Helper()
	cleanup, err := InitForTest()
	if err != nil {
		t.Fatalf("InitForTest: %v", err)
	}
	t.Cleanup(cleanup)
}

// marshalJSON serializes v to a string for leak assertions.
func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// containsAny reports whether haystack contains any non-empty needle.
func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
