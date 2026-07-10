//go:build windows

package acceptance

import (
	"testing"

	rawfoundation "github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/threading"
)

// TestIdiomaticSliceParam drives WaitForMultipleObjects, whose idiomatic
// wrapper collapses the (lpHandles, nCount) pair into a single []HANDLE
// (count derived from len) and turns bWaitAll into a Go bool. Correct count
// derivation is load-bearing: a wrong nCount would read past the slice or
// wait on the wrong set.
func TestIdiomaticSliceParam(t *testing.T) {
	// Two manual-reset events; only the second is initially signaled.
	unsignaled, err := threading.CreateEvent(nil, true, false, "")
	if err != nil {
		t.Fatalf("CreateEvent(unsignaled): %v", err)
	}
	defer rawfoundation.CloseHandle(unsignaled)

	signaled, err := threading.CreateEvent(nil, true, true, "")
	if err != nil {
		t.Fatalf("CreateEvent(signaled): %v", err)
	}
	defer rawfoundation.CloseHandle(signaled)

	handles := []foundation.HANDLE{unsignaled, signaled}

	// bWaitAll = false → return as soon as ANY object is signaled. The
	// signaled one is at index 1, so the result is WAIT_OBJECT_0 + 1.
	const waitFailed = 0xFFFFFFFF
	event, err := threading.WaitForMultipleObjects(handles, false, 1000)
	if uint32(event) == waitFailed {
		t.Fatalf("WaitForMultipleObjects failed: %v", err)
	}
	if uint32(event) != 1 {
		t.Fatalf("event = %d, want WAIT_OBJECT_0+1 (the signaled handle at index 1)", uint32(event))
	}

	// Sanity: a single-element slice must derive count=1 and wait only on
	// the unsignaled handle, timing out (WAIT_TIMEOUT = 0x102).
	const waitTimeout = 0x102
	event, _ = threading.WaitForMultipleObjects(handles[:1], false, 0)
	if uint32(event) != waitTimeout {
		t.Fatalf("single-handle wait = %#x, want WAIT_TIMEOUT (count must be 1)", uint32(event))
	}
}
