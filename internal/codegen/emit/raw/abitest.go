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
	Size      uint32
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
	record := ABIRecord{Namespace: namespace, TypeName: typeName, Size: detailed.size}
	if modelFields != nil && len(modelFields) == len(detailed.offsets) {
		for i := range modelFields {
			record.Fields = append(record.Fields, ABIField{
				Name:   modelFields[i].Name,
				Offset: detailed.offsets[i],
			})
		}
	}
	g.abiRecords[namespace+"."+typeName] = record
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
// ("System.Threading" → "abi_system_threading_test.go").
func ABITestFileName(namespace string) string {
	return "abi_" + strings.ReplaceAll(naming.PackagePath(namespace), "/", "_") + "_test.go"
}

// BuildABITests renders one generated test file per namespace asserting the
// size and every field offset of every recorded struct: a package-level
// table of compile-time constants (unsafe.Sizeof / unsafe.Offsetof) that the
// hand-written checkABI driver in package abi walks. importPathFor maps a
// namespace to its bindings import path. The result maps file name → source.
func BuildABITests(records []ABIRecord, importPathFor func(namespace string) string) map[string]string {
	byNamespace := map[string][]ABIRecord{}
	var namespaces []string
	for _, record := range records {
		if _, seen := byNamespace[record.Namespace]; !seen {
			namespaces = append(namespaces, record.Namespace)
		}
		byNamespace[record.Namespace] = append(byNamespace[record.Namespace], record)
	}
	sort.Strings(namespaces)

	files := make(map[string]string, len(namespaces))
	for _, namespace := range namespaces {
		alias := naming.ImportAlias(namespace)
		ident := strings.ReplaceAll(namespace, ".", "_")
		var body strings.Builder
		fmt.Fprintf(&body, "// abi%s holds the expected C layout of every struct emitted for\n// %s%s.\n", ident, apiRootPrefix, namespace)
		fmt.Fprintf(&body, "var abi%s = []abiCase{\n", ident)
		for _, record := range byNamespace[namespace] {
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
		file.WriteString("//go:build " + fileasm.GeneratedBuildTag + "\n\n")
		file.WriteString("package abi\n\nimport (\n\t\"testing\"\n\t\"unsafe\"\n\n")
		fmt.Fprintf(&file, "\t%s %q\n)\n\n", alias, importPathFor(namespace))
		file.WriteString(body.String())
		files[ABITestFileName(namespace)] = file.String()
	}
	return files
}
