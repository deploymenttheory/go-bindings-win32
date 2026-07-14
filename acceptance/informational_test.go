//go:build windows

package acceptance

import (
	"runtime"
	"testing"
	"unsafe"

	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/data/xml/xmllite"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
)

// TestInformationalSuccessCoInitializeEx proves the curated
// (win32.HRESULT, error) shape preserves informational successes: a second
// CoInitializeEx on the same thread succeeds (nil error) with S_FALSE.
func TestInformationalSuccessCoInitializeEx(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const coinitApartmentThreaded = 0x2
	if _, err := com.CoInitializeEx(coinitApartmentThreaded); err != nil {
		t.Skipf("CoInitializeEx: thread already in an incompatible apartment: %v", err)
	}
	defer com.CoUninitialize()

	hr, err := com.CoInitializeEx(coinitApartmentThreaded)
	if err != nil {
		t.Fatalf("second CoInitializeEx failed: %v", err)
	}
	defer com.CoUninitialize()
	if hr != win32.S_FALSE {
		t.Fatalf("second CoInitializeEx HRESULT = 0x%08X, want S_FALSE", uint32(hr))
	}
}

// TestInformationalSuccessXmlReaderEOF drives IXmlReader.Read to the end of
// a document: every node returns S_OK, and end-of-input is S_FALSE with a
// nil error — previously indistinguishable from a successful node read.
func TestInformationalSuccessXmlReaderEOF(t *testing.T) {
	var stream *com.IStream
	if err := structuredstorage.CreateStreamOnHGlobal(0, true, &stream); err != nil {
		t.Fatalf("CreateStreamOnHGlobal: %v", err)
	}
	defer stream.Release()

	document := []byte(`<root attr="v"><child/>text</root>`)
	var written uint32
	if err := stream.Write(document, &written); err != nil {
		t.Fatalf("IStream.Write: %v", err)
	}
	var position uint64
	if err := stream.Seek(0, 0, &position); err != nil { // STREAM_SEEK_SET
		t.Fatalf("IStream.Seek: %v", err)
	}

	var out *win32.IUnknown
	if err := xmllite.CreateXmlReader(&xmllite.IID_IXmlReader, &out, nil); err != nil {
		t.Fatalf("CreateXmlReader: %v", err)
	}
	reader := (*xmllite.IXmlReader)(unsafe.Pointer(out))
	defer reader.Release()
	if err := reader.SetInput((*com.IUnknown)(unsafe.Pointer(stream))); err != nil {
		t.Fatalf("SetInput: %v", err)
	}

	nodes := 0
	var nodeType xmllite.XmlNodeType
	for {
		hr, err := reader.Read(&nodeType)
		if err != nil {
			t.Fatalf("Read after %d nodes: %v", nodes, err)
		}
		if hr == win32.S_FALSE {
			break // end of input
		}
		nodes++
		if nodes > 100 {
			t.Fatal("Read never returned S_FALSE at end of input")
		}
	}
	if nodes == 0 {
		t.Fatal("Read returned S_FALSE before any node")
	}
}
