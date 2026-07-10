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
// dispatch wrappers.
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
				imports["win32"] = g.mapper.ModulePath + "/bindings/runtime/win32"
			} else {
				g.diag("interface %s: IID name %s already used", name, iidVar)
			}
		}
	}

	// Base embedding: a severed or dangling base demotes the wrapper to a
	// rootless vtable (slots stay correct; inherited methods unpromoted).
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

func (g *Generator) buildComMethod(meta *win32meta.NamespaceMeta, interfaceName string, method *win32meta.ComMethod, slot int, imports typemap.ImportSet) (view.ComMethodModel, bool) {
	context := typemap.Context{Namespace: meta.Namespace}
	trialImports := typemap.ImportSet{}

	var paramDecls, argExprs []string
	for i := range method.Params {
		param := &method.Params[i]
		resolved := g.mapper.GoType(&param.Type, context, trialImports)
		argClass := typemap.ArgClassOf(resolved, resolved.GoType)
		if argClass == typemap.ArgUnsupported || resolved.Kind == typemap.KindVoid {
			g.diag("interface %s: method %s param %s not marshalable (%s), method skipped",
				interfaceName, method.Name, param.Name, resolved.GoType)
			return view.ComMethodModel{}, false
		}
		paramName := naming.ParamName(param.Name)
		paramDecls = append(paramDecls, paramName+" "+resolved.GoType)
		if argClass == typemap.ArgPointer {
			argExprs = append(argExprs, "uintptr(unsafe.Pointer("+paramName+"))")
		} else {
			argExprs = append(argExprs, "uintptr("+paramName+")")
		}
	}

	returnContext := context
	returnContext.IsReturn = true
	returnResolved := g.mapper.GoType(&method.Return, returnContext, trialImports)
	model := view.ComMethodModel{
		GoName:   naming.Export(method.Name),
		ParamStr: strings.Join(paramDecls, ", "),
		Slot:     slot,
		ArgExprs: argExprs,
	}
	switch returnResolved.Kind {
	case typemap.KindVoid:
		model.ReturnKind = view.RetVoid
	case typemap.KindStruct, typemap.KindUnion, typemap.KindArray, typemap.KindGUID:
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

	for alias, path := range trialImports {
		imports[alias] = path
	}
	model.CommentLines = []string{fmt.Sprintf("%s dispatches through %s's vtable slot %d.", model.GoName, interfaceName, slot)}
	return model, true
}
