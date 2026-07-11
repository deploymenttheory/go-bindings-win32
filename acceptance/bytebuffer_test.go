//go:build windows

package acceptance

import (
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/diagnostics/debug"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestByteBufferParam drives ReadProcessMemory, whose (lpBuffer void*, nSize)
// pair collapses into a single []byte via the [MemorySize] metadata (size
// derived from len). Correct size derivation is load-bearing: a wrong nSize
// would under- or over-read the source region.
func TestByteBufferParam(t *testing.T) {
	source := []byte("go-bindings-win32 byte buffer collapse")
	buffer := make([]byte, len(source))

	var bytesRead uintptr
	err := debug.ReadProcessMemory(
		threading.GetCurrentProcess(),
		unsafe.Pointer(&source[0]),
		buffer,
		&bytesRead,
	)
	if err != nil {
		t.Fatalf("ReadProcessMemory: %v", err)
	}
	if bytesRead != uintptr(len(source)) {
		t.Fatalf("read %d bytes, want %d (nSize must derive from len)", bytesRead, len(source))
	}
	if string(buffer) != string(source) {
		t.Fatalf("round trip = %q, want %q", buffer, source)
	}

	// A shorter destination must derive a smaller nSize and stop there.
	partial := make([]byte, 8)
	bytesRead = 0
	if err := debug.ReadProcessMemory(threading.GetCurrentProcess(), unsafe.Pointer(&source[0]), partial, &bytesRead); err != nil {
		t.Fatalf("ReadProcessMemory(partial): %v", err)
	}
	if bytesRead != uintptr(len(partial)) {
		t.Fatalf("partial read = %d bytes, want %d", bytesRead, len(partial))
	}
	if string(partial) != string(source[:len(partial)]) {
		t.Fatalf("partial round trip = %q, want %q", partial, source[:len(partial)])
	}
}
