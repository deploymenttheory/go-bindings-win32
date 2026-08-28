//go:build windows

package acceptance

import (
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/security"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestPseudoTokenInlines covers the [Constant] header inlines that
// win32metadata attributes to the FORCEINLINE pseudo-module: they must be
// the SDK's pseudo-handle values, not LoadLibrary dispatches, and the
// process pseudo-token must be usable with a real token API.
func TestPseudoTokenInlines(t *testing.T) {
	if got := threading.GetCurrentProcessToken(); got != ^foundation.HANDLE(3) {
		t.Fatalf("GetCurrentProcessToken = %#x, want (HANDLE)-4", got)
	}
	if got := threading.GetCurrentThreadToken(); got != ^foundation.HANDLE(4) {
		t.Fatalf("GetCurrentThreadToken = %#x, want (HANDLE)-5", got)
	}
	if got := threading.GetCurrentThreadEffectiveToken(); got != ^foundation.HANDLE(5) {
		t.Fatalf("GetCurrentThreadEffectiveToken = %#x, want (HANDLE)-6", got)
	}

	var stats security.TOKEN_STATISTICS
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&stats)), unsafe.Sizeof(stats))
	var length uint32
	if err := security.GetTokenInformation(threading.GetCurrentProcessToken(), security.TokenStatistics, buffer, &length); err != nil {
		t.Fatalf("GetTokenInformation(process pseudo-token): %v", err)
	}
	if stats.TokenType != security.TokenPrimary {
		t.Errorf("TokenType = %v, want TokenPrimary", stats.TokenType)
	}
}
