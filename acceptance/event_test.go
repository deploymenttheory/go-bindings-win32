//go:build windows

// Package acceptance exercises the generated bindings against the live
// Win32 API: real DLL dispatch, real handles, real GetLastError. The bindings
// are the single idiomatic-shaped tier under bindings/win32 — Go strings for
// names, Go bools for flags, and Go error returns, with no manual UTF-16 or
// BOOL conversion at the call site.
package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestEventRoundTrip drives the full unsignaled → signal → reset cycle of an
// unnamed manual-reset event: create, wait (timeout), SetEvent, wait (success),
// ResetEvent, wait (timeout again) — exercising the string/bool/error shape and
// the BOOL+SetLastError → error returns of SetEvent/ResetEvent.
func TestEventRoundTrip(t *testing.T) {
	// CreateEvent: string name, bool flags, (HANDLE, error).
	event, err := threading.CreateEvent(nil, true /*manualReset*/, false /*initialState*/, "")
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if event == 0 {
		t.Fatal("CreateEvent returned NULL without error")
	}
	defer foundation.CloseHandle(event)

	// Unsignaled: wait must time out (WAIT_TIMEOUT = 0x102).
	const waitTimeout = 0x102
	if status, _ := threading.WaitForSingleObject(event, 0); uint32(status) != waitTimeout {
		t.Fatalf("WaitForSingleObject(unsignaled) = %#x, want WAIT_TIMEOUT", uint32(status))
	}

	// Signal, then the wait must succeed (WAIT_OBJECT_0 = 0).
	if err := threading.SetEvent(event); err != nil {
		t.Fatalf("SetEvent: %v", err)
	}
	if status, _ := threading.WaitForSingleObject(event, 0); uint32(status) != 0 {
		t.Fatalf("WaitForSingleObject(signaled) = %#x, want WAIT_OBJECT_0", uint32(status))
	}

	// ResetEvent then re-check timeout.
	if err := threading.ResetEvent(event); err != nil {
		t.Fatalf("ResetEvent: %v", err)
	}
	if status, _ := threading.WaitForSingleObject(event, 0); uint32(status) != waitTimeout {
		t.Fatalf("WaitForSingleObject(reset) = %#x, want WAIT_TIMEOUT", uint32(status))
	}
}

// TestNamedEventCrossHandle creates a named event and opens it under a second
// handle, then signals through one and observes through the other — the string
// name flows through UTF-16 conversion at both CreateEvent and OpenEvent.
func TestNamedEventCrossHandle(t *testing.T) {
	const name = "go-bindings-win32-acceptance-event"
	event, err := threading.CreateEvent(nil, false /*manualReset*/, false /*initialState*/, name)
	if err != nil {
		t.Fatalf("CreateEvent(named): %v", err)
	}
	defer foundation.CloseHandle(event)

	// Opening the same name must yield a handle to the same object.
	const eventAllAccess = 0x1F0003
	opened, err := threading.OpenEvent(eventAllAccess, false, name)
	if err != nil {
		t.Fatalf("OpenEvent: %v", err)
	}
	defer foundation.CloseHandle(opened)

	// Signal through one handle, observe through the other.
	if err := threading.SetEvent(event); err != nil {
		t.Fatalf("SetEvent: %v", err)
	}
	if status, _ := threading.WaitForSingleObject(opened, 0); uint32(status) != 0 {
		t.Fatalf("cross-handle wait = %#x, want WAIT_OBJECT_0", uint32(status))
	}
}

// TestFailurePath confirms a failed call returns a real Go error and a zero
// handle.
func TestFailurePath(t *testing.T) {
	const eventAllAccess = 0x1F0003
	opened, err := threading.OpenEvent(eventAllAccess, false, "go-bindings-win32-does-not-exist-467913")
	if err == nil {
		foundation.CloseHandle(opened)
		t.Fatal("OpenEvent(nonexistent) succeeded, want error")
	}
	if opened != 0 {
		t.Fatalf("failed OpenEvent returned handle %#x", opened)
	}
	t.Logf("OpenEvent error surfaced correctly: %v", err)
}

// TestGetCurrentProcessInfo exercises plain-value returns (no SetLastError).
func TestGetCurrentProcessInfo(t *testing.T) {
	pid := threading.GetCurrentProcessId()
	if pid == 0 {
		t.Error("GetCurrentProcessId() = 0")
	}
	tid := threading.GetCurrentThreadId()
	if tid == 0 {
		t.Error("GetCurrentThreadId() = 0")
	}
	t.Logf("pid=%d tid=%d", pid, tid)
}
