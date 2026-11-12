//go:build windows && (amd64 || arm64)

package acceptance

import (
	"os"
	"testing"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/networking/activedirectory"
)

// Both workgroup and domain-joined computers support this local query. Each
// call copies its observations before freeing the returned Netapi32 allocation.
func TestDsRoleLocalComputer(t *testing.T) {
	name, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	want := readPrimaryDomainRole(t, win32.Str(name))
	for i := 0; i < 100; i++ {
		if got := readPrimaryDomainRole(t, nil); got != want {
			t.Fatalf("NULL and explicit local name returned different roles on iteration %d", i)
		}
	}
}

type primaryDomainRole struct {
	role              activedirectory.DSROLE_MACHINE_ROLE
	flags             uint32
	flat, dns, forest string
	guid              win32.GUID
}

func readPrimaryDomainRole(t *testing.T, server *string) primaryDomainRole {
	t.Helper()
	var buffer *byte
	status := activedirectory.DsRoleGetPrimaryDomainInformation(server, activedirectory.DsRolePrimaryDomainInfoBasic, &buffer)
	if buffer != nil {
		defer activedirectory.DsRoleFreeMemory(unsafe.Pointer(buffer))
	}
	if status != 0 {
		t.Fatalf("DsRoleGetPrimaryDomainInformation: status %d", status)
	}
	if buffer == nil {
		t.Fatal("DsRoleGetPrimaryDomainInformation succeeded without a buffer")
	}
	info := (*activedirectory.DSROLE_PRIMARY_DOMAIN_INFO_BASIC)(unsafe.Pointer(buffer))
	if info.MachineRole < activedirectory.DsRole_RoleStandaloneWorkstation || info.MachineRole > activedirectory.DsRole_RolePrimaryDomainController {
		t.Fatalf("invalid machine role %d", info.MachineRole)
	}
	return primaryDomainRole{
		role: info.MachineRole, flags: info.Flags, guid: info.DomainGuid,
		flat: win32.UTF16ToString(info.DomainNameFlat),
		dns:  win32.UTF16ToString(info.DomainNameDns), forest: win32.UTF16ToString(info.DomainForestName),
	}
}

func TestDsRoleInvalidInformationLevel(t *testing.T) {
	var buffer *byte
	status := activedirectory.DsRoleGetPrimaryDomainInformation(nil, 0, &buffer)
	if buffer != nil {
		defer activedirectory.DsRoleFreeMemory(unsafe.Pointer(buffer))
	}
	if status == 0 {
		t.Fatal("invalid information level unexpectedly succeeded")
	}
}
