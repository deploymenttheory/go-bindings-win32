package rawwin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// buildFunctionModels converts a namespace's functions plus the DLL/proc
// declaration block that dispatches them. Each function is emitted once with
// the best shape: idiomatic Go signature (string/bool/[]T/error/…) dispatching
// directly through syscall.SyscallN.
func (g *Generator) buildFunctionModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) ([]view.FunctionModel, []view.DLLModel) {
	var functions []view.FunctionModel
	dllProcs := map[string][]view.ProcModel{}
	dllSpelling := map[string]string{}

	// Iterate in metadata order taking the first amd64 entry per name (Win32
	// has same-named duplicate entries); claim names in that order so the
	// -W desuffix is deterministic. Output is sorted by Go name below.
	seen := map[string]bool{}
	for i := range meta.Functions {
		function := &meta.Functions[i]
		if !amd64Compatible(function.Availability.Architectures) {
			continue
		}
		rawName := naming.Export(function.Name)
		if seen[rawName] {
			continue
		}
		seen[rawName] = true
		model, ok := g.buildFunction(meta, function, rawName, imports)
		if !ok {
			continue
		}
		functions = append(functions, model)

		exportName := function.Name
		if function.EntryPoint != "" {
			exportName = function.EntryPoint
		}
		// DLL names vary in case across functions ("MFPlat.dll" /
		// "mfplat.dll"); group case-insensitively, keep first spelling.
		key := strings.ToUpper(function.DLL)
		if dllSpelling[key] == "" {
			dllSpelling[key] = function.DLL
		}
		dllProcs[key] = append(dllProcs[key], view.ProcModel{
			VarName:    model.ProcVar,
			ExportName: exportName,
		})
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i].GoName < functions[j].GoName })

	dllKeys := make([]string, 0, len(dllProcs))
	for key := range dllProcs {
		dllKeys = append(dllKeys, key)
	}
	sort.Strings(dllKeys)
	dlls := make([]view.DLLModel, 0, len(dllKeys))
	for _, key := range dllKeys {
		procs := dllProcs[key]
		sort.Slice(procs, func(i, j int) bool { return procs[i].VarName < procs[j].VarName })
		dlls = append(dlls, view.DLLModel{
			VarName:  dllVarName(dllSpelling[key]),
			FileName: dllSpelling[key],
			Procs:    procs,
		})
	}
	return functions, dlls
}

// splitIdents extracts the Go identifiers from a type expression
// ("*security.SECURITY_ATTRIBUTES" → ["security", "SECURITY_ATTRIBUTES"]),
// used to detect parameter names that would shadow a type referenced in the
// signature.
func splitIdents(expr string) []string {
	var idents []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			idents = append(idents, current.String())
			current.Reset()
		}
	}
	for _, r := range expr {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			current.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return idents
}

// avoidCollision suffixes a parameter name with '_' until it no longer matches
// any type identifier used in the signature. Go puts parameter names in scope
// for the result types, so a param named e.g. "Node" would shadow the type
// "Node" in a "(*Node, error)" return, making it an invalid type reference.
func avoidCollision(name string, reserved map[string]bool) string {
	for reserved[name] {
		name += "_"
	}
	return name
}

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

// dllVarName derives the module variable from the DLL file name
// ("KERNEL32.dll" → "modKERNEL32", "api-ms-win-core-x.dll" → sanitized).
func dllVarName(dll string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(dll, ".dll"), ".DLL")
	var builder strings.Builder
	builder.WriteString("mod")
	for _, r := range base {
		if r == '-' || r == '.' {
			builder.WriteByte('_')
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// buildFunction resolves one function into a render model: an idiomatic Go
// signature whose body converts its params and dispatches via syscall.
// Functions whose signatures cannot be marshaled (by-value struct/float
// params or returns) are skipped with a diagnostic.
func (g *Generator) buildFunction(meta *win32meta.NamespaceMeta, function *win32meta.Function, rawName string, imports typemap.ImportSet) (view.FunctionModel, bool) {
	context := typemap.Context{Namespace: meta.Namespace}
	scratch := typemap.ImportSet{}

	// Resolve every parameter up front, then plan array→slice collapses
	// (which span two params: the pointer and its count).
	resolvedParams := make([]typemap.Resolved, len(function.Params))
	for i := range function.Params {
		resolvedParams[i] = g.mapper.GoType(&function.Params[i].Type, context, scratch)
	}
	retypeComOutParams(function.Params, resolvedParams, scratch, g.mapper.ModulePath)
	slicePlans, elidedCounts := planSliceParams(function.Params, resolvedParams, true)

	returnContext := context
	returnContext.IsReturn = true
	returnResolved := g.mapper.GoType(&function.Return, returnContext, scratch)
	retValMode := retValElevationMode(function, returnResolved)

	// Parameter names enter scope for the result types, so rename any that
	// would shadow a type identifier used anywhere in the signature.
	reserved := map[string]bool{}
	for _, ident := range splitIdents(returnResolved.GoType) {
		reserved[ident] = true
	}
	for i := range resolvedParams {
		for _, ident := range splitIdents(resolvedParams[i].GoType) {
			reserved[ident] = true
		}
	}
	paramNames := make([]string, len(function.Params))
	for i := range function.Params {
		paramNames[i] = avoidCollision(naming.ParamName(function.Params[i].Name), reserved)
	}

	var decls, preamble, argWords, returnValues, returnTypes []string
	usesUnsafe := false
	for i := range function.Params {
		param := &function.Params[i]
		resolved := resolvedParams[i]

		// [out,retval] → elevated Go return value (a COM out is just a typed
		// pointer here, so no special-casing is needed).
		if retValMode != retValNone {
			if element, ok := retValElement(param, resolved); ok {
				local := "_" + paramNames[i]
				preamble = append(preamble, "var "+local+" "+element)
				argWords = append(argWords, "uintptr(unsafe.Pointer(&"+local+"))")
				returnValues = append(returnValues, local)
				returnTypes = append(returnTypes, element)
				usesUnsafe = true
				continue
			}
		}
		// A count parameter collapsed into a slice: derive from len().
		if arrayIndex, ok := elidedCounts[i]; ok {
			argWords = append(argWords, "uintptr(len("+paramNames[arrayIndex]+"))")
			continue
		}
		// An array pointer collapsed into a []T parameter.
		if plan, ok := slicePlans[i]; ok {
			name := paramNames[i]
			local := "_" + name
			decls = append(decls, name+" []"+plan.element)
			preamble = append(preamble, "var "+local+" "+plan.rawPointerType)
			preamble = append(preamble, "if len("+name+") > 0 { "+local+" = &"+name+"[0] }")
			argWords = append(argWords, "uintptr(unsafe.Pointer("+local+"))")
			usesUnsafe = true
			continue
		}

		decl, pre, word, pointer, ok := shapeParam(paramNames[i], param, resolved)
		if !ok {
			g.diag("function %s: param %s not marshalable (%s), function skipped",
				function.Name, param.Name, resolved.GoType)
			return view.FunctionModel{}, false
		}
		if decl != "" {
			decls = append(decls, decl)
		}
		preamble = append(preamble, pre...)
		argWords = append(argWords, word)
		usesUnsafe = usesUnsafe || pointer
	}

	model := view.FunctionModel{
		ProcVar:  "proc" + rawName,
		ParamStr: strings.Join(decls, ", "),
		Preamble: preamble,
		ArgExprs: argWords,
	}
	if len(returnValues) > 0 {
		model.ReturnValues = returnValues
		switch retValMode {
		case retValHRESULT:
			model.ReturnKind = view.RetRetValHResult
			model.ReturnSig = "(" + strings.Join(append(returnTypes, "error"), ", ") + ")"
		case retValRawError:
			model.ReturnKind = view.RetRetValBoolErr
			model.ReturnSig = "(" + strings.Join(append(returnTypes, "error"), ", ") + ")"
		case retValVoid:
			model.ReturnKind = view.RetRetValVoid
			if len(returnTypes) == 1 {
				model.ReturnSig = returnTypes[0]
			} else {
				model.ReturnSig = "(" + strings.Join(returnTypes, ", ") + ")"
			}
		}
	} else if !g.buildReturnShape(&model, function, returnResolved) {
		return view.FunctionModel{}, false
	}

	// Choose the exported name: drop a trailing -W when the bare name is free.
	goName := rawName
	if function.UnsuffixedName != "" {
		candidate := naming.Export(function.UnsuffixedName)
		if g.claimName(candidate) {
			goName = candidate
		} else {
			g.diag("function %s: bare name %s taken, keeping %s", meta.Namespace, candidate, rawName)
		}
	}
	if goName == rawName {
		if !g.claimName(goName) {
			g.diag("function %s: name already used in package %s", function.Name, meta.Namespace)
			return view.FunctionModel{}, false
		}
	}
	model.GoName = goName
	model.ProcVar = "proc" + goName

	// Commit imports and the packages every body needs.
	for alias, path := range scratch {
		imports[alias] = path
	}
	imports["syscall"] = "syscall"
	imports["win32"] = g.mapper.ModulePath + "/bindings/runtime/win32"
	if usesUnsafe {
		imports["unsafe"] = "unsafe"
	}
	model.CommentLines = functionComments(function, goName)
	return model, true
}

// shapeParam maps one non-slice, non-retval parameter to its idiomatic Go
// declaration, any conversion preamble, and the syscall word. pointer reports
// whether the word uses unsafe.Pointer. ok is false when the param cannot be
// marshaled (float / by-value struct); the caller then skips its function or
// method with a context-appropriate diagnostic. Shared by functions and COM
// methods.
func shapeParam(name string, param *win32meta.Param, resolved typemap.Resolved) (decl string, preamble []string, word string, pointer bool, ok bool) {
	// Reserved parameters always take NULL — dropped from the signature.
	if param.IsReserved {
		return "", nil, "0", false, true
	}
	// Input PWSTR/PCWSTR → Go string.
	if isWideStringPtr(resolved) && !param.IsOut {
		local := "_" + name
		return name + " string",
			[]string{local + " := win32.UTF16Ptr(" + name + ")"},
			"uintptr(unsafe.Pointer(" + local + "))", true, true
	}
	// BOOL input → Go bool.
	if isBOOL(resolved) && !param.IsOut {
		local := "_" + name
		return name + " bool",
			[]string{local + " := win32.Bool32(" + name + ")"},
			"uintptr(" + local + ")", false, true
	}
	// Everything else passes through with the resolved type.
	switch typemap.ArgClassOf(resolved, resolved.GoType) {
	case typemap.ArgScalar:
		return name + " " + resolved.GoType, nil, "uintptr(" + name + ")", false, true
	case typemap.ArgPointer:
		return name + " " + resolved.GoType, nil, "uintptr(unsafe.Pointer(" + name + "))", true, true
	default:
		return "", nil, "", false, false
	}
}

// buildReturnShape selects the body/return template shape (idiomatic).
func (g *Generator) buildReturnShape(model *view.FunctionModel, function *win32meta.Function, resolved typemap.Resolved) bool {
	switch resolved.Kind {
	case typemap.KindVoid:
		model.ReturnKind = view.RetVoid
		return true
	case typemap.KindStruct, typemap.KindUnion, typemap.KindArray, typemap.KindGUID:
		g.diag("function %s: by-value %s return not marshalable, function skipped",
			function.Name, resolved.GoType)
		return false
	case typemap.KindScalar:
		if resolved.GoType == "float32" || resolved.GoType == "float64" || resolved.GoType == "bool" {
			g.diag("function %s: %s return not marshalable, function skipped", function.Name, resolved.GoType)
			return false
		}
	}

	model.RetExpr = returnConversion(resolved)

	// BOOL + SetLastError → error (the BOOL carries nothing beyond success).
	if function.SetLastError && isBOOL(resolved) {
		model.ReturnKind = view.RetBoolErr
		model.ReturnSig = "error"
		return true
	}
	// HRESULT → error.
	if isHRESULT(resolved) {
		model.ReturnKind = view.RetHResultErr
		model.ReturnSig = "error"
		return true
	}
	// Plain BOOL (no SetLastError) → bool.
	if isBOOL(resolved) {
		model.ReturnKind = view.RetBoolValue
		model.ReturnSig = "bool"
		return true
	}
	// Value + SetLastError with a known failure sentinel → clean (T, error).
	if function.SetLastError {
		if checks := g.failureChecks(resolved); len(checks) > 0 {
			model.ReturnKind = view.RetValErr
			model.ReturnSig = "(" + resolved.GoType + ", error)"
			model.FailureChecks = checks
			return true
		}
		// No derivable sentinel: err is advisory (GetLastError).
		model.ReturnKind = view.RetValLast
		model.ReturnSig = "(" + resolved.GoType + ", error)"
		return true
	}
	// Plain value → T.
	model.ReturnKind = view.RetVal
	model.ReturnSig = resolved.GoType
	return true
}

// failureChecks derives failure predicates over `ret` from the return type's
// metadata: handle typedefs use their [InvalidHandleValue] sentinels;
// pointers use nil.
func (g *Generator) failureChecks(resolved typemap.Resolved) []string {
	switch resolved.Kind {
	case typemap.KindHandleTypedef:
		typedef := g.mapper.Registry.Typedef(resolved.TypedefApi, resolved.TypedefName)
		if typedef == nil {
			return nil
		}
		values := typedef.InvalidValues
		if len(values) == 0 {
			values = []string{"0"}
		}
		checks := make([]string, 0, len(values))
		for _, value := range values {
			if value == "-1" {
				checks = append(checks, "ret == ^"+resolved.GoType+"(0)")
				continue
			}
			checks = append(checks, "ret == "+value)
		}
		return checks
	case typemap.KindPointer, typemap.KindPointerTypedef, typemap.KindComPtr:
		return []string{"ret == nil"}
	}
	return nil
}

// returnConversion renders the r1 → Go value conversion.
func returnConversion(resolved typemap.Resolved) string {
	switch resolved.Kind {
	case typemap.KindPointer, typemap.KindComPtr:
		if resolved.GoType == "unsafe.Pointer" {
			return "unsafe.Pointer(r1)"
		}
		return "(" + resolved.GoType + ")(unsafe.Pointer(r1))"
	case typemap.KindPointerTypedef:
		return resolved.GoType + "(unsafe.Pointer(r1))"
	case typemap.KindVoid:
		return ""
	default:
		return resolved.GoType + "(r1)"
	}
}

// functionComments assembles the doc comment lines.
func functionComments(function *win32meta.Function, goName string) []string {
	lines := []string{fmt.Sprintf("%s calls %s!%s.", goName, strings.TrimSuffix(function.DLL, ".dll"), exportOf(function))}
	if function.Availability.DocURL != "" {
		lines = append(lines, function.Availability.DocURL)
	}
	if function.Availability.Platform != "" {
		lines = append(lines, "Minimum OS: "+function.Availability.Platform+".")
	}
	return lines
}

func exportOf(function *win32meta.Function) string {
	if function.EntryPoint != "" {
		return function.EntryPoint
	}
	return function.Name
}

// ── idiomatic param/return helpers (folded from the former idiomatic tier) ──

// slicePlan describes an array-pointer parameter to collapse into a []T.
type slicePlan struct {
	element        string // Go element type
	rawPointerType string // pointer type of the address-of local ("*foundation.XXX")
	countIndex     int    // the count parameter this slice supplies
}

// planSliceParams finds parameter pairs to collapse into a single slice
// parameter, returning the buffer-index→plan and size-index→buffer-index maps.
// Two shapes qualify:
//
//   - typed arrays (typedArrays=true): an array pointer plus its
//     [NativeArrayInfo] CountParamIndex input count → []T. The count must be
//     referenced by exactly one array, the array a typed pointer (not void*),
//     and the count an input integer.
//   - byte buffers: a literal void* or byte* plus its [MemorySize]
//     BytesParamIndex input size → []byte. The size must be referenced by
//     exactly one buffer and by no [NativeArrayInfo] count (a size shared
//     with a typed-array collapse stays raw rather than un-collapsing the
//     array). Typed non-byte pointers keep their type rather than blobbing.
//
// A buffer that is [retval] or [Reserved] never collapses: those params are
// consumed earlier in the shaping loop, and a surviving size elision would
// reference an undeclared identifier.
func planSliceParams(params []win32meta.Param, resolved []typemap.Resolved, typedArrays bool) (map[int]slicePlan, map[int]int) {
	references := map[int]int{}
	for i := range params {
		if j := params[i].NativeArrayCountParamIndex; j >= 0 {
			references[j]++
		}
	}
	plans := map[int]slicePlan{}
	elided := map[int]int{}
	if typedArrays {
		for i := range params {
			param := &params[i]
			if param.IsRetVal || param.IsReserved {
				continue
			}
			countIndex := param.NativeArrayCountParamIndex
			if countIndex < 0 || countIndex >= len(params) || countIndex == i {
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
			countParam := &params[countIndex]
			if countParam.IsOut || countParam.IsReserved || !isIntegerCount(resolved[countIndex]) {
				continue
			}
			plans[i] = slicePlan{element: element, rawPointerType: array.GoType, countIndex: countIndex}
			elided[countIndex] = i
		}
	}
	sizeReferences := map[int]int{}
	for i := range params {
		if j := params[i].MemorySizeBytesParamIndex; j >= 0 {
			sizeReferences[j]++
		}
	}
	for i := range params {
		param := &params[i]
		if param.IsRetVal || param.IsReserved {
			continue
		}
		if _, planned := plans[i]; planned {
			continue
		}
		if !isVoidPointer(&param.Type) && !isBytePointer(&param.Type) {
			continue
		}
		sizeIndex := param.MemorySizeBytesParamIndex
		if sizeIndex < 0 || sizeIndex >= len(params) || sizeIndex == i {
			continue
		}
		if sizeReferences[sizeIndex] != 1 || references[sizeIndex] != 0 {
			continue // shared size, or a typed-array count owns this param
		}
		if _, taken := elided[sizeIndex]; taken {
			continue
		}
		sizeParam := &params[sizeIndex]
		if sizeParam.IsOut || sizeParam.IsReserved || !isIntegerCount(resolved[sizeIndex]) {
			continue
		}
		plans[i] = slicePlan{element: "byte", rawPointerType: "*byte", countIndex: sizeIndex}
		elided[sizeIndex] = i
	}
	return plans, elided
}

// isVoidPointer reports whether a metadata type is literally void*.
func isVoidPointer(ref *win32meta.TypeRef) bool {
	return ref.Kind == "PointerTo" && ref.Child != nil &&
		ref.Child.Kind == "Native" && ref.Child.Name == "Void"
}

// isVoidDoublePointer reports whether a metadata type is literally void**.
func isVoidDoublePointer(ref *win32meta.TypeRef) bool {
	return ref.Kind == "PointerTo" && ref.Child != nil && isVoidPointer(ref.Child)
}

// retypeComOutParams upgrades void** [out] params carrying [ComOutPtr] or an
// [IidParameterIndex] linkage to **win32.IUnknown: the metadata guarantees
// the slot receives a COM object pointer (the concrete interface is selected
// at runtime by the riid argument). The marshaling word is unchanged; a
// [retval] param retyped here elevates through the normal retValElement path.
func retypeComOutParams(params []win32meta.Param, resolved []typemap.Resolved, imports typemap.ImportSet, modulePath string) {
	for i := range params {
		param := &params[i]
		if !param.IsOut || param.IsReserved {
			continue
		}
		if !param.IsComOutPtr && param.IidParamIndex < 0 {
			continue
		}
		if !isVoidDoublePointer(&param.Type) {
			continue
		}
		resolved[i] = typemap.Resolved{GoType: "**win32.IUnknown", Kind: typemap.KindPointer}
		imports["win32"] = modulePath + "/bindings/runtime/win32"
	}
}

// isBytePointer reports whether a metadata type is literally byte*.
func isBytePointer(ref *win32meta.TypeRef) bool {
	return ref.Kind == "PointerTo" && ref.Child != nil &&
		ref.Child.Kind == "Native" && ref.Child.Name == "Byte"
}

// retVal elevation modes.
const (
	retValNone = iota
	retValHRESULT
	retValRawError // syscall status is a BOOL + GetLastError
	retValVoid
)

// retValElevationMode decides whether a function's [out,retval] params can be
// elevated to return values, based on the return status shape. Only a clean
// status (HRESULT, BOOL+SetLastError, or void) qualifies.
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

// retValElement returns the Go element type behind an elevatable [out,retval]
// pointer param (a typed pointer, incl. a COM out `IFoo**` whose element is
// `*IFoo`), or ok=false.
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
// slice length.
func isIntegerCount(resolved typemap.Resolved) bool {
	switch resolved.Kind {
	case typemap.KindScalar:
		return resolved.GoType != "bool" && resolved.GoType != "float32" && resolved.GoType != "float64"
	case typemap.KindScalarTypedef, typemap.KindEnum:
		return true
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
