//go:build windows

package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dxcore"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestNativeBoolReturn drives a COM method declared with the one-byte C++
// bool return (not Win32 BOOL): only AL is defined on return, so the binding
// must read the low byte. IDXCoreAdapterFactory::IsNotificationTypeSupported
// is documented to return true for AdapterListStale on every DXCore build.
func TestNativeBoolReturn(t *testing.T) {
	var out *win32.IUnknown
	if err := dxcore.DXCoreCreateAdapterFactory(&dxcore.IID_IDXCoreAdapterFactory, &out); err != nil {
		t.Skipf("DXCoreCreateAdapterFactory unavailable: %v", err)
	}
	factory := win32.Cast[dxcore.IDXCoreAdapterFactory](out)
	defer factory.Release()

	if !factory.IsNotificationTypeSupported(dxcore.AdapterListStale) {
		t.Error("IsNotificationTypeSupported(AdapterListStale) = false, want true")
	}
	if factory.IsNotificationTypeSupported(dxcore.DXCoreNotificationType(0xFFFF)) {
		t.Error("IsNotificationTypeSupported(bogus) = true, want false")
	}
}

// TestBoolOutParam drives a Win32 BOOL [out] parameter, which the binding
// exposes as *bool: the callee writes a 4-byte BOOL into a hidden local and
// the generated write-back converts it. IsWow64Process is false for a native
// amd64/arm64 process, which is the only kind this module builds.
func TestBoolOutParam(t *testing.T) {
	var wow64 bool
	if err := threading.IsWow64Process(threading.GetCurrentProcess(), &wow64); err != nil {
		t.Fatalf("IsWow64Process: %v", err)
	}
	if wow64 {
		t.Error("IsWow64Process = true for a native 64-bit process, want false")
	}

	// A nil out-param must not panic: the write-back is guarded.
	if err := threading.IsWow64Process(threading.GetCurrentProcess(), nil); err != nil {
		t.Fatalf("IsWow64Process(nil): %v", err)
	}
}
