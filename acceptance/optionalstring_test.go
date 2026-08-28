//go:build windows

package acceptance

import (
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networkmanagement/netmanagement"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestOptionalStringNULL drives [Optional] string parameters typed *string:
// nil passes NULL (an unnamed event; the local machine for NetUserEnum), and
// win32.Str passes a real string that a required-string API can then find.
func TestOptionalStringNULL(t *testing.T) {
	unnamed, err := threading.CreateEvent(nil, true, false, nil)
	if err != nil {
		t.Fatalf("CreateEvent(name=NULL): %v", err)
	}
	defer foundation.CloseHANDLE(unnamed)

	const name = "go-bindings-win32-optional-string-test"
	named, err := threading.CreateEvent(nil, true, false, win32.Str(name))
	if err != nil {
		t.Fatalf("CreateEvent(win32.Str): %v", err)
	}
	defer foundation.CloseHANDLE(named)
	const eventAllAccess = 0x1F0003
	opened, err := threading.OpenEvent(eventAllAccess, false, name) // required string: plain Go string
	if err != nil {
		t.Fatalf("OpenEvent(%q) after CreateEvent(win32.Str): %v", name, err)
	}
	defer foundation.CloseHANDLE(opened)

	// NetUserEnum's server name is [Optional]: NULL means the local machine.
	var (
		buffer      *byte
		read, total uint32
		resume      uint32
	)
	status := netmanagement.NetUserEnum(nil, 0, netmanagement.FILTER_NORMAL_ACCOUNT,
		&buffer, netmanagement.MAX_PREFERRED_LENGTH, &read, &total, &resume)
	if status != 0 {
		t.Fatalf("NetUserEnum(servername=NULL) status %d, want NERR_Success", status)
	}
	defer netmanagement.NetApiBufferFree(unsafe.Pointer(buffer))
	if read == 0 {
		t.Error("NetUserEnum on the local machine listed no accounts")
	}
}
