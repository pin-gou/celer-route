package modelcatalog

import (
	"testing"
	"time"
)

// TestBundledMCPLibraryLoads verifies the embedded MCP library catalog parses
// into valid entries so the fallback copy always works at runtime.
func TestBundledMCPLibraryLoads(t *testing.T) {
	entries, err := loadBundledMCPLibrary()
	if err != nil {
		t.Fatalf("loadBundledMCPLibrary: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("bundled MCP library catalog is empty")
	}
	// Assert a known well-known entry to catch truncated/stale snapshots.
	found := false
	for _, e := range entries {
		if e.Name == "Filesystem" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("bundled MCP library missing Filesystem entry (got %d entries)", len(entries))
	}
}

// TestDefaultMCPLibraryTimeoutIs5Seconds pins the user-requested download
// timeout for the MCP library sync.
func TestDefaultMCPLibraryTimeoutIs5Seconds(t *testing.T) {
	if DefaultMCPLibraryTimeout != 5*time.Second {
		t.Fatalf("DefaultMCPLibraryTimeout = %v, want 5s", DefaultMCPLibraryTimeout)
	}
}
