//go:build windows && (amd64 || arm64)

package acceptance

import (
	"io"
	"net"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networking/ldap"
)

// A loopback LDAP responder advertises paging support; no directory is used.
// A non-NULL handle proves success regardless of the previous LastError.
func TestLDAPPagingIgnoresStaleLastError(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go servePagingCapability(listener)
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	connection := ldap.Ldap_initW("127.0.0.1", uint32(port))
	if connection == nil {
		t.Fatalf("ldap_initW: %d", ldap.LdapGetLastError())
	}
	defer ldap.Ldap_unbind(connection)
	version := uint32(ldap.LDAP_VERSION3)
	if status := ldap.Ldap_set_optionW(connection, int32(ldap.LDAP_OPT_PROTOCOL_VERSION), unsafe.Pointer(&version)); status != 0 {
		t.Fatal(status)
	}
	if err := ldap.Procs.Ldap_search_init_pageW.Find(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		foundation.SetLastError(foundation.ERROR_ACCESS_DENIED)
		page, err := ldap.Ldap_search_init_pageW(connection, "", ldap.LDAP_SCOPE_BASE, "(objectClass=*)", nil, 0, nil, nil, 1, 0, nil)
		if page == 0 {
			t.Fatalf("paging initialization failed: %v (LDAP %d)", err, ldap.LdapGetLastError())
		}
		status := ldap.Ldap_search_abandon_page(connection, page)
		if status != 0 {
			t.Fatalf("paging cleanup: %d", status)
		}
		if err != nil {
			t.Fatalf("valid paging handle returned with stale error: %v", err)
		}
	}
}

// The initializer may query rootDSE before creating its block. Return only the
// paging capability requested by this test and fail closed for other messages.
func servePagingCapability(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := int(header[1])
		if length&128 != 0 {
			n := length & 127
			if n > 4 {
				return
			}
			bytes := make([]byte, n)
			if _, err := io.ReadFull(conn, bytes); err != nil {
				return
			}
			length = 0
			for _, b := range bytes {
				length = length*256 + int(b)
			}
		}
		if length > 65536 {
			return
		}
		packet := make([]byte, length)
		if _, err := io.ReadFull(conn, packet); err != nil {
			return
		}
		if len(packet) < 4 || packet[0] != 2 || int(packet[1])+2 >= len(packet) {
			return
		}
		id := packet[:int(packet[1])+2]
		success := []byte{0x0a, 1, 0, 4, 0, 4, 0}
		switch packet[len(id)] {
		case 0x60:
			_, _ = conn.Write(pagingBER(0x30, id, pagingBER(0x61, success)))
		case 0x63:
			attribute := pagingBER(0x30, pagingBER(4, []byte("supportedControl")), pagingBER(0x31, pagingBER(4, []byte("1.2.840.113556.1.4.319"))))
			entry := pagingBER(0x64, pagingBER(4, nil), pagingBER(0x30, attribute))
			_, _ = conn.Write(pagingBER(0x30, id, entry))
			_, _ = conn.Write(pagingBER(0x30, id, pagingBER(0x65, success)))
		default:
			return
		}
	}
}

func pagingBER(tag byte, values ...[]byte) []byte {
	n := 0
	for _, value := range values {
		n += len(value)
	}
	encoded := []byte{tag, byte(n)}
	if n >= 128 {
		encoded = []byte{tag, 0x82, byte(n >> 8), byte(n)}
	}
	for _, value := range values {
		encoded = append(encoded, value...)
	}
	return encoded
}

func TestLDAPPagingNullHandleIsFailure(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	connection := ldap.Ldap_initW("127.0.0.1", uint32(port))
	if connection == nil {
		t.Fatal("ldap_initW returned NULL")
	}
	defer ldap.Ldap_unbind(connection)
	page, err := ldap.Ldap_search_init_pageW(connection, "", ldap.LDAP_SCOPE_BASE, "(objectClass=*)", nil, 0, nil, nil, 1, 0, nil)
	if page != 0 {
		_ = ldap.Ldap_search_abandon_page(connection, page)
		t.Fatal("unexpected paging service on reserved closed port")
	}
	if err == nil {
		t.Fatalf("NULL handle reported success (LDAP %d)", ldap.LdapGetLastError())
	}
}
