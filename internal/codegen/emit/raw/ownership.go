package rawwin

import (
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// shapeFunctionParam preserves native pointer identity where string shaping
// would change an API's ownership or method-dependent credential semantics.
// WinMD describes ldap_memfreeW.Block as an input PWSTR but does not encode
// ownership transfer. Treating it as a string would free a fresh Go UTF-16
// buffer instead of the LDAP allocation. Keep the mapper's original pointer
// type; this exception changes marshaling, not metadata or type resolution.
// https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_memfreew
// ldap_bind_sW.cred accepts a password string for SIMPLE, but an identity
// structure for Negotiate. Preserve that nullable pointer; never UTF-16 encode
// the structure as a Go string. DN remains an ordinary optional string.
// https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_bind_sw
func (g *Generator) shapeFunctionParam(namespace, function, name string, param *win32meta.Param, resolved typemap.Resolved) (shapedParam, bool) {
	if namespace == "Networking.Ldap" {
		switch function + "." + param.Name {
		case "ldap_memfreeW.Block", "ldap_bind_sW.cred":
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
	}
	return g.shapeParam(name, param, resolved)
}
