//go:build windows

package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/direct2d"
	direct2dcommon "github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/direct2d/common"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dxgi/common"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/storage/filesystem"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/console"
)

// TestFlatStructReturn drives flat functions returning a register-sized
// aggregate: LsnCreate builds an 8-byte CLS_LSN in RAX from three fields the
// accessor functions read back.
func TestFlatStructReturn(t *testing.T) {
	lsn := filesystem.LsnCreate(1, 512, 3) // the block offset must be sector-aligned
	if got := filesystem.LsnContainer(&lsn); got != 1 {
		t.Errorf("LsnContainer = %d, want 1", got)
	}
	if got := filesystem.LsnBlockOffset(&lsn); got != 512 {
		t.Errorf("LsnBlockOffset = %d, want 512", got)
	}
	if got := filesystem.LsnRecordSequence(&lsn); got != 3 {
		t.Errorf("LsnRecordSequence = %d, want 3", got)
	}
}

// TestFlatStructReturnWithLastError drives a SetLastError function returning
// a 4-byte COORD: (COORD, error) with the error advisory.
func TestFlatStructReturnWithLastError(t *testing.T) {
	if err := console.AllocConsole(); err != nil {
		t.Logf("AllocConsole: %v (a console may already be attached)", err)
	} else {
		defer console.FreeConsole()
	}
	handle, err := console.GetStdHandle(console.STD_OUTPUT_HANDLE)
	if err != nil {
		t.Skipf("GetStdHandle: %v", err)
	}
	size, err := console.GetLargestConsoleWindowSize(handle)
	if err != nil {
		t.Skipf("GetLargestConsoleWindowSize: %v (no console window)", err)
	}
	if size.X <= 0 || size.Y <= 0 {
		t.Errorf("largest console window = %+v, want positive dimensions", size)
	}
}

// TestComStructReturn drives COM methods returning aggregates by value
// through the hidden result pointer: an 8-byte non-float struct
// (D2D1_PIXEL_FORMAT), an 8-byte float pair (D2D_SIZE_F) and an 8-byte
// integer pair (D2D_SIZE_U) from a Direct2D DC render target.
func TestComStructReturn(t *testing.T) {
	var out *win32.IUnknown
	if err := direct2d.D2D1CreateFactory(direct2d.D2D1_FACTORY_TYPE_SINGLE_THREADED, &direct2d.IID_ID2D1Factory, nil, &out); err != nil {
		t.Skipf("D2D1CreateFactory: %v", err)
	}
	factory := win32.Cast[direct2d.ID2D1Factory](out)
	defer factory.Release()

	properties := direct2d.D2D1_RENDER_TARGET_PROPERTIES{
		PixelFormat: direct2dcommon.D2D1_PIXEL_FORMAT{
			Format:    common.DXGI_FORMAT_B8G8R8A8_UNORM,
			AlphaMode: direct2dcommon.D2D1_ALPHA_MODE_IGNORE,
		},
	}
	var target *direct2d.ID2D1DCRenderTarget
	if err := factory.CreateDCRenderTarget(&properties, &target); err != nil {
		t.Skipf("CreateDCRenderTarget: %v", err)
	}
	defer target.Release()

	format := target.GetPixelFormat()
	if format.Format != common.DXGI_FORMAT_B8G8R8A8_UNORM || format.AlphaMode != direct2dcommon.D2D1_ALPHA_MODE_IGNORE {
		t.Errorf("GetPixelFormat = %+v, want the format the target was created with", format)
	}
	// A DC target has no bound DC yet: size and pixel size are both zero,
	// but the calls must round-trip without faulting through the hidden
	// pointer. GetDpi (float out-params) confirms the object is live.
	size := target.GetSize()
	pixelSize := target.GetPixelSize()
	var dpiX, dpiY float32
	target.GetDpi(&dpiX, &dpiY)
	if dpiX <= 0 || dpiY <= 0 {
		t.Errorf("GetDpi = (%v, %v), want positive", dpiX, dpiY)
	}
	t.Logf("GetSize = %+v, GetPixelSize = %+v, dpi = (%v, %v)", size, pixelSize, dpiX, dpiY)
}
