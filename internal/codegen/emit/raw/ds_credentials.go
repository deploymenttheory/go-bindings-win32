package rawwin

import (
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// The ordinary string projection is convenient but creates an immutable Go
// password and an inaccessible UTF-16 copy. This additive companion lets a
// caller control input-buffer lifetime without changing the existing API or
// substituting a guessed representation for the returned opaque credential.
// https://learn.microsoft.com/windows/win32/api/ntdsapi/nf-ntdsapi-dsmakepasswordcredentialsw
func (g *Generator) buildDSCredentialHelpers(meta *win32meta.NamespaceMeta, procs []view.ProcModel, imports typemap.ImportSet) []view.FunctionModel {
	if meta.Namespace != "Networking.ActiveDirectory" {
		return nil
	}
	var proc string
	for _, candidate := range procs {
		if candidate.ExportName == "DsMakePasswordCredentialsW" {
			proc = candidate.VarName
			break
		}
	}
	if proc == "" {
		return nil
	}
	const helper = "DsMakePasswordCredentialsUTF16"
	function := g.registry.FunctionOwner["DsMakePasswordCredentialsW"].Function
	if function == nil || len(function.Params) != 4 || !strings.EqualFold(function.DLL, "NTDSAPI.dll") {
		g.diag("helper %s: credential maker signature changed", helper)
		return nil
	}
	context := typemap.Context{Namespace: meta.Namespace}
	scratch := typemap.ImportSet{}
	want := []string{"foundation.PWSTR", "foundation.PWSTR", "foundation.PWSTR", "*unsafe.Pointer"}
	names := []string{"User", "Domain", "Password", "pAuthIdentity"}
	for i := range function.Params {
		param := &function.Params[i]
		if param.Name != names[i] || param.IsReserved || param.IsOut != (i == 3) || param.IsIn != (i < 3) || param.IsOptional != (i < 3) || g.mapper.GoType(&param.Type, context, scratch).GoType != want[i] {
			g.diag("helper %s: credential maker parameter contract changed", helper)
			return nil
		}
	}
	if g.mapper.GoType(&function.Return, context, scratch).GoType != "uint32" || function.SetLastError {
		g.diag("helper %s: credential maker status contract changed", helper)
		return nil
	}
	if !g.claimName(helper) {
		g.diag("helper %s: name already used in package %s", helper, meta.Namespace)
		return nil
	}
	for alias, path := range scratch {
		imports[alias] = path
	}
	imports["syscall"], imports["unsafe"] = "syscall", "unsafe"
	return []view.FunctionModel{{
		GoName: helper, ProcVar: proc,
		ParamStr:  "User foundation.PWSTR, Domain foundation.PWSTR, Password foundation.PWSTR, pAuthIdentity *unsafe.Pointer",
		ReturnSig: "uint32", ReturnKind: view.RetVal, RetExpr: "uint32(r1)",
		CallExpr: fmt.Sprintf("syscall.SyscallN(%s.Addr(), uintptr(unsafe.Pointer(User)), uintptr(unsafe.Pointer(Domain)), uintptr(unsafe.Pointer(Password)), uintptr(unsafe.Pointer(pAuthIdentity)))", proc),
		CommentLines: []string{
			helper + " calls DsMakePasswordCredentialsW with caller-owned UTF-16 input buffers.",
			"Inputs are nullable NUL-terminated buffers; no Go string or hidden UTF-16 copy is created by this wrapper.",
			"All three NULL inputs select default credentials. Otherwise User is required; NULL Domain requires a fully qualified User.",
			"Keep inputs alive through the call, then clear sensitive buffers. The native credential has its own allocation.",
			"Retain the returned credential until every binding using it is unbound, then call DsFreePasswordCredentials.",
			"The returned uint32 is the native status (zero is success). Probe Procs.DsMakePasswordCredentials for availability.",
			"https://learn.microsoft.com/windows/win32/api/ntdsapi/nf-ntdsapi-dsmakepasswordcredentialsw",
		},
	}}
}
