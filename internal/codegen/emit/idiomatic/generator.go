// Package idiowin emits the idiomatic tier: ergonomic Go wrappers over the
// raw bindings (opinionated/idiomatic/win32/). It never imports
// bindings/win32; it calls the raw packages and adapts their signatures —
// Go strings for PWSTR, bool for BOOL, error for HRESULT/SetLastError,
// elided reserved params, and W-desuffixed names.
package idiowin

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/idiomatic/render"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/shared/fileasm"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Generator emits the idiomatic tier.
type Generator struct {
	registry *pipeline.Registry
	mapper   *typemap.Mapper
	// emittedFunctions is the raw tier's namespace → emitted-name set; the
	// idiomatic tier only wraps functions the raw tier actually produced.
	emittedFunctions map[string]map[string]bool
	// emittedComMethods is the raw tier's "namespace\x00Interface" →
	// metadata-index → emitted raw method name, so the idiomatic COM
	// wrapper calls the exact (deduped, non-skipped) raw method.
	emittedComMethods map[string]map[int]string
	// reExports is the raw tier's per-namespace top-level identifiers the
	// idiomatic package re-exports when it does not define them itself.
	reExports map[string][]ReExport

	modulePath string
	outDir     string

	claimedNames map[string]bool
	writtenFiles map[string]bool

	Diagnostics []string
}

// New builds an idiomatic Generator sharing the raw tier's mapper (so type
// degradation decisions match exactly) and its emitted-function set.
func New(registry *pipeline.Registry, mapper *typemap.Mapper, emittedFunctions map[string]map[string]bool, emittedComMethods map[string]map[int]string, reExports map[string][]ReExport, modulePath, outDir string) *Generator {
	return &Generator{
		registry:          registry,
		mapper:            mapper,
		emittedFunctions:  emittedFunctions,
		emittedComMethods: emittedComMethods,
		reExports:         reExports,
		modulePath:        modulePath,
		outDir:            outDir,
	}
}

// EmitAll generates the idiomatic tier. When filter is non-nil, only those
// namespaces are emitted (used to mirror a filtered raw run exactly); nil
// emits every namespace. Returns the package count.
func (g *Generator) EmitAll(filter map[string]bool) (int, error) {
	g.writtenFiles = map[string]bool{}
	written := 0
	for _, meta := range g.registry.Namespaces {
		if filter != nil && !filter[meta.Namespace] {
			continue
		}
		emitted, err := g.emitNamespace(meta)
		if err != nil {
			return written, fmt.Errorf("idiomatic %s: %w", meta.Namespace, err)
		}
		if emitted {
			written++
		}
	}
	// A full run sweeps the whole idiomatic tree; a filtered run prunes only
	// inside the packages it (re)emitted.
	if err := g.pruneStale(filter == nil); err != nil {
		return written, err
	}
	g.Diagnostics = append(g.Diagnostics, g.mapper.Diagnostics...)
	return written, nil
}

func (g *Generator) emitNamespace(meta *win32meta.NamespaceMeta) (bool, error) {
	g.claimedNames = map[string]bool{}
	packageName := naming.PackageName(meta.Namespace)
	packageDir := filepath.Join(g.outDir, filepath.FromSlash(naming.PackagePath(meta.Namespace)))

	// Interface wrapper type names claim before function names so a function
	// can never shadow a wrapper's name.
	interfaceImports := typemap.ImportSet{}
	functionImports := typemap.ImportSet{}
	interfaceModels := g.buildInterfaceModels(meta, interfaceImports)
	functionModels := g.buildFunctionModels(meta, functionImports)
	// Handle closers claim names after interfaces and functions so a
	// Close<Handle> helper can never shadow one of them.
	handleImports := typemap.ImportSet{}
	handleBody := g.buildHandleClosers(meta, handleImports)
	// Re-exports come last so wrappers/improved functions win any name
	// clash; they make the idiomatic package self-contained and are grouped
	// into the same files the raw tier uses (types / constants / functions).
	reExports := g.buildReExports(meta)
	if len(functionModels) == 0 && len(interfaceModels) == 0 && handleBody == "" &&
		reExports.types == "" && reExports.constants == "" && reExports.functions == "" {
		return false, nil
	}

	if len(interfaceModels) > 0 {
		var body strings.Builder
		for _, model := range interfaceModels {
			block, err := render.Interface(model)
			if err != nil {
				return false, err
			}
			body.WriteString(block)
		}
		if err := g.writeFile(packageDir, packageName+"_interfaces.go", packageName, interfaceImports, body.String()); err != nil {
			return false, err
		}
	}

	// Functions file: idiomatic wrappers plus any pass-through re-exports.
	if len(functionModels) > 0 || reExports.functions != "" {
		var body strings.Builder
		for _, model := range functionModels {
			block, err := render.Function(model)
			if err != nil {
				return false, err
			}
			body.WriteString(block)
		}
		if reExports.functions != "" {
			functionImports[reExports.rawAlias] = g.rawImportPath(meta.Namespace)
			body.WriteString(reExports.functions)
		}
		if err := g.writeFile(packageDir, packageName+"_functions.go", packageName, functionImports, body.String()); err != nil {
			return false, err
		}
	}

	if handleBody != "" {
		if err := g.writeFile(packageDir, packageName+"_handles.go", packageName, handleImports, handleBody); err != nil {
			return false, err
		}
	}

	// Re-exported types and constants mirror the raw tier's file names.
	if reExports.types != "" {
		imports := typemap.ImportSet{reExports.rawAlias: g.rawImportPath(meta.Namespace)}
		if err := g.writeFile(packageDir, packageName+"_types.go", packageName, imports, reExports.types); err != nil {
			return false, err
		}
	}
	if reExports.constants != "" {
		imports := typemap.ImportSet{reExports.rawAlias: g.rawImportPath(meta.Namespace)}
		if err := g.writeFile(packageDir, packageName+"_constants.go", packageName, imports, reExports.constants); err != nil {
			return false, err
		}
	}

	doc := fmt.Sprintf(
		"%s\n\n//go:build %s\n\n// Package %s provides idiomatic Go wrappers over the raw Windows.Win32.%s bindings.\npackage %s\n",
		generatedHeader, fileasm.GeneratedBuildTag, packageName, meta.Namespace, packageName)
	docPath := filepath.Join(packageDir, "doc.go")
	g.writtenFiles[docPath] = true
	return true, writeRawFile(docPath, []byte(doc))
}

const generatedHeader = "// Code generated by go-bindings-win32-codegen. DO NOT EDIT."

// rawImportPath is the raw package path for a namespace.
func (g *Generator) rawImportPath(namespace string) string {
	return g.modulePath + "/bindings/win32/" + naming.PackagePath(namespace)
}

func (g *Generator) writeFile(dir, fileName, packageName string, imports typemap.ImportSet, body string) error {
	// Import usage is detected against code only — doc comments mention
	// qualified type names (e.g. a wrapped interface) without using them.
	code := stripComments(body)
	pruned := map[string]string{}
	for alias, path := range imports {
		if referencesAlias(code, alias) {
			pruned[alias] = path
		}
	}
	if referencesAlias(code, "unsafe") {
		pruned["unsafe"] = "unsafe"
	}
	path := filepath.Join(dir, fileName)
	g.writtenFiles[path] = true
	return fileasm.WriteGoFile(path, fileasm.File{
		PackageName: packageName,
		BuildTag:    fileasm.GeneratedBuildTag,
		Imports:     pruned,
		Body:        body,
	})
}

func (g *Generator) claimName(name string) bool {
	if g.claimedNames[name] {
		return false
	}
	g.claimedNames[name] = true
	return true
}

func (g *Generator) diag(format string, args ...any) {
	g.Diagnostics = append(g.Diagnostics, fmt.Sprintf(format, args...))
}

// pruneStale removes idiomatic generated files not written this run. A full
// sweep cleans the whole tree; otherwise only directories this run wrote into.
func (g *Generator) pruneStale(fullSweep bool) error {
	return pruneGeneratedTree(g.outDir, g.writtenFiles, fullSweep)
}
