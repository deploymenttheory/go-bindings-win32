//go:build windows && (amd64 || arm64)

package acceptance

import (
	"syscall"
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networking/ldap"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/security/authentication/identity"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/security/cryptography"
)

var declineLDAPClientCertificate = ldap.QUERYCLIENTCERT(syscall.NewCallbackCDecl(func(*ldap.LDAP, *identity.SecPkgContext_IssuerListInfoEx, **cryptography.CERT_CONTEXT) uintptr {
	return 0
}))

// No network operation is submitted. WinLDAP's real callback option must retain
// the code pointer itself, and its presence controls the Schannel client flags.
// Passing &callback would appear to install successfully but fail this roundtrip
// and would crash when Windows later tried to invoke the data address.
func TestLDAPClientCertificateOptionNativeRoundTrip(t *testing.T) {
	connection := ldap.Ldap_initW("127.0.0.1", 51300)
	if connection == nil {
		t.Fatalf("ldap_initW failed: %d", ldap.LdapGetLastError())
	}
	defer func() {
		if status := ldap.Ldap_unbind(connection); status != 0 {
			t.Errorf("ldap_unbind: %d", status)
		}
	}()
	const requested = uint32(identity.SCH_CRED_AUTO_CRED_VALIDATION | identity.SCH_CRED_NO_DEFAULT_CREDS | identity.SCH_USE_STRONG_CRYPTO)
	for _, callback := range []ldap.QUERYCLIENTCERT{declineLDAPClientCertificate, 0, declineLDAPClientCertificate} {
		if status := ldap.Ldap_set_client_certificate(connection, callback); status != 0 {
			t.Fatalf("set client callback: %d", status)
		}
		var stored ldap.QUERYCLIENTCERT
		if status := ldap.Ldap_get_optionW(connection, int32(ldap.LDAP_OPT_CLIENT_CERTIFICATE), unsafe.Pointer(&stored)); status != 0 {
			t.Fatalf("get client callback: %d", status)
		}
		if stored != callback {
			t.Fatalf("stored callback %#x, want code address %#x", stored, callback)
		}
		flags := requested
		if status := ldap.Ldap_set_optionW(connection, int32(ldap.LDAP_OPT_SCH_FLAGS), unsafe.Pointer(&flags)); status != 0 {
			t.Fatalf("set Schannel flags: %d", status)
		}
		if status := ldap.Ldap_get_optionW(connection, int32(ldap.LDAP_OPT_SCH_FLAGS), unsafe.Pointer(&flags)); status != 0 {
			t.Fatalf("get Schannel flags: %d", status)
		}
		if callback != 0 && flags != requested {
			t.Fatalf("callback did not retain explicit client policy: %#x", flags)
		}
		if flags&uint32(identity.SCH_CRED_AUTO_CRED_VALIDATION) == 0 || flags&uint32(identity.SCH_CRED_MANUAL_CRED_VALIDATION) != 0 {
			t.Fatalf("client callback changed server-validation policy: %#x", flags)
		}
	}
}
