package rawwin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Header inlines.
//
// win32metadata attributes a handful of "functions" to the pseudo-module
// FORCEINLINE: SDK header macros that no DLL exports (GetCurrentProcessToken
// is ((HANDLE)(LONG_PTR) -4)). The metadata carries their value as a
// [Constant] attribute, which the ingest projects to Function.Constant.
// Dispatching such a function through LoadLibrary would panic at first
// call, so one with a constant is emitted as a plain Go function returning
// that value, and one without is skipped with a diagnostic the ratchet
// surfaces.

// loadableModuleSuffixes are the PE module extensions Win32 metadata
// attributes real exports to.
var loadableModuleSuffixes = []string{".dll", ".drv", ".exe", ".cpl", ".sys", ".ax", ".ocx"}

// IsLoadableModule reports whether a DllImport module name denotes a real PE
// module rather than a header-inline marker such as "FORCEINLINE".
func IsLoadableModule(dll string) bool {
	lower := strings.ToLower(dll)
	for _, suffix := range loadableModuleSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// buildInlineFunction emits a [Constant] header inline as a Go function
// returning the constant, or skips a constant-less non-export with a
// diagnostic.
func (g *Generator) buildInlineFunction(meta *win32meta.NamespaceMeta, function *win32meta.Function, rawName string, imports typemap.ImportSet) (view.FunctionModel, bool) {
	if function.Constant == "" {
		g.diag("function %s: DLL %q is not a loadable module and no [Constant] value is recorded, skipped", function.Name, function.DLL)
		return view.FunctionModel{}, false
	}
	if len(function.Params) != 0 {
		g.diag("function %s: [Constant] inline takes %d params, skipped", function.Name, len(function.Params))
		return view.FunctionModel{}, false
	}
	scratch := typemap.ImportSet{}
	resolved := g.mapper.GoType(&function.Return, typemap.Context{Namespace: meta.Namespace, IsReturn: true}, scratch)
	if resolved.Kind == typemap.KindUnsupported || resolved.Kind == typemap.KindVoid {
		g.diag("function %s: [Constant] inline return not representable, skipped", function.Name)
		return view.FunctionModel{}, false
	}
	expr, ok := g.inlineConstantExpr(meta, function, resolved)
	if !ok {
		g.diag("function %s: [Constant] value %q not representable as %s, skipped", function.Name, function.Constant, resolved.GoType)
		return view.FunctionModel{}, false
	}
	goName, ok := g.claimFunctionName(meta, function, rawName)
	if !ok {
		return view.FunctionModel{}, false
	}
	for alias, path := range scratch {
		imports[alias] = path
	}
	return view.FunctionModel{
		GoName:     goName,
		ReturnSig:  resolved.GoType,
		ReturnKind: view.RetInline,
		RetExpr:    expr,
		CommentLines: append([]string{
			fmt.Sprintf("%s is a header inline, not a DLL export: it evaluates to the constant %s.", goName, function.Constant),
		}, availabilityComments(function)...),
	}, true
}

// inlineConstantExpr renders the [Constant] value as a typed Go expression.
// Negative values on uintptr-backed returns (handles, pointer sentinels)
// wrap to the two's-complement word: -4 → ^T(3).
func (g *Generator) inlineConstantExpr(meta *win32meta.NamespaceMeta, function *win32meta.Function, resolved typemap.Resolved) (string, bool) {
	literal := function.Constant
	value, err := strconv.ParseInt(literal, 10, 64)
	if err != nil {
		return "", false
	}
	base := g.constantBase(meta, &function.Return)
	if value < 0 && (resolved.Kind == typemap.KindHandleTypedef || resolved.GoType == "uintptr" || base == "uintptr") {
		return fmt.Sprintf("^%s(%d)", resolved.GoType, -value-1), true
	}
	if value < 0 {
		literal = literalForBase(literal, base)
	}
	return resolved.GoType + "(" + literal + ")", true
}
