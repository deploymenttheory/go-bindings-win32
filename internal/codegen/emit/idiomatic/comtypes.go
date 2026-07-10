package idiowin

import (
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// comEmitted reports whether the idiomatic wrapper for a COM interface was
// emitted (the raw tier marked the interface as emitted).
func (g *Generator) comEmitted(api, name string) bool {
	return g.emittedComMethods[api+"\x00"+naming.Export(name)] != nil
}

// comWrapperType returns the idiomatic wrapper type for a COM interface and
// its Wrap constructor expression, qualified for the package being emitted.
// ok is false when the wrapper was not emitted (caller keeps the raw type).
func (g *Generator) comWrapperType(api, name, currentNamespace string, imports typemap.ImportSet) (goType, constructor string, ok bool) {
	if !g.comEmitted(api, name) {
		return "", "", false
	}
	wrapper := naming.Export(name)
	if api == currentNamespace {
		return wrapper, "Wrap" + wrapper, true
	}
	alias := naming.ImportAlias(api) + "idiom"
	imports[alias] = g.idiomaticImportPath(api)
	return alias + "." + wrapper, alias + ".Wrap" + wrapper, true
}

// comInParam returns the idiomatic wrapper declaration and raw argument for
// an input COM interface parameter (`IFoo*`): the caller passes the wrapper
// value and the raw call receives its `.Raw` pointer. ok is false when the
// param is not an input COM pointer or the wrapper is unavailable.
func (g *Generator) comInParam(param *win32meta.Param, resolved typemap.Resolved, currentNamespace string, imports typemap.ImportSet) (decl, rawArg string, ok bool) {
	if resolved.Kind != typemap.KindComPtr || param.IsOut {
		return "", "", false
	}
	if param.Type.Kind != "ApiRef" || param.Type.TargetKind != "Com" {
		return "", "", false
	}
	goType, _, wrapped := g.comWrapperType(param.Type.Api, param.Type.Name, currentNamespace, imports)
	if !wrapped {
		return "", "", false
	}
	name := naming.ParamName(param.Name)
	return name + " " + goType, name + ".Raw", true
}

// elevateRetValParam plans elevating an [out,retval] parameter into a Go
// return value. For a COM out (`IFoo**`) it returns the idiomatic wrapper
// type and wraps the raw pointer; for any other typed pointer it returns the
// raw element directly. ok is false when the parameter is not elevatable.
func (g *Generator) elevateRetValParam(param *win32meta.Param, resolved typemap.Resolved, currentNamespace string, imports typemap.ImportSet) (preambleDecl, rawArg, returnExpr, returnType string, ok bool) {
	if !param.IsRetVal || param.IsIn {
		return "", "", "", "", false
	}
	local := "_" + naming.ParamName(param.Name)

	// COM interface out parameter → idiomatic wrapper return value.
	if param.Type.Kind == "PointerTo" && param.Type.Child != nil &&
		param.Type.Child.Kind == "ApiRef" && param.Type.Child.TargetKind == "Com" &&
		strings.HasPrefix(resolved.GoType, "*") {
		goType, constructor, wrapped := g.comWrapperType(param.Type.Child.Api, param.Type.Child.Name, currentNamespace, imports)
		if wrapped {
			rawElement := resolved.GoType[1:] // *rawpkg.IFoo
			return "var " + local + " " + rawElement, "&" + local, constructor + "(" + local + ")", goType, true
		}
	}

	// Plain typed-pointer out value.
	element, viable := retValElement(param, resolved)
	if !viable {
		return "", "", "", "", false
	}
	return "var " + local + " " + element, "&" + local, local, element, true
}
