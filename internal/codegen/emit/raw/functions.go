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
// declaration block that dispatches them.
func (g *Generator) buildFunctionModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) ([]view.FunctionModel, []view.DLLModel) {
	var functions []view.FunctionModel
	dllProcs := map[string][]view.ProcModel{}
	dllSpelling := map[string]string{}

	for i := range meta.Functions {
		function := &meta.Functions[i]
		if !amd64Compatible(function.Availability.Architectures) {
			continue
		}
		goName := naming.Export(function.Name)
		if !g.claimName(goName) {
			g.diag("function %s: name already used in package %s", function.Name, meta.Namespace)
			continue
		}
		model, ok := g.buildFunction(meta, function, imports)
		if !ok {
			g.unclaimName(goName)
			continue
		}
		model.GoName = goName
		model.ProcVar = "proc" + goName
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

// buildFunction resolves one function into a render model. Functions whose
// signatures the raw tier cannot marshal (by-value structs, floats) are
// skipped with a diagnostic.
func (g *Generator) buildFunction(meta *win32meta.NamespaceMeta, function *win32meta.Function, imports typemap.ImportSet) (view.FunctionModel, bool) {
	context := typemap.Context{Namespace: meta.Namespace}
	trialImports := typemap.ImportSet{}

	// Resolve parameters.
	var paramDecls, argExprs []string
	for i := range function.Params {
		param := &function.Params[i]
		resolved := g.mapper.GoType(&param.Type, context, trialImports)
		argClass := typemap.ArgClassOf(resolved, resolved.GoType)
		if argClass == typemap.ArgUnsupported || resolved.Kind == typemap.KindVoid {
			g.diag("function %s: param %s not marshalable (%s), function skipped",
				function.Name, param.Name, resolved.GoType)
			return view.FunctionModel{}, false
		}
		paramName := naming.ParamName(param.Name)
		paramDecls = append(paramDecls, paramName+" "+resolved.GoType)
		if argClass == typemap.ArgPointer {
			argExprs = append(argExprs, "uintptr(unsafe.Pointer("+paramName+"))")
			imports["unsafe"] = "unsafe"
		} else {
			argExprs = append(argExprs, "uintptr("+paramName+")")
		}
	}

	// Resolve the return shape.
	returnContext := context
	returnContext.IsReturn = true
	returnResolved := g.mapper.GoType(&function.Return, returnContext, trialImports)
	model := view.FunctionModel{
		GoName:   function.Name,
		ParamStr: strings.Join(paramDecls, ", "),
		ProcVar:  "proc" + function.Name,
		ArgExprs: argExprs,
	}
	if !g.buildReturnShape(&model, function, returnResolved) {
		return view.FunctionModel{}, false
	}

	// Commit the trial imports plus the packages every body needs.
	for alias, path := range trialImports {
		imports[alias] = path
	}
	imports["syscall"] = "syscall"
	imports["win32"] = g.mapper.ModulePath + "/bindings/runtime/win32"

	model.CommentLines = functionComments(function)
	return model, true
}

// buildReturnShape selects the body/return template shape.
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
		if resolved.GoType == "float32" || resolved.GoType == "float64" {
			g.diag("function %s: float return not marshalable, function skipped", function.Name)
			return false
		}
		if resolved.GoType == "bool" {
			g.diag("function %s: bool return not marshalable, function skipped", function.Name)
			return false
		}
	}

	model.RetExpr = returnConversion(resolved)

	// BOOL + SetLastError collapses to `error` (the BOOL carries nothing
	// beyond success/failure).
	if function.SetLastError && resolved.TypedefApi == "Foundation" && resolved.TypedefName == "BOOL" {
		model.ReturnKind = view.RetBoolErr
		model.ReturnSig = "error"
		return true
	}

	if !function.SetLastError {
		model.ReturnKind = view.RetVal
		model.ReturnSig = resolved.GoType
		return true
	}

	// SetLastError with a known failure sentinel → clean (T, error).
	if checks := g.failureChecks(resolved); len(checks) > 0 {
		model.ReturnKind = view.RetValErr
		model.ReturnSig = "(" + resolved.GoType + ", error)"
		model.FailureChecks = checks
		return true
	}

	// SetLastError with no derivable sentinel: err is advisory (the raw
	// GetLastError value; consult it only when ret indicates failure).
	model.ReturnKind = view.RetValLast
	model.ReturnSig = "(" + resolved.GoType + ", error)"
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
func functionComments(function *win32meta.Function) []string {
	var lines []string
	first := fmt.Sprintf("%s calls %s!%s.", function.Name, strings.TrimSuffix(function.DLL, ".dll"), exportOf(function))
	lines = append(lines, first)
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
