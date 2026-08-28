//go:build windows

package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestHandleCloser drives the generated [RAIIFree] closer: HANDLE's closer
// CloseHANDLE forwards to CloseHandle and returns a Go error. Closing a live
// handle succeeds; closing it again fails (invalid handle), proving the error
// normalization is wired through.
func TestHandleCloser(t *testing.T) {
	event, err := threading.CreateEvent(nil, true, false, nil)
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

	// The zero value and INVALID_HANDLE_VALUE are never handed to CloseHandle
	// (which would report ERROR_INVALID_HANDLE): a deferred close on an error
	// path is a no-op.
	if err := foundation.CloseHANDLE(0); err != nil {
		t.Errorf("CloseHANDLE(0) = %v, want nil", err)
	}
	if err := foundation.CloseHANDLE(^foundation.HANDLE(0)); err != nil {
		t.Errorf("CloseHANDLE(INVALID_HANDLE_VALUE) = %v, want nil", err)
	}
}
