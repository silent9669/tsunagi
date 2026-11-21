//go:build !e2e

package keychain

import "testing"

// The default build must keep shelling out to the real `security` tool.
func TestDefaultBackendIsSecurity(t *testing.T) {
	if impl != "security" {
		t.Fatalf("default build backend = %q, want security", impl)
	}
}
