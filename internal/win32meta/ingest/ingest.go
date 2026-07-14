// Package ingest projects a parsed Windows.Win32.winmd into the win32meta
// IR: one NamespaceMeta per Windows.Win32 namespace.
package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
	"github.com/deploymenttheory/go-winmd"
)

// namespacePrefix scopes ingestion to the Win32 projection; the attribute
// definitions namespace is metadata plumbing and never emitted.
const (
	namespacePrefix    = "Windows.Win32."
	metadataNamespace  = "Windows.Win32.Foundation.Metadata"
	interopNamespace   = "Windows.Win32.Interop"
)

// Options parameterizes a projection run. The zero value reproduces the
// Win32 projection exactly; sister generators (wdk) set these to project a
// different assembly root and to resolve cross-assembly references.
type Options struct {
	// ProjectPrefix selects which TypeDefs this run projects
	// (default namespacePrefix, "Windows.Win32.").
	ProjectPrefix string
	// ApiName maps a full CLR namespace to its IR Api key ("" = not
	// projectable). Default: strip ProjectPrefix; other roots unmapped.
	ApiName func(fullNamespace string) string
	// ExcludedNamespaces are metadata-plumbing namespaces never emitted
	// (default: the Win32 attribute/interop namespaces).
	ExcludedNamespaces []string
	// ExtraKinds seeds kindIndex with externally-classified TypeDefs
	// (full-name keys), for cross-assembly TypeRef targeting.
	ExtraKinds map[string]string
}

// withDefaults fills unset fields with the Win32 projection behavior.
func (o Options) withDefaults() Options {
	if o.ProjectPrefix == "" {
		o.ProjectPrefix = namespacePrefix
	}
	if o.ApiName == nil {
		prefix := o.ProjectPrefix
		o.ApiName = func(fullNamespace string) string {
			if !strings.HasPrefix(fullNamespace, prefix) {
				return ""
			}
			return strings.TrimPrefix(fullNamespace, prefix)
		}
	}
	if o.ExcludedNamespaces == nil {
		o.ExcludedNamespaces = []string{metadataNamespace, interopNamespace}
	}
	return o
}

// Ingester projects a winmd file into NamespaceMeta values.
type Ingester struct {
	file         *winmd.File
	winmdVersion string
	opts         Options

	// kindIndex classifies every Windows.Win32 TypeDef by full name
	// ("Namespace.Name") → TargetKind string.
	kindIndex map[string]string
	// implMapIndex maps 1-based MethodDef rows → ImplMap rows.
	implMapIndex map[uint32]*winmd.ImplMapRow
	// constantIndex maps 1-based Field rows → Constant rows.
	constantIndex map[uint32]*winmd.ConstantRow
	// classLayoutIndex maps 1-based TypeDef rows → ClassLayout rows.
	classLayoutIndex map[uint32]*winmd.ClassLayoutRow
	// nestedIndex maps enclosing 1-based TypeDef rows → nested rows.
	nestedIndex map[uint32][]uint32
	// nestedSet marks TypeDef rows that are nested inside another type.
	nestedSet map[uint32]bool
	// interfaceImplIndex maps 1-based TypeDef rows → implemented interfaces.
	interfaceImplIndex map[uint32][]winmd.CodedIndex

	// Diagnostics collects non-fatal projection notes.
	Diagnostics []string
}

// New builds an Ingester over a parsed winmd file with the default (Win32)
// projection options.
func New(file *winmd.File, winmdVersion string) *Ingester {
	return NewWithOptions(file, winmdVersion, Options{})
}

// NewWithOptions builds an Ingester with explicit projection options.
func NewWithOptions(file *winmd.File, winmdVersion string, opts Options) *Ingester {
	ingester := &Ingester{file: file, winmdVersion: winmdVersion, opts: opts.withDefaults()}
	ingester.buildIndices()
	return ingester
}

// KindIndex exposes the full-name → TargetKind classification of this file's
// TypeDefs, for seeding a sister assembly's ingest (Options.ExtraKinds).
func (in *Ingester) KindIndex() map[string]string {
	return in.kindIndex
}

// Ingest projects every Windows.Win32 namespace, sorted by namespace name.
func (in *Ingester) Ingest() ([]*win32meta.NamespaceMeta, error) {
	byNamespace := map[string]*win32meta.NamespaceMeta{}
	tables := &in.file.Tables

	for typeDefRow := range tables.TypeDefs {
		typeDef := &tables.TypeDefs[typeDefRow]
		row := uint32(typeDefRow + 1)
		if !in.isProjectedNamespace(typeDef.Namespace) || in.nestedSet[row] {
			continue
		}
		shortNamespace := in.opts.ApiName(typeDef.Namespace)
		meta := byNamespace[shortNamespace]
		if meta == nil {
			meta = &win32meta.NamespaceMeta{
				Namespace:     shortNamespace,
				SchemaVersion: win32meta.CurrentSchemaVersion,
				WinmdVersion:  in.winmdVersion,
				Structs:       map[string]win32meta.Struct{},
				Enums:         map[string]win32meta.Enum{},
				Interfaces:    map[string]win32meta.ComInterface{},
				Delegates:     map[string]win32meta.FuncPointer{},
				Typedefs:      map[string]win32meta.Typedef{},
			}
			byNamespace[shortNamespace] = meta
		}
		if err := in.projectTypeDef(meta, typeDef, row); err != nil {
			return nil, fmt.Errorf("projecting %s.%s: %w", typeDef.Namespace, typeDef.Name, err)
		}
	}

	namespaces := make([]*win32meta.NamespaceMeta, 0, len(byNamespace))
	for _, meta := range byNamespace {
		sort.Slice(meta.Functions, func(i, j int) bool { return meta.Functions[i].Name < meta.Functions[j].Name })
		sort.Slice(meta.Constants, func(i, j int) bool { return meta.Constants[i].Name < meta.Constants[j].Name })
		namespaces = append(namespaces, meta)
	}
	sort.Slice(namespaces, func(i, j int) bool { return namespaces[i].Namespace < namespaces[j].Namespace })
	return namespaces, nil
}

func (in *Ingester) isProjectedNamespace(namespace string) bool {
	if !strings.HasPrefix(namespace, in.opts.ProjectPrefix) {
		return false
	}
	for _, excluded := range in.opts.ExcludedNamespaces {
		if namespace == excluded {
			return false
		}
	}
	return true
}

// buildIndices precomputes the lookup tables used during projection.
func (in *Ingester) buildIndices() {
	tables := &in.file.Tables

	in.implMapIndex = make(map[uint32]*winmd.ImplMapRow, len(tables.ImplMaps))
	for i := range tables.ImplMaps {
		implMap := &tables.ImplMaps[i]
		if implMap.MemberForwarded.Table == winmd.TableMethodDef {
			in.implMapIndex[implMap.MemberForwarded.Row] = implMap
		}
	}

	in.constantIndex = make(map[uint32]*winmd.ConstantRow, len(tables.Constants))
	for i := range tables.Constants {
		constant := &tables.Constants[i]
		if constant.Parent.Table == winmd.TableField {
			in.constantIndex[constant.Parent.Row] = constant
		}
	}

	in.classLayoutIndex = make(map[uint32]*winmd.ClassLayoutRow, len(tables.ClassLayouts))
	for i := range tables.ClassLayouts {
		layout := &tables.ClassLayouts[i]
		in.classLayoutIndex[layout.Parent] = layout
	}

	in.nestedIndex = make(map[uint32][]uint32, len(tables.NestedClasses))
	in.nestedSet = make(map[uint32]bool, len(tables.NestedClasses))
	for _, nested := range tables.NestedClasses {
		in.nestedIndex[nested.EnclosingClass] = append(in.nestedIndex[nested.EnclosingClass], nested.NestedClass)
		in.nestedSet[nested.NestedClass] = true
	}

	in.interfaceImplIndex = make(map[uint32][]winmd.CodedIndex, len(tables.InterfaceImpls))
	for _, impl := range tables.InterfaceImpls {
		in.interfaceImplIndex[impl.Class] = append(in.interfaceImplIndex[impl.Class], impl.Interface)
	}

	// Classify every projected TypeDef so ApiRef conversion can stamp
	// TargetKind without re-walking. ExtraKinds seeds classifications from a
	// sister assembly (cross-assembly TypeRef targets).
	in.kindIndex = make(map[string]string, len(tables.TypeDefs)+len(in.opts.ExtraKinds))
	for fullName, kind := range in.opts.ExtraKinds {
		in.kindIndex[fullName] = kind
	}
	for typeDefRow := range tables.TypeDefs {
		typeDef := &tables.TypeDefs[typeDefRow]
		if !strings.HasPrefix(typeDef.Namespace, in.opts.ProjectPrefix) && typeDef.Namespace != "" {
			continue
		}
		kind := in.classifyTypeDef(typeDef, uint32(typeDefRow+1))
		if kind != "" && typeDef.Namespace != "" {
			in.kindIndex[typeDef.Namespace+"."+typeDef.Name] = kind
		}
	}
}

// classifyTypeDef determines a TypeDef's TargetKind.
func (in *Ingester) classifyTypeDef(typeDef *winmd.TypeDefRow, row uint32) string {
	if typeDef.Flags&winmd.TypeAttrInterface != 0 {
		return "Com"
	}
	extendsNamespace, extendsName := in.extendsOf(typeDef)
	switch {
	case extendsNamespace == "System" && extendsName == "Enum":
		return "Enum"
	case extendsNamespace == "System" && extendsName == "MulticastDelegate":
		return "FunctionPointer"
	case extendsNamespace == "System" && extendsName == "ValueType":
		if in.hasAttribute(typeDefTarget(row), "NativeTypedefAttribute") ||
			in.hasAttribute(typeDefTarget(row), "MetadataTypedefAttribute") {
			return "Typedef"
		}
		if typeDef.Flags&winmd.TypeAttrExplicitLayout != 0 {
			return "Union"
		}
		return "Struct"
	case extendsNamespace == "System" && extendsName == "Attribute":
		return "" // attribute definitions are not part of the API surface
	}
	// Non-Apis classes extending Object with a GUID are coclass IDs.
	if typeDef.Name != "Apis" {
		return "ComClassID"
	}
	return "Apis"
}

// extendsOf resolves a TypeDef's Extends coded index to (namespace, name).
func (in *Ingester) extendsOf(typeDef *winmd.TypeDefRow) (string, string) {
	tables := &in.file.Tables
	switch typeDef.Extends.Table {
	case winmd.TableTypeRef:
		if int(typeDef.Extends.Row) <= len(tables.TypeRefs) {
			ref := &tables.TypeRefs[typeDef.Extends.Row-1]
			return ref.Namespace, ref.Name
		}
	case winmd.TableTypeDef:
		if int(typeDef.Extends.Row) <= len(tables.TypeDefs) {
			def := &tables.TypeDefs[typeDef.Extends.Row-1]
			return def.Namespace, def.Name
		}
	}
	return "", ""
}

// projectTypeDef routes one top-level TypeDef into the namespace meta.
func (in *Ingester) projectTypeDef(meta *win32meta.NamespaceMeta, typeDef *winmd.TypeDefRow, row uint32) error {
	switch in.classifyTypeDef(typeDef, row) {
	case "Apis":
		return in.projectApis(meta, typeDef)
	case "Enum":
		meta.Enums[typeDef.Name] = in.projectEnum(typeDef, row)
	case "Struct", "Union":
		in.projectStructInto(meta, typeDef, row)
	case "Typedef":
		meta.Typedefs[typeDef.Name] = in.projectTypedef(typeDef, row)
	case "FunctionPointer":
		if delegate, ok := in.projectDelegate(typeDef, row); ok {
			meta.Delegates[typeDef.Name] = delegate
		}
	case "Com":
		meta.Interfaces[typeDef.Name] = in.projectInterface(typeDef, row)
	case "ComClassID":
		if guid := in.guidOf(typeDefTarget(row)); guid != "" {
			meta.Constants = append(meta.Constants, win32meta.Constant{
				Name:      typeDef.Name,
				Type:      win32meta.TypeRef{Kind: "Native", Name: "Guid"},
				Value:     guid,
				ValueKind: "Guid",
			})
		}
	}
	return nil
}

func typeDefTarget(row uint32) winmd.CodedIndex {
	return winmd.CodedIndex{Table: winmd.TableTypeDef, Row: row}
}
