//go:build windows

package acceptance

import (
	"testing"
	"unsafe"

	rawcom "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/com"
)

// TestIdiomaticComStream drives a COM object through the idiomatic wrappers:
// methods return Go errors instead of HRESULT, and base-interface methods
// (Write/Read from ISequentialStream, QueryInterface from IUnknown) are
// promoted through embedding.
func TestIdiomaticComStream(t *testing.T) {
	var rawStream *rawcom.IStream
	if hr := structuredstorage.CreateStreamOnHGlobal(0, 1, &rawStream); hr != 0 {
		t.Fatalf("CreateStreamOnHGlobal: hr=%#x", uint32(hr))
	}
	defer rawStream.Release()

	stream := com.WrapIStream(rawStream)

	// Write via the promoted ISequentialStream method — returns error.
	payload := []byte("idiomatic COM round trip")
	var written uint32
	if err := stream.Write(unsafe.Pointer(&payload[0]), uint32(len(payload)), &written); err != nil {
		t.Fatalf("idiomatic Write: %v", err)
	}
	if written != uint32(len(payload)) {
		t.Fatalf("wrote %d bytes, want %d", written, len(payload))
	}

	// Seek (own method) — returns error.
	var pos uint64
	if err := stream.Seek(0, 0, &pos); err != nil {
		t.Fatalf("idiomatic Seek: %v", err)
	}

	// Read back via the promoted method.
	readBack := make([]byte, len(payload))
	var read uint32
	if err := stream.Read(unsafe.Pointer(&readBack[0]), uint32(len(readBack)), &read); err != nil {
		t.Fatalf("idiomatic Read: %v", err)
	}
	if string(readBack) != string(payload) {
		t.Fatalf("round trip = %q, want %q", readBack, payload)
	}

	// QueryInterface promoted from IUnknown — returns error.
	var unknownPtr unsafe.Pointer
	if err := stream.QueryInterface(&rawcom.IID_IUnknown, &unknownPtr); err != nil {
		t.Fatalf("idiomatic QueryInterface: %v", err)
	}
	if unknownPtr == nil {
		t.Fatal("QueryInterface returned nil without error")
	}
	(*rawcom.IUnknown)(unknownPtr).Release()
}
