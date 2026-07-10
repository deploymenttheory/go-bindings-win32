package idiowin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/idiomatic/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// buildFunctionModels transforms each raw-emitted function into an idiomatic
// wrapper. Functions whose raw form the tier cannot improve or safely call
// are skipped with a diagnostic.
func (g *Generator) buildFunctionModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.FunctionModel {
	emitted := g.emittedFunctions[meta.Namespace]
	if len(emitted) == 0 {
		return nil
	}

	// The raw package is imported under the same alias the mapper qualifies
	// cross-namespace refs with, so a same-namespace type and the raw call
	// share one import.
	rawAlias := naming.ImportAlias(meta.Namespace)

	// Iterate in metadata order and take the first amd64 entry per name —
	// identical to the raw tier's "claim first" selection, so the wrapper's
	// resolved types always match the raw function it calls. (A plain sort
	// would diverge: sort.Slice is unstable and Win32 has same-named
	// duplicate function entries.)
	seen := map[string]bool{}
	var models []view.FunctionModel
	for i := range meta.Functions {
		function := &meta.Functions[i]
		if !amd64Compatible(function.Availability.Architectures) {
			continue
		}
		rawName := naming.Export(function.Name)
		if seen[rawName] || !emitted[rawName] {
			continue
		}
		seen[rawName] = true
		model, ok := g.buildFunction(meta, function, rawName, rawAlias, imports)
		if !ok {
			continue
		}
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].GoName < models[j].GoName })
	if len(models) > 0 {
		imports[rawAlias] = g.rawImportPath(meta.Namespace)
	}
	return models
}

// idiomaticParam is the resolved idiomatic form of one raw parameter.
type idiomaticParam struct {
	// decl is the idiomatic parameter declaration ("name string"); empty
	// when the parameter is elided (reserved).
	decl string
	// preamble converts the idiomatic param into a raw-arg local; empty when
	// the raw arg is the param verbatim.
	preamble string
	// rawArg is the expression passed to the raw call.
	rawArg string
}

// buildFunction produces one idiomatic wrapper model.
func (g *Generator) buildFunction(meta *win32meta.NamespaceMeta, function *win32meta.Function, rawName, rawAlias string, imports typemap.ImportSet) (view.FunctionModel, bool) {
	// QualifyOwn: the idiomatic package names raw types from a different
	// package, so even same-namespace refs must be qualified. Blocked/skip
	// decisions still key off the real namespace, so signatures match raw.
	context := typemap.Context{Namespace: meta.Namespace, QualifyOwn: true}
	scratch := typemap.ImportSet{}

	var decls, preamble, rawArgs []string
	improved := false
	for i := range function.Params {
		param := &function.Params[i]
		resolved := g.mapper.GoType(&param.Type, context, scratch)
		idiomatic := g.idiomaticParam(param, resolved, i)
		if idiomatic.decl == "" && idiomatic.preamble == "" {
			// Elided reserved parameter.
			improved = true
		}
		if idiomatic.decl != "" {
			decls = append(decls, idiomatic.decl)
		}
		if idiomatic.preamble != "" {
			preamble = append(preamble, idiomatic.preamble)
			improved = true
		}
		rawArgs = append(rawArgs, idiomatic.rawArg)
	}

	returnContext := context
	returnContext.IsReturn = true
	returnContext.QualifyOwn = true
	returnResolved := g.mapper.GoType(&function.Return, returnContext, scratch)

	model := view.FunctionModel{
		ParamStr: strings.Join(decls, ", "),
		RawCall:  rawAlias + "." + rawName + "(" + strings.Join(rawArgs, ", ") + ")",
		Preamble: preamble,
	}
	if !g.buildReturn(&model, function, returnResolved, &improved) {
		return view.FunctionModel{}, false
	}

	// Choose the exported name: desuffix a -W function when the bare name is
	// free in this package; otherwise keep the raw name.
	goName := rawName
	if function.UnsuffixedName != "" {
		candidate := naming.Export(function.UnsuffixedName)
		if g.claimName(candidate) {
			goName = candidate
			improved = true
		} else {
			g.diag("idiomatic %s: bare name %s taken, keeping %s", meta.Namespace, candidate, rawName)
		}
	}
	if goName == rawName {
		if !g.claimName(goName) {
			g.diag("idiomatic %s: name %s already used", meta.Namespace, goName)
			return view.FunctionModel{}, false
		}
	}
	model.GoName = goName

	// Nothing gained over the raw call: skip to avoid a pointless alias.
	if !improved {
		g.claimedNames[goName] = false
		delete(g.claimedNames, goName)
		return view.FunctionModel{}, false
	}

	if preambleUsesWin32(preamble) || model.RetExpr != "" && strings.Contains(model.RetExpr, "win32.") {
		imports["win32"] = g.modulePath + "/bindings/runtime/win32"
	}
	for alias, path := range scratch {
		imports[alias] = path
	}
	model.CommentLines = idiomaticComments(function, goName, rawName)
	return model, true
}

// idiomaticParam maps one raw parameter to its idiomatic form.
func (g *Generator) idiomaticParam(param *win32meta.Param, resolved typemap.Resolved, index int) idiomaticParam {
	name := naming.ParamName(param.Name)

	// Reserved parameters always take NULL: drop them from the signature,
	// passing the raw type's zero value.
	if param.IsReserved {
		return idiomaticParam{rawArg: zeroValue(resolved)}
	}

	// Input PWSTR/PSTR string pointers become Go strings.
	if isWideStringPtr(resolved) && !param.IsOut {
		local := "_" + name
		return idiomaticParam{
			decl:     name + " string",
			preamble: fmt.Sprintf("%s := win32.UTF16Ptr(%s)", local, name),
			rawArg:   resolved.GoType + "(" + local + ")",
		}
	}

	// BOOL inputs become Go bool.
	if isBOOL(resolved) && !param.IsOut {
		local := "_" + name
		return idiomaticParam{
			decl:     name + " bool",
			preamble: fmt.Sprintf("%s := %s(win32.Bool32(%s))", local, resolved.GoType, name),
			rawArg:   local,
		}
	}

	// Everything else passes through unchanged.
	return idiomaticParam{decl: name + " " + resolved.GoType, rawArg: name}
}

// buildReturn selects the idiomatic return handling.
func (g *Generator) buildReturn(model *view.FunctionModel, function *win32meta.Function, resolved typemap.Resolved, improved *bool) bool {
	// Raw BOOL + SetLastError already returns error — pass it through.
	if function.SetLastError && isBOOL(resolved) {
		model.Shape = view.FuncErrorOnly
		model.ReturnSig = "error"
		return true
	}
	// Raw value + SetLastError returns (T, error): the idiomatic value keeps
	// the raw type; pass through unchanged.
	if function.SetLastError {
		model.Shape = view.FuncPassthrough
		model.ReturnSig = "(" + resolved.GoType + ", error)"
		return true
	}
	// HRESULT return with no SetLastError → error.
	if isHRESULT(resolved) {
		model.Shape = view.FuncValueError
		model.ReturnSig = "error"
		model.RetExpr = "return win32.HRESULTError(int32(" + model.RawCall + "))"
		*improved = true
		return true
	}
	// Plain BOOL return → Go bool.
	if isBOOL(resolved) {
		model.Shape = view.FuncBoolResult
		model.ReturnSig = "bool"
		*improved = true
		return true
	}
	// Void.
	if resolved.Kind == typemap.KindVoid {
		model.Shape = view.FuncPassthrough
		return true
	}
	// Any other value: pass through.
	model.Shape = view.FuncPassthrough
	model.ReturnSig = resolved.GoType
	return true
}

// zeroValue renders the raw type's zero for an elided argument: nil for
// pointer-shaped types, 0 for scalars/handles.
func zeroValue(resolved typemap.Resolved) string {
	switch resolved.Kind {
	case typemap.KindPointer, typemap.KindComPtr, typemap.KindPointerTypedef:
		return "nil"
	default:
		return "0"
	}
}

// amd64Compatible mirrors the raw tier's architecture selection.
func amd64Compatible(architectures []string) bool {
	if len(architectures) == 0 {
		return true
	}
	for _, arch := range architectures {
		if arch == "amd64" {
			return true
		}
	}
	return false
}

func isWideStringPtr(resolved typemap.Resolved) bool {
	return resolved.Kind == typemap.KindPointerTypedef &&
		resolved.TypedefApi == "Foundation" &&
		(resolved.TypedefName == "PWSTR" || resolved.TypedefName == "PCWSTR")
}

func isBOOL(resolved typemap.Resolved) bool {
	return resolved.TypedefApi == "Foundation" && resolved.TypedefName == "BOOL"
}

func isHRESULT(resolved typemap.Resolved) bool {
	return resolved.TypedefApi == "Foundation" && resolved.TypedefName == "HRESULT"
}

func preambleUsesWin32(preamble []string) bool {
	for _, line := range preamble {
		if strings.Contains(line, "win32.") {
			return true
		}
	}
	return false
}

func idiomaticComments(function *win32meta.Function, goName, rawName string) []string {
	lines := []string{fmt.Sprintf("%s wraps the raw %s call with idiomatic Go types.", goName, rawName)}
	if function.Availability.DocURL != "" {
		lines = append(lines, function.Availability.DocURL)
	}
	return lines
}
