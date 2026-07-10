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

	// Resolve every parameter up front, then plan array→slice collapses
	// (which span two params: the pointer and its count).
	resolvedParams := make([]typemap.Resolved, len(function.Params))
	for i := range function.Params {
		resolvedParams[i] = g.mapper.GoType(&function.Params[i].Type, context, scratch)
	}
	slicePlans, elidedCounts := planSliceParams(function, resolvedParams)

	returnContext := context
	returnContext.IsReturn = true
	returnResolved := g.mapper.GoType(&function.Return, returnContext, scratch)

	// [out,retval] elevation is viable only when the raw return is a clean
	// status (HRESULT, BOOL+SetLastError→error, or void). A raw
	// (value, error) return would collide with the elevated values.
	retValMode := retValElevationMode(function, returnResolved)

	var decls, preamble, rawArgs, returnValues, returnTypes []string
	improved := false
	for i := range function.Params {
		param := &function.Params[i]
		resolved := resolvedParams[i]

		if retValMode != retValNone {
			if decl, rawArg, returnExpr, returnType, ok := g.elevateRetValParam(param, resolved, meta.Namespace, scratch); ok {
				preamble = append(preamble, decl)
				rawArgs = append(rawArgs, rawArg)
				returnValues = append(returnValues, returnExpr)
				returnTypes = append(returnTypes, returnType)
				improved = true
				continue
			}
		}
		// Input COM interface param → idiomatic wrapper value.
		if decl, rawArg, ok := g.comInParam(param, resolved, meta.Namespace, scratch); ok {
			decls = append(decls, decl)
			rawArgs = append(rawArgs, rawArg)
			improved = true
			continue
		}

		// A count parameter collapsed into a slice: drop it, derive its
		// value from the slice length at the call site.
		if arrayIndex, ok := elidedCounts[i]; ok {
			arrayName := naming.ParamName(function.Params[arrayIndex].Name)
			rawArgs = append(rawArgs, resolved.GoType+"(len("+arrayName+"))")
			improved = true
			continue
		}
		// An array pointer collapsed into a []T parameter.
		if plan, ok := slicePlans[i]; ok {
			name := naming.ParamName(param.Name)
			local := "_" + name
			decls = append(decls, name+" []"+plan.element)
			preamble = append(preamble, "var "+local+" "+plan.rawPointerType)
			preamble = append(preamble, "if len("+name+") > 0 { "+local+" = &"+name+"[0] }")
			rawArgs = append(rawArgs, local)
			improved = true
			continue
		}

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

	model := view.FunctionModel{
		ParamStr: strings.Join(decls, ", "),
		RawCall:  rawAlias + "." + rawName + "(" + strings.Join(rawArgs, ", ") + ")",
		Preamble: preamble,
	}
	if len(returnValues) > 0 {
		model.ReturnValues = returnValues
		switch retValMode {
		case retValHRESULT:
			model.Shape = view.FuncRetValHRESULT
			imports["win32"] = g.modulePath + "/bindings/runtime/win32"
			model.ReturnSig = "(" + strings.Join(append(returnTypes, "error"), ", ") + ")"
		case retValRawError:
			model.Shape = view.FuncRetValRawError
			model.ReturnSig = "(" + strings.Join(append(returnTypes, "error"), ", ") + ")"
		case retValVoid:
			model.Shape = view.FuncRetValVoid
			if len(returnTypes) == 1 {
				model.ReturnSig = returnTypes[0]
			} else {
				model.ReturnSig = "(" + strings.Join(returnTypes, ", ") + ")"
			}
		}
	} else if !g.buildReturn(&model, function, returnResolved, &improved) {
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

// slicePlan describes an array-pointer parameter to collapse into a []T.
type slicePlan struct {
	element        string // Go element type ("foundation.XXX")
	rawPointerType string // raw pointer type passed to the call ("*foundation.XXX")
	countIndex     int    // the count parameter this slice supplies
}

// planSliceParams finds (array pointer, input count) parameter pairs — via
// the ingested [NativeArrayInfo] CountParamIndex — and plans collapsing each
// into a single []T parameter. Returns the array-index→plan map and the
// count-index→array-index elision map.
//
// A pair qualifies only when the count is referenced by exactly one array
// (unambiguous length), the array is a typed pointer (not void*), and the
// count is an input integer (not out/inout, not a pointer). Shared or
// out-counts are left as raw parameters.
func planSliceParams(function *win32meta.Function, resolved []typemap.Resolved) (map[int]slicePlan, map[int]int) {
	references := map[int]int{}
	for i := range function.Params {
		if j := function.Params[i].NativeArrayCountParamIndex; j >= 0 {
			references[j]++
		}
	}

	plans := map[int]slicePlan{}
	elided := map[int]int{}
	for i := range function.Params {
		param := &function.Params[i]
		countIndex := param.NativeArrayCountParamIndex
		if countIndex < 0 || countIndex >= len(function.Params) || countIndex == i {
			continue
		}
		if references[countIndex] != 1 {
			continue // ambiguous shared count
		}
		array := resolved[i]
		if array.Kind != typemap.KindPointer || !strings.HasPrefix(array.GoType, "*") {
			continue
		}
		element := array.GoType[1:]
		if element == "" || strings.Contains(element, "unsafe.") {
			continue
		}
		countParam := &function.Params[countIndex]
		if countParam.IsOut || countParam.IsReserved || !isIntegerCount(resolved[countIndex]) {
			continue
		}
		plans[i] = slicePlan{element: element, rawPointerType: array.GoType, countIndex: countIndex}
		elided[countIndex] = i
	}
	return plans, elided
}

// retVal elevation modes for flat functions.
const (
	retValNone = iota
	retValHRESULT
	retValRawError // raw call already returns a bare error (BOOL+SetLastError)
	retValVoid
)

// retValElevationMode decides whether a flat function's [out,retval] params
// can be elevated, based on the raw return shape. Only clean status returns
// qualify — a raw (value, error) return would collide with elevated values.
func retValElevationMode(function *win32meta.Function, returnResolved typemap.Resolved) int {
	switch {
	case isHRESULT(returnResolved):
		return retValHRESULT
	case function.SetLastError && isBOOL(returnResolved):
		return retValRawError
	case returnResolved.Kind == typemap.KindVoid:
		return retValVoid
	}
	return retValNone
}

// retValElement reports whether a parameter is an elevatable [out,retval]
// pointer and, if so, the Go element type behind the pointer. It excludes
// void pointers (untyped) and non-pointer params.
func retValElement(param *win32meta.Param, resolved typemap.Resolved) (string, bool) {
	if !param.IsRetVal || param.IsIn {
		return "", false
	}
	if resolved.Kind != typemap.KindPointer || !strings.HasPrefix(resolved.GoType, "*") {
		return "", false
	}
	element := resolved.GoType[1:]
	if element == "" || strings.Contains(element, "unsafe.") {
		return "", false
	}
	return element, true
}

// isIntegerCount reports whether a resolved type is an integer usable as a
// slice length (excludes bool/float/pointers).
func isIntegerCount(resolved typemap.Resolved) bool {
	switch resolved.Kind {
	case typemap.KindScalar:
		return resolved.GoType != "bool" && resolved.GoType != "float32" && resolved.GoType != "float64"
	case typemap.KindScalarTypedef, typemap.KindEnum:
		return true
	}
	return false
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
