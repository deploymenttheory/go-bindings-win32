//go:build windows && (amd64 || arm64)

package acceptance

import (
	"runtime"
	"slices"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networking/activedirectory"
)

// This host-only test creates and frees opaque credential storage. Synthetic
// Unicode strings are never authenticated; no bind, server or directory call
// occurs. Native storage is not inspected or treated as a guessed struct ABI.
func TestDsPasswordUTF16NativeMakeWipeFree(t *testing.T) {
	for _, domainPresent := range []bool{false, true} {
		for iteration := 0; iteration < 100; iteration++ {
			username := "synthetic-üser-\U0001f512"
			if !domainPresent {
				username += "@synthetic.invalid"
			}
			user := append(utf16.Encode([]rune(username)), 0)
			domain := append(utf16.Encode([]rune("synthetic-dömain")), 0)
			password := append(utf16.Encode([]rune("synthetic-密碼-\U0001f680")), 0)
			wantUser, wantDomain, wantPassword := slices.Clone(user), slices.Clone(domain), slices.Clone(password)
			var domainPointer *uint16
			if domainPresent {
				domainPointer = &domain[0]
			}
			var owner unsafe.Pointer
			var pin runtime.Pinner
			pin.Pin(&user[0])
			pin.Pin(&domain[0])
			pin.Pin(&password[0])
			pin.Pin(&owner)
			status := activedirectory.DsMakePasswordCredentialsUTF16(&user[0], domainPointer, &password[0], &owner)
			unchanged := slices.Equal(user, wantUser) && slices.Equal(domain, wantDomain) && slices.Equal(password, wantPassword)
			separate := owner != unsafe.Pointer(&user[0]) && owner != unsafe.Pointer(&domain[0]) && owner != unsafe.Pointer(&password[0])
			clear(user)
			clear(domain)
			clear(password)
			clear(wantUser)
			clear(wantDomain)
			clear(wantPassword)
			allocated := owner != nil
			if owner != nil {
				activedirectory.DsFreePasswordCredentials(owner)
				owner = nil
			}
			wiped := true
			for _, buffer := range [][]uint16{user, domain, password} {
				for _, unit := range buffer {
					if unit != 0 {
						wiped = false
					}
				}
			}
			runtime.KeepAlive(user)
			runtime.KeepAlive(domain)
			runtime.KeepAlive(password)
			pin.Unpin()
			if status != 0 || !allocated || !separate || !unchanged || !wiped {
				t.Fatalf("native credential make/wipe/free: status=%d allocated=%t separate=%t inputs_unchanged=%t inputs_wiped=%t", status, allocated, separate, unchanged, wiped)
			}
		}
	}
}

func TestDsPasswordUTF16NULLSelectsDefaultCredentials(t *testing.T) {
	var owner unsafe.Pointer
	status := activedirectory.DsMakePasswordCredentialsUTF16(nil, nil, nil, &owner)
	if owner != nil {
		activedirectory.DsFreePasswordCredentials(owner)
	}
	if status != 0 || owner != nil {
		t.Fatalf("documented default credential: status=%d nonnull=%t", status, owner != nil)
	}
}
