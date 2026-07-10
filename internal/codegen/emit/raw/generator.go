// Package rawwin emits the raw-tier Win32 bindings: one Go package per
// namespace under bindings/win32/, dispatching through syscall.SyscallN.
package rawwin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/render"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/shared/fileasm"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Generator emits the raw tier for a loaded Registry.
type Generator struct {
	registry *pipeline.Registry
	mapper   *typemap.Mapper
	// outDir is the bindings/win32 output root.
	outDir string

	// claimedNames tracks package-level identifiers in the namespace being
	// emitted, preventing collisions between enums/constants/functions.
	claimedNames map[string]bool
	// typeNames pre-claims all type names before any value names, so an
	// enum member or constant can never steal a name a type needs.
	typeNames map[string]bool

	// abiRecords collects expected struct layouts, keyed "Namespace.Type".
	abiRecords map[string]ABIRecord

	// writtenFiles records every path this run produced, so stale generated
	// files from earlier runs can be pruned afterwards.
	writtenFiles map[string]bool

	// emittedFunctions records which function wrappers each namespace
	// actually emitted (namespace → exported Go name), consumed by the
	// idiomatic tier so it never wraps a skipped function.
	emittedFunctions map[string]map[string]bool

	// emittedComMethods records the exact emitted Go name of each COM
	// method, keyed "namespace\x00Interface" → metadata-index → Go name, so
	// the idiomatic tier calls the right (deduped, non-skipped) raw method.
	emittedComMethods map[string]map[int]string

	// Diagnostics collects all degradations and skips (ratchet input).
	Diagnostics []string
}

// Mapper exposes the type mapper (blocked edges, skip set) so the idiomatic
// tier resolves types with identical degradation decisions.
func (g *Generator) Mapper() *typemap.Mapper { return g.mapper }

// EmittedFunctions returns namespace → set of emitted function Go names.
func (g *Generator) EmittedFunctions() map[string]map[string]bool {
	return g.emittedFunctions
}

// EmittedComMethods returns "namespace\x00Interface" → metadata-index → the
// emitted raw Go method name.
func (g *Generator) EmittedComMethods() map[string]map[int]string {
	return g.emittedComMethods
}

// New builds a Generator. Import cycles among namespaces are computed up
// front; references along severed edges degrade instead of importing.
func New(registry *pipeline.Registry, modulePath, outDir string) *Generator {
	return &Generator{
		registry: registry,
		mapper: &typemap.Mapper{
			Registry:   registry,
			ModulePath: modulePath,
			Blocked:    pipeline.ComputeBlockedImports(registry),
		},
		outDir: outDir,
	}
}

// EmitAll generates every loaded namespace (or, when filter is non-empty,
// the filter set plus the transitive closure of namespaces it references —
// generated packages must always compile). Returns the package count.
func (g *Generator) EmitAll(filter map[string]bool) (int, error) {
	g.computeSkippedTypes()
	// ABI records collected by the pre-pass may include structs the real
	// pass will skip; keep only real-pass records.
	g.abiRecords = map[string]ABIRecord{}
	g.writtenFiles = map[string]bool{}
	g.emittedFunctions = map[string]map[string]bool{}
	g.emittedComMethods = map[string]map[int]string{}
	emitted := map[string]bool{}
	pending := make([]string, 0, len(g.registry.Namespaces))
	if len(filter) > 0 {
		for namespace := range filter {
			pending = append(pending, namespace)
		}
		sort.Strings(pending)
	} else {
		for _, meta := range g.registry.Namespaces {
			pending = append(pending, meta.Namespace)
		}
	}

	for len(pending) > 0 {
		namespace := pending[0]
		pending = pending[1:]
		if emitted[namespace] {
			continue
		}
		meta := g.registry.ByNamespace[namespace]
		if meta == nil {
			return len(emitted), fmt.Errorf("referenced namespace %s not in loaded metadata (re-run ingest without a filter)", namespace)
		}
		emitted[namespace] = true
		if err := g.emitNamespace(meta); err != nil {
			return len(emitted), fmt.Errorf("emitting %s: %w", namespace, err)
		}
		// Chase namespaces this one referenced.
		var discovered []string
		for referenced := range g.mapper.Referenced {
			if !emitted[referenced] {
				discovered = append(discovered, referenced)
			}
		}
		sort.Strings(discovered)
		pending = append(pending, discovered...)
	}
	// Remove generated files from earlier runs that this run did not
	// rewrite (renamed constructs, removed namespaces). A filtered run
	// prunes only inside the packages it emitted; a full run sweeps the
	// whole output tree.
	if err := g.pruneStale(len(filter) == 0); err != nil {
		return len(emitted), err
	}
	g.Diagnostics = append(g.Diagnostics, g.mapper.Diagnostics...)
	return len(emitted), nil
}

// generatedHeader marks files this generator owns; only files carrying it
// are ever deleted.
const generatedHeader = "// Code generated by go-bindings-win32-codegen. DO NOT EDIT."

// pruneStale deletes generated .go files not written by this run, then
// removes directories left empty.
func (g *Generator) pruneStale(fullSweep bool) error {
	if _, err := os.Stat(g.outDir); err != nil {
		return nil // nothing emitted yet
	}
	emittedDirs := map[string]bool{}
	for path := range g.writtenFiles {
		emittedDirs[filepath.Dir(path)] = true
	}
	var emptied []string
	err := filepath.WalkDir(g.outDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			emptied = append(emptied, path)
			return nil
		}
		if !strings.HasSuffix(path, ".go") || g.writtenFiles[path] {
			return nil
		}
		if !fullSweep && !emittedDirs[filepath.Dir(path)] {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !strings.HasPrefix(string(content), generatedHeader) {
			return nil // never touch hand-written files
		}
		return os.Remove(path)
	})
	if err != nil {
		return err
	}
	// Deepest-first: removing empty leaves may empty their parents.
	sort.Sort(sort.Reverse(sort.StringSlice(emptied)))
	for _, dir := range emptied {
		if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return nil
}

// computeSkippedTypes dry-runs the struct gather over every loaded
// namespace to learn which types will not be emitted, so references to them
// degrade instead of naming an undefined type. Diagnostics from the dry run
// are discarded (the real pass re-records them).
func (g *Generator) computeSkippedTypes() {
	g.mapper.SkippedTypes = map[string]bool{}
	savedDiagnostics := len(g.Diagnostics)
	savedMapperDiagnostics := len(g.mapper.Diagnostics)
	// Skips cascade (a struct embedding a skipped struct may itself become
	// unemittable), so iterate to a fixed point. The set only grows, so
	// this converges; the bound is a safety net.
	for round := 0; round < 10; round++ {
		grew := false
		for _, meta := range g.registry.Namespaces {
			g.prepareNamespaceClaims(meta)
			scratch := typemap.ImportSet{}
			emittedTypes := map[string]bool{}
			for _, model := range g.buildStructModels(meta, scratch) {
				emittedTypes[model.TypeName] = true
			}
			for name := range meta.Structs {
				exported := naming.Export(name)
				key := meta.Namespace + "." + exported
				if !emittedTypes[exported] && !g.mapper.SkippedTypes[key] {
					g.mapper.SkippedTypes[key] = true
					grew = true
				}
			}
		}
		if !grew {
			break
		}
	}
	g.Diagnostics = g.Diagnostics[:savedDiagnostics]
	g.mapper.Diagnostics = g.mapper.Diagnostics[:savedMapperDiagnostics]
}

func (g *Generator) emitNamespace(meta *win32meta.NamespaceMeta) error {
	g.prepareNamespaceClaims(meta)
	packageName := naming.PackageName(meta.Namespace)
	packageDir := filepath.Join(g.outDir, filepath.FromSlash(naming.PackagePath(meta.Namespace)))

	// Types file: typedefs, enums, structs, delegates.
	typesImports := typemap.ImportSet{}
	var typesBody strings.Builder
	for _, model := range g.buildTypedefModels(meta, typesImports) {
		if err := renderInto(&typesBody, render.Typedef, model); err != nil {
			return err
		}
	}
	for _, model := range g.buildEnumModels(meta) {
		if err := renderInto(&typesBody, render.Enum, model); err != nil {
			return err
		}
	}
	for _, model := range g.buildStructModels(meta, typesImports) {
		if err := renderInto(&typesBody, render.Struct, model); err != nil {
			return err
		}
	}
	for _, model := range g.buildDelegateModels(meta, typesImports) {
		if err := renderInto(&typesBody, render.Delegate, model); err != nil {
			return err
		}
	}
	if err := g.writeFile(packageDir, packageName+"_types.go", packageName, typesImports, typesBody.String()); err != nil {
		return err
	}

	// COM interfaces file.
	interfaceImports := typemap.ImportSet{}
	var interfaceBody strings.Builder
	for _, model := range g.buildInterfaceModels(meta, interfaceImports) {
		if err := renderInto(&interfaceBody, render.Interface, model); err != nil {
			return err
		}
	}
	if err := g.writeFile(packageDir, packageName+"_interfaces.go", packageName, interfaceImports, interfaceBody.String()); err != nil {
		return err
	}

	// Constants file.
	constImports := typemap.ImportSet{}
	var constBody strings.Builder
	for _, model := range g.buildConstantModels(meta, constImports) {
		if err := renderInto(&constBody, render.Constant, model); err != nil {
			return err
		}
	}
	if err := g.writeFile(packageDir, packageName+"_constants.go", packageName, constImports, constBody.String()); err != nil {
		return err
	}

	// Functions file: DLL/proc declarations plus wrappers.
	funcImports := typemap.ImportSet{}
	functions, dlls := g.buildFunctionModels(meta, funcImports)
	var funcBody strings.Builder
	if len(dlls) > 0 {
		block, err := render.DLL(dlls)
		if err != nil {
			return err
		}
		funcBody.WriteString(block)
	}
	for _, model := range functions {
		if err := renderInto(&funcBody, render.Function, model); err != nil {
			return err
		}
	}
	if err := g.writeFile(packageDir, packageName+"_functions.go", packageName, funcImports, funcBody.String()); err != nil {
		return err
	}

	// Package doc: written directly because the package comment must sit
	// above the package clause, which fileasm's scaffold doesn't model.
	doc := fmt.Sprintf(
		"%s\n\n//go:build %s\n\n// Package %s binds the Windows.Win32.%s API surface.\npackage %s\n",
		generatedHeader, fileasm.GeneratedBuildTag, packageName, meta.Namespace, packageName)
	docPath := filepath.Join(packageDir, "doc.go")
	g.writtenFiles[docPath] = true
	return writeRawFile(docPath, []byte(doc))
}

// renderInto appends one rendered construct to the file body.
func renderInto[T any](body *strings.Builder, renderFunc func(T) (string, error), model T) error {
	block, err := renderFunc(model)
	if err != nil {
		return err
	}
	body.WriteString(block)
	return nil
}

// writeFile prunes unused imports (a resolution may have registered an
// import that a later skip made unnecessary) and assembles the file. Empty
// bodies produce no file.
func (g *Generator) writeFile(dir, fileName, packageName string, imports typemap.ImportSet, body string) error {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	// Import usage is detected against code only — doc comments mention
	// qualified types ("shape func(foundation.HANDLE)") without using them.
	code := stripComments(body)
	pruned := map[string]string{}
	for alias, path := range imports {
		if referencesAlias(code, alias) {
			pruned[alias] = path
		}
	}
	// Bodies reference these by fixed name; detect rather than track.
	if referencesAlias(code, "unsafe") {
		pruned["unsafe"] = "unsafe"
	}
	if referencesAlias(code, "syscall") {
		pruned["syscall"] = "syscall"
	}
	if referencesAlias(code, "win32") {
		pruned["win32"] = g.mapper.ModulePath + "/bindings/runtime/win32"
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

// referencesAlias reports whether code uses the import alias as a package
// qualifier (`alias.`), requiring a word boundary so a shorter alias is not
// falsely matched inside a longer one (e.g. "imaging" in "winrtimaging.").
func referencesAlias(code, alias string) bool {
	needle := alias + "."
	for from := 0; ; {
		index := strings.Index(code[from:], needle)
		if index < 0 {
			return false
		}
		position := from + index
		if position == 0 || !isIdentByte(code[position-1]) {
			return true
		}
		from = position + 1
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// stripComments removes //-comment text from a body so import-usage scans
// only see code.
func stripComments(body string) string {
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if index := strings.Index(line, "//"); index >= 0 {
			line = line[:index]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// writeRawFile writes pre-formatted content, creating parent directories.
func writeRawFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// prepareNamespaceClaims resets the per-namespace name state and pre-claims
// every top-level type name (types win any type-vs-value collision).
func (g *Generator) prepareNamespaceClaims(meta *win32meta.NamespaceMeta) {
	g.claimedNames = map[string]bool{}
	g.typeNames = map[string]bool{}
	claimType := func(name string) {
		exported := naming.Export(name)
		if !g.claimedNames[exported] {
			g.claimedNames[exported] = true
			g.typeNames[exported] = true
		}
	}
	for name := range meta.Typedefs {
		claimType(name)
	}
	for name := range meta.Enums {
		claimType(name)
	}
	for name := range meta.Structs {
		claimType(name)
	}
	for name := range meta.Delegates {
		claimType(name)
	}
	for name := range meta.Interfaces {
		claimType(name)
	}
}

// claimTypeName consumes a pre-claimed type name; false when the type lost
// its pre-claim to an earlier same-named type.
func (g *Generator) claimTypeName(name string) bool {
	if !g.typeNames[name] {
		return false
	}
	delete(g.typeNames, name) // consumed: a second same-named type is a dupe
	return true
}

// claimName reserves a package-level identifier for a value (enum member,
// constant, function) or nested type; false when already used.
func (g *Generator) claimName(name string) bool {
	if g.claimedNames[name] {
		return false
	}
	g.claimedNames[name] = true
	return true
}

// unclaimName releases a reservation after a construct is skipped.
func (g *Generator) unclaimName(name string) {
	delete(g.claimedNames, name)
}

// diag records one diagnostic.
func (g *Generator) diag(format string, args ...any) {
	g.Diagnostics = append(g.Diagnostics, fmt.Sprintf(format, args...))
}
