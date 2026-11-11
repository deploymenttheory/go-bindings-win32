//go:build windows

package acceptance

import (
	"runtime"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/memory"
)

// LocalFree and GlobalFree return NULL on success, including a NULL input.
// Their HLOCAL/HGLOBAL return types must not use allocator failure sentinels.
func TestMemoryFreeSuccess(t *testing.T) {
	for _, tc := range []struct {
		name     string
		allocate func() (uintptr, error)
		free     func(uintptr) (uintptr, error)
	}{
		{"LocalFree", func() (uintptr, error) { h, err := memory.LocalAlloc(0, 32); return uintptr(h), err }, func(h uintptr) (uintptr, error) {
			remaining, err := foundation.LocalFree(foundation.HLOCAL(h))
			return uintptr(remaining), err
		}},
		{"GlobalFree", func() (uintptr, error) { h, err := memory.GlobalAlloc(0, 32); return uintptr(h), err }, func(h uintptr) (uintptr, error) {
			remaining, err := foundation.GlobalFree(foundation.HGLOBAL(h))
			return uintptr(remaining), err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, allocate := range []bool{false, true} {
				name := "null"
				if allocate {
					name = "allocated"
				}
				t.Run(name, func(t *testing.T) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					var handle uintptr
					if allocate {
						var err error
						handle, err = tc.allocate()
						if err != nil || handle == 0 {
							t.Fatalf("allocate: handle=%#x err=%v", handle, err)
						}
					}
					// Seed LastError: successful deallocation must ignore any stale value.
					foundation.SetLastError(foundation.ERROR_ACCESS_DENIED)
					remaining, err := tc.free(handle)
					if remaining != 0 {
						t.Fatalf("free returned unfreed handle %#x: %v", remaining, err)
					}
					// Do not free again: NULL proves the first call already released it,
					// even when a broken binding reports a spurious Go error.
					if err != nil {
						t.Fatalf("successful free returned error: %v", err)
					}
				})
			}
		})
	}
}
