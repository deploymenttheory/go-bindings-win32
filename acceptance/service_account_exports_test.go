//go:build windows && (amd64 || arm64)

package acceptance

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networkmanagement/netmanagement"
)

// Resolution only. NEVER invoke any account function: even synthetic names or
// failure probes could alter the host or its directory. Compile-time assignment
// below checks the nullable ABI without executing the companion.
func TestManagedServiceAccountExportResolutionOnly(t *testing.T) {
	var _ func(foundation.PWSTR, foundation.PWSTR, foundation.PWSTR, uint32) foundation.NTSTATUS = netmanagement.NetAddServiceAccountUTF16
	for _, proc := range []*win32.Proc{netmanagement.Procs.NetAddServiceAccount, netmanagement.Procs.NetRemoveServiceAccount, netmanagement.Procs.NetIsServiceAccount, netmanagement.Procs.NetQueryServiceAccount, netmanagement.Procs.NetEnumerateServiceAccounts} {
		if err := proc.Find(); err != nil {
			t.Fatal(err)
		}
	}
}
