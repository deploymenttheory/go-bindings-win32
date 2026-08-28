//go:build windows

package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dxcore"
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
