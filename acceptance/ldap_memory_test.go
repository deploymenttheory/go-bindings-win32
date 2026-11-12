//go:build windows && (amd64 || arm64)

package acceptance

import (
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networking/ldap"
)

// ldap_dn2ufnW allocates a string on the LDAP heap without contacting a server.
// Its matching deallocator must accept that exact pointer, not a Go string.
// https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_dn2ufnw
func TestLDAPMemoryFreeNativeAllocation(t *testing.T) {
	for _, name := range []string{"Example", "J\u00f6rg\u674e"} {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 100; i++ {
				pointer := ldap.Ldap_dn2ufnW("CN=" + name + ",DC=example,DC=test")
				if pointer == nil {
					t.Fatalf("iteration %d: ldap_dn2ufnW returned NULL", i)
				}
				text := win32.UTF16ToString(pointer)
				ldap.Ldap_memfreeW(pointer)
				// Only access the Go copy after freeing the original allocation.
				if !strings.Contains(text, name) {
					t.Fatalf("iteration %d: converted name %q does not contain %q", i, text, name)
				}
			}
		})
	}
}
