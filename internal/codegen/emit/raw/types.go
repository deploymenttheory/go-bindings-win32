package rawwin

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// exportName capitalizes the first rune so struct fields are exported
// ("cbSize" → "CbSize"). Digit-leading names after underscore-trimming
// (DXGI matrix elements "_11") gain an "F" prefix.
func exportName(name string) string {
	if name == "" {
		return "Field"
	}
	name = strings.TrimLeft(name, "_")
	if name == "" {
		return "Field"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "F" + name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// nestedTypeName composes the emitted name of an anonymous nested type
// ("LARGE_INTEGER" + "_Anonymous_e__Struct" → "LARGE_INTEGER_Anonymous_e__Struct").
// The parent name is already exported; the composite therefore is too.
func nestedTypeName(parent, nested string) string {
	return parent + "_" + strings.TrimPrefix(nested, "_")
}

// buildEnumModels converts a namespace's enums, claiming member names in the
// package name set (first claim wins; duplicates are dropped with a
// diagnostic — the metadata shares members across enums deliberately).
func (g *Generator) buildEnumModels(meta *win32meta.NamespaceMeta) []view.EnumModel {
	names := make([]string, 0, len(meta.Enums))
	for name := range meta.Enums {
		names = append(names, name)
	}
	sort.Strings(names)

	models := make([]view.EnumModel, 0, len(names))
	for _, name := range names {
		goName := naming.Export(name)
		if !g.claimTypeName(goName) {
			g.diag("enum %s: name already used in package %s", name, meta.Namespace)
			continue
		}
		enum := meta.Enums[name]
		model := view.EnumModel{
			TypeName: goName,
			BaseType: enum.BaseType,
			IsFlags:  enum.IsFlags,
			DocURL:   enum.Availability.DocURL,
		}
		for _, member := range enum.Members {
			memberName := naming.Export(member.Name)
			if !g.claimName(memberName) {
				g.diag("enum member %s.%s: name already used", name, member.Name)
				continue
			}
			model.Members = append(model.Members, view.EnumMemberModel{
				Name:  memberName,
				Value: literalForBase(member.Value, enum.BaseType),
			})
		}
		models = append(models, model)
	}
	return models
}

// literalForBase renders a decimal value string as a Go literal valid for
// the base type: negative values in unsigned bases wrap to two's complement.
func literalForBase(value, baseType string) string {
	if !strings.HasPrefix(value, "-") || !strings.HasPrefix(baseType, "u") {
		return value
	}
	signed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return value
	}
	switch baseType {
	case "uint8":
		return strconv.FormatUint(uint64(uint8(signed)), 10)
	case "uint16":
		return strconv.FormatUint(uint64(uint16(signed)), 10)
	case "uint32":
		return strconv.FormatUint(uint64(uint32(signed)), 10)
	default:
		return strconv.FormatUint(uint64(signed), 10)
	}
}

// buildTypedefModels converts a namespace's typedefs.
func (g *Generator) buildTypedefModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.TypedefModel {
	names := make([]string, 0, len(meta.Typedefs))
	for name := range meta.Typedefs {
		names = append(names, name)
	}
	sort.Strings(names)

	models := make([]view.TypedefModel, 0, len(names))
	for _, name := range names {
		goName := naming.Export(name)
		if !g.claimTypeName(goName) {
			g.diag("typedef %s: name already used in package %s", name, meta.Namespace)
			continue
		}
		typedef := meta.Typedefs[name]
		backing := g.typedefBacking(&typedef, meta.Namespace, imports)
		models = append(models, view.TypedefModel{
			TypeName:      goName,
			Backing:       backing,
			DocURL:        typedef.Availability.DocURL,
			InvalidValues: typedef.InvalidValues,
			FreeFunc:      typedef.FreeFunc,
		})
	}
	return models
}

// typedefBacking picks the Go backing type for a named typedef: void*
// handles become uintptr (GC-opaque), other pointers stay typed pointers,
// scalars keep their scalar type.
func (g *Generator) typedefBacking(typedef *win32meta.Typedef, namespace string, imports typemap.ImportSet) string {
	underlying := &typedef.Underlying
	if underlying.Kind == "PointerTo" && underlying.Child != nil &&
		underlying.Child.Kind == "Native" && underlying.Child.Name == "Void" {
		return "uintptr"
	}
	resolved := g.mapper.GoType(underlying, typemap.Context{Namespace: namespace}, imports)
	if resolved.Kind == typemap.KindUnsupported || resolved.GoType == "" {
		return "uintptr"
	}
	return resolved.GoType
}

// buildStructModels converts structs and unions, flattening anonymous
// nested types into sibling named types.
func (g *Generator) buildStructModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.StructModel {
	names := make([]string, 0, len(meta.Structs))
	for name := range meta.Structs {
		names = append(names, name)
	}
	sort.Strings(names)

	var models []view.StructModel
	for _, name := range names {
		definition := meta.Structs[name]
		chosen := g.chooseArchVariant(name, &definition)
		if chosen == nil {
			continue
		}
		models = append(models, g.buildStructTree(meta, naming.Export(name), chosen, imports, false)...)
	}
	return models
}

// pickAmd64Variant returns the amd64-compatible layout of a struct, or nil.
// Pure — usable from layout computation without emitting diagnostics.
func pickAmd64Variant(definition *win32meta.Struct) *win32meta.Struct {
	matches := func(s *win32meta.Struct) bool {
		if len(s.Availability.Architectures) == 0 {
			return true
		}
		for _, arch := range s.Availability.Architectures {
			if arch == "amd64" {
				return true
			}
		}
		return false
	}
	if matches(definition) {
		return definition
	}
	for i := range definition.ArchVariants {
		if matches(&definition.ArchVariants[i]) {
			return &definition.ArchVariants[i]
		}
	}
	return nil
}

// chooseArchVariant picks the amd64 layout for emission, with diagnostics.
func (g *Generator) chooseArchVariant(name string, definition *win32meta.Struct) *win32meta.Struct {
	chosen := pickAmd64Variant(definition)
	if chosen == nil {
		g.diag("struct %s: no amd64 variant, skipped", name)
		return nil
	}
	if len(definition.ArchVariants) > 0 {
		g.diag("struct %s: emitting amd64 variant only (%d variants)", name, len(definition.ArchVariants)+1)
	}
	return chosen
}

// buildStructTree emits one struct plus its anonymous nested types as
// sibling named types. Top-level names consume their type pre-claim; nested
// composite names claim as values.
func (g *Generator) buildStructTree(meta *win32meta.NamespaceMeta, name string, definition *win32meta.Struct, imports typemap.ImportSet, isNested bool) []view.StructModel {
	claimed := false
	if isNested {
		claimed = g.claimName(name)
	} else {
		claimed = g.claimTypeName(name)
	}
	if !claimed {
		g.diag("struct %s: name already used in package %s", name, meta.Namespace)
		return nil
	}
	var models []view.StructModel
	model := view.StructModel{TypeName: name, DocURL: definition.Availability.DocURL}

	if definition.IsUnion {
		blob, ok := g.opaqueBlobFields(definition, name, "union")
		if !ok {
			g.unclaimName(name)
			return nil
		}
		model.IsUnionBlob = true
		model.Fields = blob
		g.recordABI(meta.Namespace, name, definition, nil)
		return []view.StructModel{model}
	}

	// A struct whose packed C layout differs from Go's natural layout of the
	// same fields cannot be represented field-by-field — emitting typed fields
	// would silently corrupt every offset after the first packed field. Emit
	// it as an opaque, correctly sized and aligned blob (like a union) so the
	// type still exists: references to it resolve to the precise named type
	// (FOO / *FOO) instead of degrading to unsafe.Pointer or being skipped.
	packedLayout := g.structLayoutOf(definition, true)
	naturalLayout := g.structLayoutOf(definition, false)
	if packedLayout.ok && naturalLayout.ok && !sameLayout(packedLayout, naturalLayout) {
		blob, ok := g.opaqueBlobFields(definition, name, "packed struct")
		if !ok {
			g.unclaimName(name)
			return nil
		}
		model.IsPackedBlob = true
		model.Fields = blob
		g.recordABI(meta.Namespace, name, definition, nil)
		return []view.StructModel{model}
	}

	// Emit anonymous nested types as siblings first (they precede the parent
	// in output), recording which actually emitted. A nested type that is
	// itself unrepresentable (e.g. packed) is skipped, and a by-value field
	// referencing it must not dangle — see fieldGoType.
	nestedNames := make([]string, 0, len(definition.NestedTypes))
	for nested := range definition.NestedTypes {
		nestedNames = append(nestedNames, nested)
	}
	sort.Strings(nestedNames)
	emittedNested := map[string]bool{}
	for _, nested := range nestedNames {
		nestedDefinition := definition.NestedTypes[nested]
		sub := g.buildStructTree(meta, nestedTypeName(name, nested), &nestedDefinition, imports, true)
		if len(sub) > 0 {
			models = append(models, sub...)
			emittedNested[nested] = true
		}
	}

	fieldNames := map[string]bool{}
	for i := range definition.Fields {
		field := &definition.Fields[i]
		goType, ok := g.fieldGoType(meta, name, definition, &field.Type, emittedNested, imports)
		if !ok {
			g.diag("struct %s: field %s type unresolved, struct skipped", name, field.Name)
			g.unclaimName(name)
			return models // keep the already-emitted nested siblings
		}
		fieldName := exportName(field.Name)
		if len(field.Bitfields) > 0 {
			fieldName = exportName(strings.ReplaceAll(field.Name, "_bitfield", "Bitfield"))
		}
		// Capitalization can collapse distinct C names ("y"/"Y"); suffix
		// the later one.
		for fieldNames[fieldName] {
			fieldName += "_"
		}
		fieldNames[fieldName] = true
		model.Fields = append(model.Fields, view.StructFieldModel{Name: fieldName, GoType: goType})
	}

	g.recordABI(meta.Namespace, name, definition, model.Fields)
	return append(models, model)
}

// sameLayout compares size, alignment, and every field offset.
func sameLayout(a, b structLayout) bool {
	if a.size != b.size || a.align != b.align || len(a.offsets) != len(b.offsets) {
		return false
	}
	for i := range a.offsets {
		if a.offsets[i] != b.offsets[i] {
			return false
		}
	}
	return true
}

// fieldGoType resolves a struct field type, mapping anonymous nested
// references onto their emitted sibling names. emittedNested reports which of
// the parent's nested types actually emitted; a by-value reference to a
// nested type that did not (e.g. a packed one) fails so the parent is skipped
// rather than dangling — a pointer to it degrades to unsafe.Pointer.
func (g *Generator) fieldGoType(meta *win32meta.NamespaceMeta, parentName string, parent *win32meta.Struct, ref *win32meta.TypeRef, emittedNested map[string]bool, imports typemap.ImportSet) (string, bool) {
	if ref.Kind == "ApiRef" && ref.Api == "" {
		if emittedNested[ref.Name] {
			return nestedTypeName(parentName, ref.Name), true
		}
		return "", false
	}
	if ref.Kind == "PointerTo" && ref.Child != nil && ref.Child.Kind == "ApiRef" && ref.Child.Api == "" {
		if emittedNested[ref.Child.Name] {
			return "*" + nestedTypeName(parentName, ref.Child.Name), true
		}
		return "unsafe.Pointer", true // pointee unrepresentable; the pointer is fine
	}
	if ref.Kind == "Array" && ref.Child != nil && ref.Child.Kind == "ApiRef" && ref.Child.Api == "" {
		if emittedNested[ref.Child.Name] {
			return fmt.Sprintf("[%d]%s", ref.ArrayLen, nestedTypeName(parentName, ref.Child.Name)), true
		}
		return "", false
	}
	resolved := g.mapper.GoType(ref, typemap.Context{Namespace: meta.Namespace}, imports)
	if resolved.Kind == typemap.KindUnsupported {
		// A by-value reference to a severed or skipped type: keep the
		// layout correct with an opaque, correctly sized and aligned blob.
		if ref.Kind == "ApiRef" || ref.Kind == "Array" {
			if blob, ok := g.layoutBlobType(ref); ok {
				return blob, true
			}
		}
		return "", false
	}
	// COM fields degrade via the mapper (diagnostic recorded); struct fields
	// of any resolvable type are representable in memory.
	return resolved.GoType, true
}

// layoutBlobType renders an opaque array type matching a foreign type's C
// size and alignment.
func (g *Generator) layoutBlobType(ref *win32meta.TypeRef) (string, bool) {
	refLayout := g.layoutOf(ref, nil)
	if !refLayout.ok || refLayout.size == 0 {
		return "", false
	}
	var element string
	var elementSize uint32
	switch refLayout.align {
	case 8:
		element, elementSize = "uint64", 8
	case 4:
		element, elementSize = "uint32", 4
	case 2:
		element, elementSize = "uint16", 2
	default:
		element, elementSize = "byte", 1
	}
	return fmt.Sprintf("[%d]%s", refLayout.size/elementSize, element), true
}

// opaqueBlobFields sizes a composite (union or packed struct) and renders its
// opaque backing field(s). kind labels the construct in the skip diagnostic.
func (g *Generator) opaqueBlobFields(definition *win32meta.Struct, name, kind string) ([]view.StructFieldModel, bool) {
	unionLayout := g.layoutOfStruct(definition)
	if !unionLayout.ok || unionLayout.size == 0 {
		g.diag("%s %s: layout not computable, skipped", kind, name)
		return nil, false
	}
	var element string
	var elementSize uint32
	switch unionLayout.align {
	case 8:
		element, elementSize = "uint64", 8
	case 4:
		element, elementSize = "uint32", 4
	case 2:
		element, elementSize = "uint16", 2
	default:
		element, elementSize = "byte", 1
	}
	count := unionLayout.size / elementSize
	return []view.StructFieldModel{{
		Name:   "Data",
		GoType: fmt.Sprintf("[%d]%s", count, element),
	}}, true
}

// buildDelegateModels converts callback function-pointer types.
func (g *Generator) buildDelegateModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.DelegateModel {
	names := make([]string, 0, len(meta.Delegates))
	for name := range meta.Delegates {
		names = append(names, name)
	}
	sort.Strings(names)

	models := make([]view.DelegateModel, 0, len(names))
	for _, name := range names {
		goName := naming.Export(name)
		if !g.claimTypeName(goName) {
			g.diag("delegate %s: name already used in package %s", name, meta.Namespace)
			continue
		}
		delegate := meta.Delegates[name]
		models = append(models, view.DelegateModel{
			TypeName:  goName,
			DocURL:    delegate.Availability.DocURL,
			Signature: g.delegateSignature(meta, &delegate, imports),
		})
	}
	return models
}

// delegateSignature renders the callback's Go shape for the doc comment.
func (g *Generator) delegateSignature(meta *win32meta.NamespaceMeta, delegate *win32meta.FuncPointer, imports typemap.ImportSet) string {
	var params []string
	for i := range delegate.Params {
		resolved := g.mapper.GoType(&delegate.Params[i].Type, typemap.Context{Namespace: meta.Namespace}, imports)
		params = append(params, resolved.GoType)
	}
	returnResolved := g.mapper.GoType(&delegate.Return, typemap.Context{Namespace: meta.Namespace, IsReturn: true}, imports)
	signature := "func(" + strings.Join(params, ", ") + ")"
	if returnResolved.Kind != typemap.KindVoid {
		signature += " " + returnResolved.GoType
	}
	return signature
}

// buildConstantModels converts Apis constants.
func (g *Generator) buildConstantModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) []view.ConstantModel {
	models := make([]view.ConstantModel, 0, len(meta.Constants))
	for i := range meta.Constants {
		constant := &meta.Constants[i]
		goName := naming.Export(constant.Name)
		if !g.claimName(goName) {
			g.diag("constant %s: name already used in package %s", constant.Name, meta.Namespace)
			continue
		}
		model, ok := g.buildConstant(meta, constant, imports)
		if !ok {
			g.unclaimName(goName)
			continue
		}
		model.Name = goName
		models = append(models, model)
	}
	return models
}

func (g *Generator) buildConstant(meta *win32meta.NamespaceMeta, constant *win32meta.Constant, imports typemap.ImportSet) (view.ConstantModel, bool) {
	switch constant.ValueKind {
	case "String":
		return view.ConstantModel{
			Name:    constant.Name,
			Literal: strconv.Quote(constant.Value),
		}, true
	case "Guid":
		literal, err := guidLiteral(constant.Value)
		if err != nil {
			g.diag("constant %s: %v", constant.Name, err)
			return view.ConstantModel{}, false
		}
		imports["win32"] = g.mapper.ModulePath + "/bindings/runtime/win32"
		return view.ConstantModel{Name: constant.Name, GoType: "win32.GUID", Literal: literal, IsVar: true}, true
	case "Int", "UInt", "Float":
		resolved := g.mapper.GoType(&constant.Type, typemap.Context{Namespace: meta.Namespace}, imports)
		if resolved.Kind == typemap.KindUnsupported {
			g.diag("constant %s: type unresolved", constant.Name)
			return view.ConstantModel{}, false
		}
		goType := resolved.GoType
		if resolved.Kind == typemap.KindPointerTypedef || resolved.Kind == typemap.KindPointer {
			// Integer sentinels typed as pointers ((PWSTR)-1) cannot be Go
			// pointer constants; expose the raw word instead.
			goType = "uintptr"
		}
		literal := constant.Value
		if constant.ValueKind == "Int" && strings.HasPrefix(literal, "-") {
			base := g.constantBase(meta, &constant.Type)
			// uintptr-backed targets (handles, IntPtr typedefs, pointer
			// sentinels) wrap negatives to the platform word.
			if resolved.Kind == typemap.KindHandleTypedef || goType == "uintptr" || base == "uintptr" {
				if literal == "-1" {
					return view.ConstantModel{Name: constant.Name, GoType: "", Literal: "^" + goType + "(0)"}, true
				}
				base = "uint64"
			}
			literal = literalForBase(literal, base)
		}
		return view.ConstantModel{Name: constant.Name, GoType: goType, Literal: literal}, true
	case "Struct":
		literal, ok := g.buildStructConstant(meta, constant, imports)
		if !ok {
			return view.ConstantModel{}, false
		}
		return view.ConstantModel{Name: constant.Name, Literal: literal, IsVar: true}, true
	}
	g.diag("constant %s: unknown value kind %q", constant.Name, constant.ValueKind)
	return view.ConstantModel{}, false
}

// constantBase finds the integral base type behind a constant's declared
// type so negative literals can wrap correctly in unsigned bases.
func (g *Generator) constantBase(meta *win32meta.NamespaceMeta, ref *win32meta.TypeRef) string {
	if ref.Kind == "Native" {
		if goType, ok := map[string]string{
			"Byte": "uint8", "UInt16": "uint16", "UInt32": "uint32", "UInt64": "uint64",
			"SByte": "int8", "Int16": "int16", "Int32": "int32", "Int64": "int64",
			"IntPtr": "uintptr", "UIntPtr": "uintptr",
		}[ref.Name]; ok {
			return goType
		}
		return "int64"
	}
	if ref.Kind == "PointerTo" {
		return "uintptr"
	}
	if ref.Kind == "ApiRef" {
		switch ref.TargetKind {
		case "Enum":
			if base := g.registry.EnumBase(ref.Api, ref.Name); base != "" {
				return base
			}
		case "Typedef":
			if typedef := g.registry.Typedef(ref.Api, ref.Name); typedef != nil {
				return g.constantBase(meta, &typedef.Underlying)
			}
		}
	}
	return "int64"
}

// guidLiteral renders a canonical GUID string as a win32.GUID literal.
func guidLiteral(guid string) (string, error) {
	parts := strings.Split(guid, "-")
	if len(parts) != 5 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return "", fmt.Errorf("malformed GUID %q", guid)
	}
	data1, err1 := strconv.ParseUint(parts[0], 16, 32)
	data2, err2 := strconv.ParseUint(parts[1], 16, 16)
	data3, err3 := strconv.ParseUint(parts[2], 16, 16)
	if err1 != nil || err2 != nil || err3 != nil {
		return "", fmt.Errorf("malformed GUID %q", guid)
	}
	tail := parts[3] + parts[4]
	var data4 [8]string
	for i := 0; i < 8; i++ {
		byteValue, err := strconv.ParseUint(tail[i*2:i*2+2], 16, 8)
		if err != nil {
			return "", fmt.Errorf("malformed GUID %q", guid)
		}
		data4[i] = fmt.Sprintf("0x%02x", byteValue)
	}
	return fmt.Sprintf("win32.GUID{Data1: 0x%08x, Data2: 0x%04x, Data3: 0x%04x, Data4: [8]byte{%s}}",
		data1, data2, data3, strings.Join(data4[:], ", ")), nil
}
