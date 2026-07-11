package rawwin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// buildHandleClosers emits a uniform Close<Handle>(h) error helper for each
// [RAIIFree] handle typedef whose closer can be called cleanly. Returns the
// rendered file body (empty if none).
//
// A closer is emitted only when the free function is unambiguous, takes
// exactly the handle, and has a normalizable return (HRESULT / BOOL / void).
// Everything else is skipped with a diagnostic — the close function is always
// still callable directly.
func (g *Generator) buildHandleClosers(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) string {
	names := make([]string, 0, len(meta.Typedefs))
	for name := range meta.Typedefs {
		if meta.Typedefs[name].FreeFunc != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var body strings.Builder
	for _, name := range names {
		typedef := meta.Typedefs[name]
		if block, ok := g.buildCloser(meta, name, &typedef, imports); ok {
			body.WriteString(block)
		}
	}
	return body.String()
}

func (g *Generator) buildCloser(meta *win32meta.NamespaceMeta, handleName string, typedef *win32meta.Typedef, imports typemap.ImportSet) (string, bool) {
	context := typemap.Context{Namespace: meta.Namespace}

	// The handle must be a uintptr-backed handle typedef we can convert.
	handleRef := win32meta.TypeRef{Kind: "ApiRef", Name: handleName, Api: meta.Namespace, TargetKind: "Typedef"}
	handleResolved := g.mapper.GoType(&handleRef, context, imports)
	if handleResolved.Kind != typemap.KindHandleTypedef && handleResolved.Kind != typemap.KindScalarTypedef {
		return "", false
	}

	owner := g.registry.FunctionOwner[typedef.FreeFunc]
	if owner.Function == nil {
		g.diag("handle %s: free func %s ambiguous or unknown, no closer", handleName, typedef.FreeFunc)
		return "", false
	}
	if len(owner.Function.Params) != 1 {
		g.diag("handle %s: free func %s takes %d params, no closer", handleName, typedef.FreeFunc, len(owner.Function.Params))
		return "", false
	}
	// The closer parameter must be a convertible-from-handle scalar/handle.
	paramResolved := g.mapper.GoType(&owner.Function.Params[0].Type, context, imports)
	switch paramResolved.Kind {
	case typemap.KindHandleTypedef, typemap.KindScalarTypedef, typemap.KindScalar:
	default:
		g.diag("handle %s: free func %s param not handle-convertible, no closer", handleName, typedef.FreeFunc)
		return "", false
	}

	// The free function is emitted with an idiomatic signature; normalize its
	// return to error. Close functions are not Unicode -W variants, so their
	// emitted Go name is naming.Export(FreeFunc).
	freeReturn := g.mapper.GoType(&owner.Function.Return, typemap.Context{Namespace: owner.Namespace, IsReturn: true}, typemap.ImportSet{})
	freeName := naming.Export(typedef.FreeFunc)
	callee := freeName
	if owner.Namespace != meta.Namespace {
		alias := naming.ImportAlias(owner.Namespace)
		callee = alias + "." + freeName
		imports[alias] = g.mapper.ModulePath + "/bindings/win32/" + naming.PackagePath(owner.Namespace)
	}
	call := callee + "(" + paramResolved.GoType + "(h))"

	var returnStmt string
	switch {
	case isHRESULT(freeReturn) || (owner.Function.SetLastError && isBOOL(freeReturn)):
		// Emitted as returning error.
		returnStmt = "return " + call
	case isBOOL(freeReturn):
		// Emitted as returning bool.
		returnStmt = "return win32.BoolErr(win32.Bool32(" + call + "))"
		imports["win32"] = g.mapper.ModulePath + "/bindings/runtime/win32"
	case freeReturn.Kind == typemap.KindVoid:
		returnStmt = call + "\n\treturn nil"
	default:
		g.diag("handle %s: free func %s return not normalizable, no closer", handleName, typedef.FreeFunc)
		return "", false
	}

	closerName := "Close" + naming.Export(handleName)
	if !g.claimName(closerName) {
		g.diag("handle %s: closer name %s already used", handleName, closerName)
		return "", false
	}

	return fmt.Sprintf(
		"// %s releases a %s handle by calling %s.\nfunc %s(h %s) error {\n\t%s\n}\n\n",
		closerName, handleName, typedef.FreeFunc, closerName, handleResolved.GoType, returnStmt), true
}
