package rawwin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/shared/fileasm"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// ABIRecord captures one emitted struct's expected C layout for the
// generated acceptance test: unsafe.Sizeof/Offsetof must reproduce it.
type ABIRecord struct {
	Namespace string
	TypeName  string
	// Arch is "" for the layout amd64 and arm64 share, else the architecture
	// this layout is specific to (the record's test file is tagged with it).
	Arch string
	Size uint32
	// Fields is nil for union blobs (size-only assertion).
	Fields []ABIField
}

// ABIField is one field's expected offset.
type ABIField struct {
	Name   string
	Offset uint32
}

// recordABI stores the expected layout of an emitted struct. modelFields is
// nil for unions. Records are keyed so the skip pre-pass (which also runs
// the struct gather) stays idempotent.
func (g *Generator) recordABI(namespace, typeName string, definition *win32meta.Struct, modelFields []view.StructFieldModel) {
	if g.abiRecords == nil {
		g.abiRecords = map[string]ABIRecord{}
	}
	detailed := g.structLayoutOf(definition, true)
	if !detailed.ok || detailed.size == 0 {
		return
	}
	record := ABIRecord{Namespace: namespace, TypeName: typeName, Arch: g.layoutArch, Size: detailed.size}
	if modelFields != nil && len(modelFields) == len(detailed.offsets) {
		for i := range modelFields {
			record.Fields = append(record.Fields, ABIField{
				Name:   modelFields[i].Name,
				Offset: detailed.offsets[i],
			})
		}
	}
	key := namespace + "." + typeName
	if record.Arch != "" {
		key += "@" + record.Arch
	}
	g.abiRecords[key] = record
}

// ABIRecords returns the collected layouts sorted by key.
func (g *Generator) ABIRecords() []ABIRecord {
	keys := make([]string, 0, len(g.abiRecords))
	for key := range g.abiRecords {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	records := make([]ABIRecord, 0, len(keys))
	for _, key := range keys {
		records = append(records, g.abiRecords[key])
	}
	return records
}

// ImportPathFor exposes the mapper's namespace → import-path mapping (for
// BuildABITests callers).
func (g *Generator) ImportPathFor(namespace string) string {
	return g.mapper.ImportPathFor(namespace)
}

// ABITestFileName is the generated test file for a namespace's layouts
// ("System.Threading" → "abi_system_threading_test.go"; an architecture-
// specific file adds the GOARCH suffix, "abi_system_kernel_arm64_test.go").
func ABITestFileName(namespace, arch string) string {
	name := "abi_" + strings.ReplaceAll(naming.PackagePath(namespace), "/", "_")
	if arch != "" {
		name += "_" + arch
	}
	return name + "_test.go"
}

// BuildABITests renders one generated test file per namespace asserting the
// size and every field offset of every recorded struct: a package-level
// table of compile-time constants (unsafe.Sizeof / unsafe.Offsetof) that the
// hand-written checkABI driver in package abi walks. importPathFor maps a
// namespace to its bindings import path. The result maps file name → source.
func BuildABITests(records []ABIRecord, importPathFor func(namespace string) string) map[string]string {
	type group struct{ namespace, arch string }
	byGroup := map[group][]ABIRecord{}
	var groups []group
	for _, record := range records {
		key := group{record.Namespace, record.Arch}
		if _, seen := byGroup[key]; !seen {
			groups = append(groups, key)
		}
		byGroup[key] = append(byGroup[key], record)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].namespace != groups[j].namespace {
			return groups[i].namespace < groups[j].namespace
		}
		return groups[i].arch < groups[j].arch
	})

	files := make(map[string]string, len(groups))
	for _, key := range groups {
		namespace := key.namespace
		alias := naming.ImportAlias(namespace)
		ident := strings.ReplaceAll(namespace, ".", "_")
		buildTag, scope := fileasm.GeneratedBuildTag, "every struct"
		if key.arch != "" {
			ident += "_" + key.arch
			buildTag = "windows && " + key.arch
			scope = "every " + key.arch + "-specific struct"
		}
		var body strings.Builder
		fmt.Fprintf(&body, "// abi%s holds the expected C layout of %s emitted for\n// %s%s.\n", ident, scope, apiRootPrefix, namespace)
		fmt.Fprintf(&body, "var abi%s = []abiCase{\n", ident)
		for _, record := range byGroup[key] {
			qualified := alias + "." + record.TypeName
			fmt.Fprintf(&body, "\t{%q, unsafe.Sizeof(%s{}), %d},\n", qualified+" size", qualified, record.Size)
			for _, field := range record.Fields {
				fmt.Fprintf(&body, "\t{%q, unsafe.Offsetof(%s{}.%s), %d},\n",
					qualified+"."+field.Name, qualified, field.Name, field.Offset)
			}
		}
		body.WriteString("}\n\n")
		fmt.Fprintf(&body, "func TestABI_%s(t *testing.T) { checkABI(t, abi%s) }\n", ident, ident)

		var file strings.Builder
		file.WriteString(generatedHeader + "\n\n")
		file.WriteString("//go:build " + buildTag + "\n\n")
		file.WriteString("package abi\n\nimport (\n\t\"testing\"\n\t\"unsafe\"\n\n")
		fmt.Fprintf(&file, "\t%s %q\n)\n\n", alias, importPathFor(namespace))
		file.WriteString(body.String())
		files[ABITestFileName(namespace, key.arch)] = file.String()
	}
	return files
}
