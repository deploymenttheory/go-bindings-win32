//go:build windows

package acceptance

import (
	"testing"

	rawfoundation "github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	rawthreading "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/threading"
)

// TestIdiomaticEventErgonomics drives the idiomatic wrappers: Go strings for
// names, Go bools for flags, and Go error returns — no manual UTF-16 or BOOL
// conversion at the call site.
func TestIdiomaticEventErgonomics(t *testing.T) {
	// CreateEvent: idiomatic — string name, bool flags, (HANDLE, error).
	event, err := threading.CreateEvent(nil, true /*manualReset*/, false /*initialState*/, "go-bindings-win32-idiomatic-event")
	if err != nil {
		t.Fatalf("idiomatic CreateEvent: %v", err)
	}
	if event == 0 {
		t.Fatal("CreateEvent returned NULL without error")
	}
	defer rawfoundation.CloseHandle(event)

	// OpenEvent: idiomatic — string name, bool inherit, typed access rights.
	const eventAllAccess = 0x1F0003
	opened, err := threading.OpenEvent(eventAllAccess, false, "go-bindings-win32-idiomatic-event")
	if err != nil {
		t.Fatalf("idiomatic OpenEvent: %v", err)
	}
	defer rawfoundation.CloseHandle(opened)

	// Signal through the created handle (raw), observe through the opened
	// idiomatic handle.
	if err := rawthreading.SetEvent(event); err != nil {
		t.Fatalf("SetEvent: %v", err)
	}
	if status, _ := rawthreading.WaitForSingleObject(opened, 0); status != 0 {
		t.Fatalf("cross-handle wait = %#x, want WAIT_OBJECT_0", status)
	}
}

// TestIdiomaticOpenFailure confirms a failed idiomatic call returns a real
// Go error and a zero handle.
func TestIdiomaticOpenFailure(t *testing.T) {
	const eventAllAccess = 0x1F0003
	handle, err := threading.OpenEvent(eventAllAccess, false, "go-bindings-win32-idiomatic-missing-853214")
	if err == nil {
		rawfoundation.CloseHandle(handle)
		t.Fatal("idiomatic OpenEvent(nonexistent) succeeded, want error")
	}
	if handle != 0 {
		t.Fatalf("failed OpenEvent returned handle %#x", handle)
	}
	t.Logf("idiomatic error surfaced: %v", err)
}
