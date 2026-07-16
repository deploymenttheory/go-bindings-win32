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

// buildInterfaceModels converts a namespace's COM interfaces into vtable
// dispatch structs with idiomatic methods (HRESULT → error, [out,retval]
// lifted to return values). The interface struct IS the COM object — obtain
// one by casting a factory out-param; there is no wrapper.
func (g *Generator) buildInterfaceModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.InterfaceModel {
	names := make([]string, 0, len(meta.Interfaces))
	for name := range meta.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)

	models := make([]view.InterfaceModel, 0, len(names))
	for _, name := range names {
		goName := naming.Export(name)
		if !g.claimTypeName(goName) {
			g.diag("interface %s: name already used in package %s", name, meta.Namespace)
			continue
		}
		comInterface := meta.Interfaces[name]
		model, ok := g.buildInterface(meta, name, goName, &comInterface, imports)
		if !ok {
			continue
		}
		models = append(models, model)
	}
	return models
}

func (g *Generator) buildInterface(meta *win32meta.NamespaceMeta, name, goName string, comInterface *win32meta.ComInterface, imports typemap.ImportSet) (view.InterfaceModel, bool) {
	startSlot, ok := g.registry.VtableStartSlot(meta.Namespace, name)
	if !ok {
		g.diag("interface %s: unresolvable base chain, skipped", name)
		return view.InterfaceModel{}, false
	}
	model := view.InterfaceModel{
		TypeName: goName,
		DocURL:   comInterface.Availability.DocURL,
		GUID:     comInterface.GUID,
	}

	// IID constant.
	if comInterface.GUID != "" {
		literal, err := guidLiteral(comInterface.GUID)
		if err != nil {
			g.diag("interface %s: %v", name, err)
		} else {
			iidVar := "IID_" + goName
			if g.claimName(iidVar) {
				model.IIDVar = iidVar
				model.IIDLiteral = literal
				imports["win32"] = g.mapper.RuntimeImportPath()
			} else {
				g.diag("interface %s: IID name %s already used", name, iidVar)
			}
		}
	}

	// Base embedding promotes the base's methods; a severed or dangling base
	// demotes to a rootless vtable (slots stay correct, methods unpromoted).
	if comInterface.BaseInterface != "" {
		baseApi := comInterface.BaseInterfaceApi
		blocked := baseApi != meta.Namespace && g.mapper.Blocked[meta.Namespace][baseApi]
		if g.registry.Interface(baseApi, comInterface.BaseInterface) == nil {
			blocked = true
		}
		if blocked {
			model.BaseNote = fmt.Sprintf("Base interface %s.%s not embeddable here (import cycle); inherited methods are not promoted.",
				baseApi, comInterface.BaseInterface)
			g.diag("interface %s: base %s.%s not embedded", name, baseApi, comInterface.BaseInterface)
		} else {
			baseRef := win32meta.TypeRef{Kind: "ApiRef", Name: comInterface.BaseInterface, Api: baseApi, TargetKind: "Com"}
			resolved := g.mapper.GoType(&baseRef, typemap.Context{Namespace: meta.Namespace}, imports)
			model.BaseType = strings.TrimPrefix(resolved.GoType, "*")
		}
	}

	// Vtable methods, in metadata (vtable) order. Skipping a method never
	// shifts slots: indices are absolute per the metadata.
	methodNames := map[string]bool{}
	for i := range comInterface.Methods {
		method := &comInterface.Methods[i]
		slot := startSlot + i
		methodModel, ok := g.buildComMethod(meta, name, method, slot, imports)
		if !ok {
			continue
		}
		for methodNames[methodModel.GoName] {
			methodModel.GoName += "_"
		}
		methodNames[methodModel.GoName] = true
		model.Methods = append(model.Methods, methodModel)
	}
	return model, true
}

// buildComMethod resolves one vtable method into an idiomatic render model:
// Go-string/bool params, HRESULT → error, [out,retval] lifted to returns,
// dispatched through the vtable slot.
func (g *Generator) buildComMethod(meta *win32meta.NamespaceMeta, interfaceName string, method *win32meta.ComMethod, slot int, imports typemap.ImportSet) (view.ComMethodModel, bool) {
	context := typemap.Context{Namespace: meta.Namespace}
	scratch := typemap.ImportSet{}

	resolvedParams := make([]typemap.Resolved, len(method.Params))
	for i := range method.Params {
		resolvedParams[i] = g.mapper.GoType(&method.Params[i].Type, context, scratch)
	}
	retypeComOutParams(method.Params, resolvedParams, scratch, g.mapper.RuntimeImportPath())
	slicePlans, elidedCounts := planSliceParams(method.Params, resolvedParams, true)
	returnContext := context
	returnContext.IsReturn = true
	returnResolved := g.mapper.GoType(&method.Return, returnContext, scratch)
	// Elevation is viable when the status travels in the HRESULT return.
	elevate := isHRESULT(returnResolved)

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
	paramNames := make([]string, len(method.Params))
	for i := range method.Params {
		paramNames[i] = avoidCollision(naming.ParamName(method.Params[i].Name), reserved)
	}

	var decls, preamble, argWords, returnValues, returnTypes []string
	for i := range method.Params {
		param := &method.Params[i]
		resolved := resolvedParams[i]

		// The elevated out local is heap-escaped via win32.OutParam: a native
		// callee that reenters Go can move this goroutine's stack, stranding
		// any stack out-pointer — see runtime outparam.go.
		if elevate {
			if element, ok := retValElement(param, resolved); ok {
				local := "_" + paramNames[i]
				preamble = append(preamble, local+" := new("+element+")")
				argWords = append(argWords, "uintptr(win32.OutParam(unsafe.Pointer("+local+")))")
				returnValues = append(returnValues, "*"+local)
				returnTypes = append(returnTypes, element)
				continue
			}
		}
		// A count/size parameter collapsed into a slice: derive from len().
		if bufferIndex, ok := elidedCounts[i]; ok {
			argWords = append(argWords, "uintptr(len("+paramNames[bufferIndex]+"))")
			continue
		}
		// An array or buffer pointer collapsed into a []T / []byte parameter.
		if plan, ok := slicePlans[i]; ok {
			name := paramNames[i]
			local := "_" + name
			decls = append(decls, name+" []"+plan.element)
			preamble = append(preamble, "var "+local+" "+plan.rawPointerType)
			preamble = append(preamble, "if len("+name+") > 0 { "+local+" = &"+name+"[0] }")
			argWords = append(argWords, "uintptr(unsafe.Pointer("+local+"))")
			continue
		}
		decl, pre, word, _, ok := shapeParam(paramNames[i], param, resolved)
		if !ok {
			g.diag("interface %s: method %s param %s not marshalable (%s), method skipped",
				interfaceName, method.Name, param.Name, resolved.GoType)
			return view.ComMethodModel{}, false
		}
		if decl != "" {
			decls = append(decls, decl)
		}
		preamble = append(preamble, pre...)
		argWords = append(argWords, word)
	}

	model := view.ComMethodModel{
		GoName:   naming.Export(method.Name),
		ParamStr: strings.Join(decls, ", "),
		Slot:     slot,
		Preamble: preamble,
		ArgExprs: argWords,
	}
	switch {
	case isHRESULT(returnResolved) && len(returnValues) > 0:
		if informationalComMethods[meta.Namespace+"."+interfaceName+"."+method.Name] {
			g.informationalMatched[meta.Namespace+"."+interfaceName+"."+method.Name] = true
			g.diag("interface %s: method %s informational-success annotation not applied ([out,retval] elevation)",
				interfaceName, method.Name)
		}
		model.ReturnKind = view.RetRetValHResult
		model.ReturnValues = returnValues
		model.ReturnSig = "(" + strings.Join(append(returnTypes, "error"), ", ") + ")"
	case isHRESULT(returnResolved) && g.isInformationalComMethod(meta.Namespace, interfaceName, method.Name):
		// Curated informational-success methods additionally return the raw
		// HRESULT so S_FALSE-style codes survive.
		model.ReturnKind = view.RetHResultValueErr
		model.ReturnSig = "(win32.HRESULT, error)"
		scratch["win32"] = g.mapper.RuntimeImportPath()
	case isHRESULT(returnResolved):
		model.ReturnKind = view.RetHResultErr
		model.ReturnSig = "error"
	case returnResolved.Kind == typemap.KindVoid:
		model.ReturnKind = view.RetVoid
	case returnResolved.Kind == typemap.KindStruct, returnResolved.Kind == typemap.KindUnion,
		returnResolved.Kind == typemap.KindArray, returnResolved.Kind == typemap.KindGUID:
		g.diag("interface %s: method %s by-value %s return not marshalable, method skipped",
			interfaceName, method.Name, returnResolved.GoType)
		return view.ComMethodModel{}, false
	default:
		if returnResolved.GoType == "float32" || returnResolved.GoType == "float64" || returnResolved.GoType == "bool" {
			g.diag("interface %s: method %s %s return not marshalable, method skipped",
				interfaceName, method.Name, returnResolved.GoType)
			return view.ComMethodModel{}, false
		}
		model.ReturnKind = view.RetVal
		model.ReturnSig = returnResolved.GoType
		model.RetExpr = returnConversion(returnResolved)
	}

	for alias, path := range scratch {
		imports[alias] = path
	}
	model.CommentLines = []string{fmt.Sprintf("%s dispatches through %s's vtable slot %d.", model.GoName, interfaceName, slot)}
	if model.ReturnKind == view.RetHResultValueErr {
		model.CommentLines = append(model.CommentLines,
			"The returned HRESULT preserves informational successes (e.g. S_FALSE); the error is non-nil only on failure.")
	}
	return model, true
}
