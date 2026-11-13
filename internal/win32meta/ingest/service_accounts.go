package ingest

import (
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// The metadata and Learn requirements table name Netapi32, whose forwarding
// exports exist on some systems. The API remarks explicitly require dynamic
// linking to Logoncli; the ActiveDirectory module uses that DLL as well.
// Keep this correction narrow and preserve unrelated NetAPI exports.
// https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netaddserviceaccount
// https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netremoveserviceaccount
// https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netisserviceaccount
func correctServiceAccountModule(namespace string, function *win32meta.Function) {
	if namespace != "NetworkManagement.NetManagement" || !strings.EqualFold(function.DLL, "NETAPI32.dll") {
		return
	}
	switch function.Name {
	case "NetAddServiceAccount", "NetRemoveServiceAccount", "NetIsServiceAccount", "NetEnumerateServiceAccounts":
		function.DLL = "LOGONCLI.dll"
	}
}
