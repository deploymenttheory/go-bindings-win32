package rawwin

import (
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// shapeFunctionParam preserves allocations consumed by native deallocators.
// WinMD describes ldap_memfreeW.Block as an input PWSTR but does not encode
// ownership transfer. Treating it as a string would free a fresh Go UTF-16
// buffer instead of the LDAP allocation. Keep the mapper's original pointer
// type; this exception changes marshaling, not metadata or type resolution.
// https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_memfreew
func (g *Generator) shapeFunctionParam(namespace, function, name string, param *win32meta.Param, resolved typemap.Resolved) (shapedParam, bool) {
	if namespace == "Networking.Ldap" && function == "ldap_memfreeW" && param.Name == "Block" {
		if !isWideStringPtr(resolved) || param.IsOut || param.IsReserved {
			return shapedParam{}, false
		}
		return shapedParam{
			decl:    name + " " + resolved.GoType,
			word:    "uintptr(unsafe.Pointer(" + name + "))",
			spec:    specWord,
			pointer: true,
		}, true
	}
	return g.shapeParam(name, param, resolved)
}
