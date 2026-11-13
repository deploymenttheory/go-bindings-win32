package ingest

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func TestManagedServiceAccountModules(t *testing.T) {
	namespaces, _ := ingestAll(t)
	meta := namespaceByName(t, namespaces, "NetworkManagement.NetManagement")
	want := map[string]bool{"NetAddServiceAccount": false, "NetRemoveServiceAccount": false, "NetIsServiceAccount": false, "NetEnumerateServiceAccounts": false}
	for _, fn := range meta.Functions {
		if _, ok := want[fn.Name]; ok {
			if fn.DLL != "LOGONCLI.dll" {
				t.Fatal(fn.Name, fn.DLL)
			}
			want[fn.Name] = true
		}
		if (fn.Name == "NetUserAdd" || fn.Name == "NetQueryServiceAccount") && fn.DLL != "NETAPI32.dll" {
			t.Fatal("unrelated export moved", fn.DLL)
		}
	}
	for name, found := range want {
		if !found {
			t.Fatal("missing", name)
		}
	}
}
func TestManagedServiceAccountCorrectionScope(t *testing.T) {
	for _, tc := range []struct{ ns, name, dll, want string }{{"Other", "NetAddServiceAccount", "NETAPI32.dll", "NETAPI32.dll"}, {"NetworkManagement.NetManagement", "Other", "NETAPI32.dll", "NETAPI32.dll"}, {"NetworkManagement.NetManagement", "NetAddServiceAccount", "other.dll", "other.dll"}, {"NetworkManagement.NetManagement", "NetAddServiceAccount", "Netapi32.DLL", "LOGONCLI.dll"}} {
		fn := win32meta.Function{Name: tc.name, DLL: tc.dll}
		correctServiceAccountModule(tc.ns, &fn)
		if fn.DLL != tc.want {
			t.Fatal(tc, fn.DLL)
		}
	}
}
