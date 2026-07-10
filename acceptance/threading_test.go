//go:build windows

// Package acceptance exercises the generated bindings against the live
// Win32 API: real DLL dispatch, real handles, real GetLastError.
package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

func TestEventRoundTrip(t *testing.T) {
	// CreateEventW: manual-reset, initially unsignaled, unnamed.
	event, err := threading.CreateEventW(nil, 1, 0, nil)
	if err != nil {
		t.Fatalf("CreateEventW: %v", err)
	}
	if event == 0 {
		t.Fatal("CreateEventW returned NULL without error")
	}
	defer foundation.CloseHandle(event)

	// Unsignaled: wait must time out (WAIT_TIMEOUT = 0x102).
	const waitTimeout = 0x102
	status, _ := threading.WaitForSingleObject(event, 0)
	if status != waitTimeout {
		t.Fatalf("WaitForSingleObject(unsignaled) = %#x, want WAIT_TIMEOUT", status)
	}

	// Signal, then the wait must succeed (WAIT_OBJECT_0 = 0).
	if err := threading.SetEvent(event); err != nil {
		t.Fatalf("SetEvent: %v", err)
	}
	status, _ = threading.WaitForSingleObject(event, 0)
	if status != 0 {
		t.Fatalf("WaitForSingleObject(signaled) = %#x, want WAIT_OBJECT_0", status)
	}

	// ResetEvent then re-check timeout — exercises the BOOL+error shape.
	if err := threading.ResetEvent(event); err != nil {
		t.Fatalf("ResetEvent: %v", err)
	}
	status, _ = threading.WaitForSingleObject(event, 0)
	if status != waitTimeout {
		t.Fatalf("WaitForSingleObject(reset) = %#x, want WAIT_TIMEOUT", status)
	}
}

func TestNamedEventAndStrings(t *testing.T) {
	name := win32.UTF16Ptr("go-bindings-win32-acceptance-event")
	event, err := threading.CreateEventW(nil, 0, 0, foundation.PWSTR(name))
	if err != nil {
		t.Fatalf("CreateEventW(named): %v", err)
	}
	defer foundation.CloseHandle(event)

	// Opening the same name must yield a handle to the same object.
	const eventAllAccess = 0x1F0003
	opened, err := threading.OpenEventW(eventAllAccess, 0, foundation.PWSTR(name))
	if err != nil {
		t.Fatalf("OpenEventW: %v", err)
	}
	defer foundation.CloseHandle(opened)

	// Signal through one handle, observe through the other.
	if err := threading.SetEvent(event); err != nil {
		t.Fatalf("SetEvent: %v", err)
	}
	if status, _ := threading.WaitForSingleObject(opened, 0); status != 0 {
		t.Fatalf("cross-handle wait = %#x, want WAIT_OBJECT_0", status)
	}
}

func TestFailurePath(t *testing.T) {
	// Opening a nonexistent named event must fail with a real error.
	name := win32.UTF16Ptr("go-bindings-win32-does-not-exist-467913")
	const eventAllAccess = 0x1F0003
	opened, err := threading.OpenEventW(eventAllAccess, 0, foundation.PWSTR(name))
	if err == nil {
		foundation.CloseHandle(opened)
		t.Fatal("OpenEventW(nonexistent) succeeded, want error")
	}
	if opened != 0 {
		t.Fatalf("failed OpenEventW returned handle %#x", opened)
	}
	t.Logf("OpenEventW error surfaced correctly: %v", err)
}

func TestGetCurrentProcessInfo(t *testing.T) {
	// Plain-value returns (no SetLastError).
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
