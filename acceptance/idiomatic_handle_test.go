//go:build windows

package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/threading"
)

// TestIdiomaticHandleCloser drives the generated [RAIIFree] closer: HANDLE's
// closer CloseHANDLE forwards to CloseHandle and returns a Go error. Closing
// a live handle succeeds; closing it again fails (invalid handle), proving
// the error normalization is wired through.
func TestIdiomaticHandleCloser(t *testing.T) {
	event, err := threading.CreateEvent(nil, true, false, "")
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	// First close succeeds.
	if err := foundation.CloseHANDLE(event); err != nil {
		t.Fatalf("CloseHANDLE(valid): %v", err)
	}

	// Second close of the same handle must surface an error.
	if err := foundation.CloseHANDLE(event); err == nil {
		t.Fatal("CloseHANDLE(already closed) returned nil, want error")
	} else {
		t.Logf("double-close error surfaced: %v", err)
	}
}
