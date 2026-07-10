//go:build windows

package acceptance

import (
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	systemcom "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
)

// TestComStreamRoundTrip drives a real COM object end-to-end through the
// generated vtable wrappers: CreateStreamOnHGlobal → IStream.Write → Seek →
// Read → QueryInterface → Release.
func TestComStreamRoundTrip(t *testing.T) {
	var stream *systemcom.IStream
	hr := structuredstorage.CreateStreamOnHGlobal(0, 1, &stream)
	if !win32.Succeeded(int32(hr)) {
		t.Fatalf("CreateStreamOnHGlobal: %v", win32.HRESULTError(int32(hr)))
	}
	if stream == nil {
		t.Fatal("CreateStreamOnHGlobal returned nil stream without failure")
	}
	defer stream.Release()

	// Write through the ISequentialStream slot promoted via embedding.
	payload := []byte("go-bindings-win32 COM round trip")
	var written uint32
	hr = stream.Write(unsafe.Pointer(&payload[0]), uint32(len(payload)), &written)
	if !win32.Succeeded(int32(hr)) || written != uint32(len(payload)) {
		t.Fatalf("IStream.Write: hr=%#x written=%d", uint32(hr), written)
	}

	// Seek back to the start (STREAM_SEEK_SET = 0).
	var position uint64
	hr = stream.Seek(0, 0, &position)
	if !win32.Succeeded(int32(hr)) || position != 0 {
		t.Fatalf("IStream.Seek: hr=%#x position=%d", uint32(hr), position)
	}

	// Read the payload back.
	readBack := make([]byte, len(payload))
	var read uint32
	hr = stream.Read(unsafe.Pointer(&readBack[0]), uint32(len(readBack)), &read)
	if !win32.Succeeded(int32(hr)) || read != uint32(len(payload)) {
		t.Fatalf("IStream.Read: hr=%#x read=%d", uint32(hr), read)
	}
	if string(readBack) != string(payload) {
		t.Fatalf("round trip = %q, want %q", readBack, payload)
	}

	// QueryInterface for IUnknown through the generated IID constant.
	var unknownPtr unsafe.Pointer
	hr = stream.QueryInterface(&systemcom.IID_IUnknown, &unknownPtr)
	if !win32.Succeeded(int32(hr)) || unknownPtr == nil {
		t.Fatalf("QueryInterface(IID_IUnknown): hr=%#x", uint32(hr))
	}
	unknown := (*systemcom.IUnknown)(unknownPtr)

	// Release the QI reference; the object must survive (stream still holds
	// one), so the returned refcount is non-zero after AddRef/Release pairs.
	if refs := unknown.Release(); refs == 0 {
		t.Fatal("Release after QueryInterface freed the object while a reference remained")
	}
}
