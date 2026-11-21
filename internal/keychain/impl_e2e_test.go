//go:build e2e

package keychain

import "testing"

// The e2e build must use the file store, never the real keychain.
func TestE2EBackendIsFile(t *testing.T) {
	if impl != "file" {
		t.Fatalf("e2e build backend = %q, want file", impl)
	}
}
