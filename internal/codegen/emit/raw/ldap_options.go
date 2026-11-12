package rawwin

import (
	"fmt"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// WinMD gives ldap_set_optionW's polymorphic value an ordinary void* type.
// LDAP_OPT_CLIENT_CERTIFICATE instead consumes a QUERYCLIENTCERT function
// pointer by value. A typed helper avoids converting its uintptr code address
// into a Go data pointer or passing the address of callback storage.
// https://learn.microsoft.com/windows/win32/api/winldap/nc-winldap-queryclientcert
// https://github.com/dotnet/runtime/blob/main/src/libraries/Common/src/Interop/Windows/Wldap32/Interop.Ldap.cs
func (g *Generator) buildLDAPOptionHelpers(meta *win32meta.NamespaceMeta, procs []view.ProcModel, imports typemap.ImportSet) []view.FunctionModel {
	if meta.Namespace != "Networking.Ldap" {
		return nil
	}
	var proc string
	for _, candidate := range procs {
		if candidate.ExportName == "ldap_set_optionW" {
			proc = candidate.VarName
			break
		}
	}
	if proc == "" {
		return nil
	}
	const helper = "Ldap_set_client_certificate"
	function := g.registry.FunctionOwner["ldap_set_optionW"].Function
	if function == nil || len(function.Params) != 3 {
		g.diag("helper %s: LDAP option setter signature changed", helper)
		return nil
	}
	context := typemap.Context{Namespace: meta.Namespace}
	scratch := typemap.ImportSet{}
	want := []string{"*LDAP", "int32", "unsafe.Pointer"}
	for i := range function.Params {
		param := &function.Params[i]
		if (i != 0 && param.IsOut) || param.IsReserved || g.mapper.GoType(&param.Type, context, scratch).GoType != want[i] {
			g.diag("helper %s: LDAP option setter signature changed", helper)
			return nil
		}
	}
	if g.mapper.GoType(&function.Return, context, scratch).GoType != "uint32" || function.SetLastError {
		g.diag("helper %s: LDAP option setter status contract changed", helper)
		return nil
	}
	if _, found := meta.Delegates["QUERYCLIENTCERT"]; !found || !g.claimedNames["LDAP_OPT_CLIENT_CERTIFICATE"] {
		g.diag("helper %s: missing client-certificate callback or option constant", helper)
		return nil
	}
	if !g.claimName(helper) {
		g.diag("helper %s: name already used in package %s", helper, meta.Namespace)
		return nil
	}
	imports["syscall"], imports["unsafe"] = "syscall", "unsafe"
	return []view.FunctionModel{{
		GoName: helper, ProcVar: proc,
		ParamStr: "ld *LDAP, callback QUERYCLIENTCERT", ReturnSig: "uint32",
		ReturnKind: view.RetVal, RetExpr: "uint32(r1)",
		CallExpr: fmt.Sprintf("syscall.SyscallN(%s.Addr(), uintptr(unsafe.Pointer(ld)), uintptr(LDAP_OPT_CLIENT_CERTIFICATE), uintptr(callback))", proc),
		CommentLines: []string{
			helper + " sets LDAP_OPT_CLIENT_CERTIFICATE through ldap_set_optionW.",
			"callback is passed by value; zero removes the callback. Probe Procs.Ldap_set_optionW for availability.",
			"Create QUERYCLIENTCERT callbacks with syscall.NewCallbackCDecl and retain their state for the connection lifetime.",
			"Returning false selects anonymous TLS client credentials; this does not change server certificate validation.",
			"https://learn.microsoft.com/windows/win32/api/winldap/nc-winldap-queryclientcert",
		},
	}}
}
