//go:build windows

package acceptance

import (
	"testing"
	"unsafe"

	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dxgi"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
)

// TestComStreamRoundTrip drives a real COM object end-to-end through the
// generated vtable structs. The generated IStream IS the COM object (obtained
// by casting a factory out-param); its methods return Go errors, and base
// methods (Write/Read from ISequentialStream, QueryInterface from IUnknown)
// are promoted through embedding.
func TestComStreamRoundTrip(t *testing.T) {
	var stream *com.IStream
	if err := structuredstorage.CreateStreamOnHGlobal(0, true, &stream); err != nil {
		t.Fatalf("CreateStreamOnHGlobal: %v", err)
	}
	if stream == nil {
		t.Fatal("CreateStreamOnHGlobal returned nil stream without error")
	}
	defer stream.Release()

	// Write via the promoted ISequentialStream method — the void*+[MemorySize]
	// buffer collapses to []byte, size derived from len().
	payload := []byte("go-bindings-win32 COM round trip")
	var written uint32
	if err := stream.Write(payload, &written); err != nil {
		t.Fatalf("IStream.Write: %v", err)
	}
	if written != uint32(len(payload)) {
		t.Fatalf("wrote %d bytes, want %d", written, len(payload))
	}

	// Seek back to the start (STREAM_SEEK_SET = 0) — own method, returns error.
	var position uint64
	if err := stream.Seek(0, 0, &position); err != nil {
		t.Fatalf("IStream.Seek: %v", err)
	}
	if position != 0 {
		t.Fatalf("Seek position = %d, want 0", position)
	}

	// Read the payload back via the promoted method — same []byte collapse.
	readBack := make([]byte, len(payload))
	var read uint32
	if err := stream.Read(readBack, &read); err != nil {
		t.Fatalf("IStream.Read: %v", err)
	}
	if string(readBack) != string(payload) {
		t.Fatalf("round trip = %q, want %q", readBack, payload)
	}

	// QueryInterface for IUnknown promoted from IUnknown through the generated
	// IID constant — the [ComOutPtr] void** out-param is typed **win32.IUnknown,
	// usable (Release, further QueryInterface) without a cast.
	var unknown *win32.IUnknown
	if err := stream.QueryInterface(&com.IID_IUnknown, &unknown); err != nil {
		t.Fatalf("QueryInterface(IID_IUnknown): %v", err)
	}
	if unknown == nil {
		t.Fatal("QueryInterface returned nil without error")
	}

	// Release the QI reference; the object must survive (stream still holds
	// one), so the returned refcount is non-zero.
	if refs := unknown.Release(); refs == 0 {
		t.Fatal("Release after QueryInterface freed the object while a reference remained")
	}
}

// TestComOutPtrFactory drives a flat riid/ppv factory: the [ComOutPtr] void**
// out-param is typed **win32.IUnknown, then cast to the concrete interface
// the riid selected.
func TestComOutPtrFactory(t *testing.T) {
	var out *win32.IUnknown
	if err := dxgi.CreateDXGIFactory1(&dxgi.IID_IDXGIFactory1, &out); err != nil {
		t.Fatalf("CreateDXGIFactory1: %v", err)
	}
	if out == nil {
		t.Fatal("CreateDXGIFactory1 returned nil factory without error")
	}
	factory := (*dxgi.IDXGIFactory1)(unsafe.Pointer(out))
	defer factory.Release()

	// Exercise a method through the cast pointer to prove the vtable lines up.
	var adapter *dxgi.IDXGIAdapter1
	if err := factory.EnumAdapters1(0, &adapter); err != nil {
		t.Fatalf("EnumAdapters1(0): %v", err)
	}
	defer adapter.Release()
	desc, err := adapter.GetDesc1()
	if err != nil {
		t.Fatalf("GetDesc1: %v", err)
	}
	if win32.UTF16ToString(&desc.Description[0]) == "" {
		t.Error("adapter description is empty")
	}
}
