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

// buildInterfaceModels transforms each raw COM interface into an idiomatic
// wrapper: a struct holding the raw pointer, embedding its idiomatic base,
// with HRESULT→error methods and Go string/bool inputs.
func (g *Generator) buildInterfaceModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.InterfaceModel {
	emittedByInterface := g.emittedComMethods
	if len(meta.Interfaces) == 0 {
		return nil
	}
	rawAlias := naming.ImportAlias(meta.Namespace)

	names := make([]string, 0, len(meta.Interfaces))
	for name := range meta.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)

	var models []view.InterfaceModel
	for _, name := range names {
		goName := naming.Export(name)
		emittedMethods := emittedByInterface[meta.Namespace+"\x00"+goName]
		if emittedMethods == nil {
			continue // the raw tier skipped this interface entirely
		}
		if !g.claimName(goName) || !g.claimName("Wrap"+goName) {
			g.diag("idiomatic interface %s: name already used in %s", name, meta.Namespace)
			continue
		}
		comInterface := meta.Interfaces[name]
		models = append(models, g.buildInterface(meta, name, goName, &comInterface, emittedMethods, rawAlias, imports))
	}
	if len(models) > 0 {
		imports[rawAlias] = g.rawImportPath(meta.Namespace)
	}
	return models
}

func (g *Generator) buildInterface(meta *win32meta.NamespaceMeta, name, goName string, comInterface *win32meta.ComInterface, emittedMethods map[int]string, rawAlias string, imports typemap.ImportSet) view.InterfaceModel {
	model := view.InterfaceModel{
		TypeName:     goName,
		RawType:      rawAlias + "." + goName,
		CommentLines: []string{fmt.Sprintf("%s is an idiomatic wrapper over the raw COM interface %s.%s with error-returning methods.", goName, meta.Namespace, name)},
	}

	// Base embedding mirrors the raw tier exactly: embed only when the raw
	// interface embedded its base (base resolvable and edge not severed).
	if comInterface.BaseInterface != "" {
		baseApi := comInterface.BaseInterfaceApi
		blocked := baseApi != meta.Namespace && g.mapper.Blocked[meta.Namespace][baseApi]
		if g.registry.Interface(baseApi, comInterface.BaseInterface) == nil {
			blocked = true
		}
		if !blocked {
			baseGoName := naming.Export(comInterface.BaseInterface)
			model.BaseRawField = baseGoName
			model.BaseFieldName = baseGoName
			if baseApi == meta.Namespace {
				model.BaseType = baseGoName
				model.BaseWrapCall = "Wrap" + baseGoName + "(&raw." + baseGoName + ")"
			} else {
				idiomAlias := naming.ImportAlias(baseApi) + "idiom"
				model.BaseType = idiomAlias + "." + baseGoName
				model.BaseWrapCall = idiomAlias + ".Wrap" + baseGoName + "(&raw." + baseGoName + ")"
				imports[idiomAlias] = g.idiomaticImportPath(baseApi)
			}
		}
	}

	// Own methods, in metadata order, using the raw tier's exact names.
	methodNames := map[string]bool{}
	for i := range comInterface.Methods {
		rawGoName, ok := emittedMethods[i]
		if !ok {
			continue // the raw tier skipped this method
		}
		method := &comInterface.Methods[i]
		methodModel, ok := g.buildComMethod(meta, method, rawGoName, imports)
		if !ok {
			continue
		}
		for methodNames[methodModel.GoName] {
			methodModel.GoName += "_"
		}
		methodNames[methodModel.GoName] = true
		model.Methods = append(model.Methods, methodModel)
	}
	return model
}

func (g *Generator) buildComMethod(meta *win32meta.NamespaceMeta, method *win32meta.ComMethod, rawGoName string, imports typemap.ImportSet) (view.InterfaceMethodModel, bool) {
	context := typemap.Context{Namespace: meta.Namespace, QualifyOwn: true}
	scratch := typemap.ImportSet{}

	returnContext := context
	returnContext.IsReturn = true
	returnResolved := g.mapper.GoType(&method.Return, returnContext, scratch)
	// RetVal elevation only makes sense when the status travels in the
	// HRESULT return (which becomes the trailing error).
	elevate := isHRESULT(returnResolved)

	var decls, preamble, rawArgs, returnValues, returnTypes []string
	for i := range method.Params {
		param := &method.Params[i]
		resolved := g.mapper.GoType(&param.Type, context, scratch)
		if elevate {
			if element, ok := retValElement(param, resolved); ok {
				local := "_" + naming.ParamName(param.Name)
				preamble = append(preamble, "var "+local+" "+element)
				rawArgs = append(rawArgs, "&"+local)
				returnValues = append(returnValues, local)
				returnTypes = append(returnTypes, element)
				continue
			}
		}
		idiomatic := g.idiomaticParam(param, resolved, i)
		if idiomatic.decl != "" {
			decls = append(decls, idiomatic.decl)
		}
		if idiomatic.preamble != "" {
			preamble = append(preamble, idiomatic.preamble)
		}
		rawArgs = append(rawArgs, idiomatic.rawArg)
	}

	model := view.InterfaceMethodModel{
		GoName:    naming.Export(method.Name),
		RawGoName: rawGoName,
		ParamStr:  strings.Join(decls, ", "),
		Preamble:  preamble,
		RawArgs:   rawArgs,
	}
	switch {
	case isHRESULT(returnResolved) && len(returnValues) > 0:
		model.Shape = view.FuncRetValError
		model.ReturnValues = returnValues
		model.ReturnSig = "(" + strings.Join(append(returnTypes, "error"), ", ") + ")"
		imports["win32"] = g.modulePath + "/bindings/runtime/win32"
	case isHRESULT(returnResolved):
		model.Shape = view.FuncErrorOnly
		model.ReturnSig = "error"
		imports["win32"] = g.modulePath + "/bindings/runtime/win32"
	case returnResolved.Kind == typemap.KindVoid:
		model.Shape = view.FuncPassthrough
	default:
		model.Shape = view.FuncPassthrough
		model.ReturnSig = returnResolved.GoType
	}

	if preambleUsesWin32(preamble) {
		imports["win32"] = g.modulePath + "/bindings/runtime/win32"
	}
	for alias, path := range scratch {
		imports[alias] = path
	}
	model.CommentLines = []string{fmt.Sprintf("%s wraps the raw %s call.", model.GoName, rawGoName)}
	return model, true
}

// idiomaticImportPath is the idiomatic package path for a namespace.
func (g *Generator) idiomaticImportPath(namespace string) string {
	return g.modulePath + "/opinionated/idiomatic/win32/" + naming.PackagePath(namespace)
}
