//go:build windows

package acceptance

import (
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/direct2d"
	direct2dcommon "github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/direct2d/common"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/directwrite"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/variant"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/accessibility"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"
)

// These tests drive the generated shapes that dispatch through win32.Call:
// float parameters and returns, by-value composites larger than a register
// (or made of floats), on flat functions and COM methods alike.

// D2D1Vec3Length(float, float, float) float: three float parameters and a
// float return through the register-aware call.
func TestFloatParamsAndReturn(t *testing.T) {
	if got := direct2d.D2D1Vec3Length(3, 4, 0); got != 5 {
		t.Errorf("D2D1Vec3Length(3, 4, 0) = %v, want 5", got)
	}
}

// D2D1MakeRotateMatrix(float angle, D2D_POINT_2F center, *matrix): a float
// beside an 8-byte float pair by value (an HFA — V registers on arm64, the
// integer register on x64).
func TestHFAParamByValue(t *testing.T) {
	var matrix direct2dcommon.D2D_MATRIX_3X2_F
	direct2d.D2D1MakeRotateMatrix(90, direct2dcommon.D2D_POINT_2F{X: 0, Y: 0}, &matrix)
	// A 90° rotation about the origin: [cos sin; -sin cos] = [0 1; -1 0].
	got := *(*[6]float32)(unsafe.Pointer(&matrix))
	want := [6]float32{0, 1, -1, 0, 0, 0}
	for i := range got {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Fatalf("D2D1MakeRotateMatrix(90°) = %v, want %v", got, want)
		}
	}
}

// VariantTimeToSystemTime(double, *SYSTEMTIME): a float64 parameter;
// VariantToDoubleWithDefault(*VARIANT, double) double: float64 in and out.
func TestFloat64Params(t *testing.T) {
	var st foundation.SYSTEMTIME
	if variant.VariantTimeToSystemTime(2.5, &st) == 0 {
		t.Fatal("VariantTimeToSystemTime(2.5) failed")
	}
	// Variant day 0 is 1899-12-30, so 2.5 is 1900-01-01 12:00.
	if st.WYear != 1900 || st.WMonth != 1 || st.WDay != 1 || st.WHour != 12 {
		t.Errorf("VariantTimeToSystemTime(2.5) = %+v, want 1900-01-01 12:00", st)
	}
	var empty variant.VARIANT // VT_EMPTY: the default applies
	if got := variant.VariantToDoubleWithDefault(&empty, 1.5); got != 1.5 {
		t.Errorf("VariantToDoubleWithDefault(VT_EMPTY, 1.5) = %v, want 1.5", got)
	}
}

// shlwapi!AssocCreate takes a CLSID by value: a 16-byte composite.
func TestGUIDByValue(t *testing.T) {
	var out *win32.IUnknown
	if err := shell.AssocCreate(shell.CLSID_QueryAssociations, &shell.IID_IQueryAssociations, &out); err != nil {
		t.Fatalf("AssocCreate(CLSID by value): %v", err)
	}
	if out == nil {
		t.Fatal("AssocCreate returned no object")
	}
	out.Release()
}

// COM float parameters and return: ID2D1Factory::CreateStrokeStyle takes a
// D2D1_STROKE_STYLE_PROPERTIES by pointer whose miter limit comes back from
// ID2D1StrokeStyle::GetMiterLimit() float; IDWriteFactory::CreateTextFormat
// takes the font size as its sixth argument (a stack float on x64) and
// IDWriteTextFormat::GetFontSize() returns it.
func TestComFloatReturnAndStackFloatParam(t *testing.T) {
	var out *win32.IUnknown
	if err := direct2d.D2D1CreateFactory(direct2d.D2D1_FACTORY_TYPE_SINGLE_THREADED, &direct2d.IID_ID2D1Factory, nil, &out); err != nil {
		t.Skipf("D2D1CreateFactory: %v", err)
	}
	factory := win32.Cast[direct2d.ID2D1Factory](out)
	defer factory.Release()
	properties := direct2d.D2D1_STROKE_STYLE_PROPERTIES{MiterLimit: 12.5}
	var style *direct2d.ID2D1StrokeStyle
	if err := factory.CreateStrokeStyle(&properties, nil, &style); err != nil {
		t.Fatalf("CreateStrokeStyle: %v", err)
	}
	defer style.Release()
	if got := style.GetMiterLimit(); got != 12.5 {
		t.Errorf("GetMiterLimit = %v, want 12.5", got)
	}

	var dwrite *win32.IUnknown
	if err := directwrite.DWriteCreateFactory(directwrite.DWRITE_FACTORY_TYPE_SHARED, &directwrite.IID_IDWriteFactory, &dwrite); err != nil {
		t.Skipf("DWriteCreateFactory: %v", err)
	}
	writeFactory := win32.Cast[directwrite.IDWriteFactory](dwrite)
	defer writeFactory.Release()
	var format *directwrite.IDWriteTextFormat
	if err := writeFactory.CreateTextFormat("Segoe UI", nil, directwrite.DWRITE_FONT_WEIGHT_NORMAL,
		directwrite.DWRITE_FONT_STYLE_NORMAL, directwrite.DWRITE_FONT_STRETCH_NORMAL, 14, "en-us", &format); err != nil {
		t.Fatalf("CreateTextFormat: %v", err)
	}
	defer format.Release()
	if got := format.GetFontSize(); got != 14 {
		t.Errorf("GetFontSize = %v, want 14 (the float travelled on the stack)", got)
	}
}

// VARIANT (24 bytes) and RECT (16 bytes) by value on COM methods:
// IUIAutomation::RectToVariant / VariantToRect round-trip a rectangle.
func TestVariantAndRectByValue(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const coinitApartmentThreaded = 0x2
	if _, err := com.CoInitializeEx(coinitApartmentThreaded); err != nil {
		t.Skipf("CoInitializeEx: %v", err)
	}
	defer com.CoUninitialize()

	clsidCUIAutomation := win32.GUID{Data1: 0xff48dba4, Data2: 0x60ef, Data3: 0x4201, Data4: [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	var out *win32.IUnknown
	if err := com.CoCreateInstance(&clsidCUIAutomation, nil, com.CLSCTX_INPROC_SERVER, &accessibility.IID_IUIAutomation, &out); err != nil {
		t.Skipf("CoCreateInstance(CUIAutomation): %v", err)
	}
	automation := win32.Cast[accessibility.IUIAutomation](out)
	defer automation.Release()

	rect := foundation.RECT{Left: 1, Top: 2, Right: 30, Bottom: 40}
	packed, err := automation.RectToVariant(rect) // RECT by value in
	if err != nil {
		t.Fatalf("RectToVariant: %v", err)
	}
	back, err := automation.VariantToRect(packed) // VARIANT by value in
	if err != nil {
		t.Fatalf("VariantToRect: %v", err)
	}
	if back != rect {
		t.Errorf("RectToVariant/VariantToRect round trip = %+v, want %+v", back, rect)
	}
}
