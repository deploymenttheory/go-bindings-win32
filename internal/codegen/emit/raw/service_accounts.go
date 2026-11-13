package rawwin

import (
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Preserve the existing string wrapper while exposing the native NULL password
// contract and permitting a caller to wipe explicit RODC sMSA passwords. The
// documented Install-ADServiceAccount password form uses flag2; ordinary
// installation uses a NULL password and flag1. No account workflow lives here.
func (g *Generator) buildServiceAccountHelpers(meta *win32meta.NamespaceMeta, procs []view.ProcModel, imports typemap.ImportSet) []view.FunctionModel {
	if meta.Namespace != "NetworkManagement.NetManagement" {
		return nil
	}
	var proc string
	for _, candidate := range procs {
		if candidate.ExportName == "NetAddServiceAccount" {
			proc = candidate.VarName
			break
		}
	}
	if proc == "" {
		return nil
	}
	const helper = "NetAddServiceAccountUTF16"
	function := g.registry.FunctionOwner["NetAddServiceAccount"].Function
	if function == nil || len(function.Params) != 4 || !strings.EqualFold(function.DLL, "LOGONCLI.dll") {
		g.diag("helper %s: service account signature changed", helper)
		return nil
	}
	context := typemap.Context{Namespace: meta.Namespace}
	scratch := typemap.ImportSet{}
	want := []string{"foundation.PWSTR", "foundation.PWSTR", "foundation.PWSTR", "uint32"}
	names := []string{"ServerName", "AccountName", "Password", "Flags"}
	for i := range function.Params {
		p := &function.Params[i]
		if p.Name != names[i] || p.IsReserved || p.IsOut || !p.IsIn || g.mapper.GoType(&p.Type, context, scratch).GoType != want[i] || (i == 0 && !p.IsOptional) || (i == 1 && p.IsOptional) {
			g.diag("helper %s: service account parameter contract changed", helper)
			return nil
		}
	}
	if g.mapper.GoType(&function.Return, context, scratch).GoType != "foundation.NTSTATUS" || function.SetLastError {
		g.diag("helper %s: service account status contract changed", helper)
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
	return []view.FunctionModel{{GoName: helper, ProcVar: proc, ParamStr: "ServerName foundation.PWSTR, AccountName foundation.PWSTR, Password foundation.PWSTR, Flags uint32", ReturnSig: "foundation.NTSTATUS", ReturnKind: view.RetVal, RetExpr: "foundation.NTSTATUS(r1)", CallExpr: fmt.Sprintf("syscall.SyscallN(%s.Addr(), uintptr(unsafe.Pointer(ServerName)), uintptr(unsafe.Pointer(AccountName)), uintptr(unsafe.Pointer(Password)), uintptr(Flags))", proc), CommentLines: []string{
		helper + " calls LOGONCLI!NetAddServiceAccount with caller-owned UTF-16 buffers.",
		"ServerName must be NULL: this API acts on the executing Windows host.",
		"AccountName is required and NUL-terminated. Password may be NULL; this wrapper creates no hidden string copies.",
		"Keep buffers alive through the synchronous call and clear sensitive buffers afterward. Inputs remain caller-owned.",
		"The return value is the native NTSTATUS, not a Win32 last-error code. Probe Procs.NetAddServiceAccount before use.",
		"Use documented account flags deliberately: the native default can create an sMSA directory object.",
		"https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netaddserviceaccount",
	}}}
}
