package ingest

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
	"github.com/deploymenttheory/go-bindings-win32/internal/winmd"
)

// ── Attribute helpers ─────────────────────────────────────────────────────────

// hasAttribute reports whether the target carries the named attribute.
func (in *Ingester) hasAttribute(target winmd.CodedIndex, name string) bool {
	for _, attr := range in.file.AttributesFor(target) {
		if attr.Name == name {
			return true
		}
	}
	return false
}

// guidOf reassembles a [Guid] attribute's 11 fixed args into canonical form.
func (in *Ingester) guidOf(target winmd.CodedIndex) string {
	for _, attr := range in.file.AttributesFor(target) {
		if attr.Name != "GuidAttribute" || len(attr.Fixed) != 11 {
			continue
		}
		data1, _ := attr.Fixed[0].(uint32)
		data2, _ := attr.Fixed[1].(uint16)
		data3, _ := attr.Fixed[2].(uint16)
		var data4 [8]byte
		for i := 0; i < 8; i++ {
			data4[i], _ = attr.Fixed[3+i].(byte)
		}
		return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
			data1, data2, data3,
			data4[0], data4[1], data4[2], data4[3], data4[4], data4[5], data4[6], data4[7])
	}
	return ""
}

// availabilityOf extracts platform/architecture/doc attributes.
func (in *Ingester) availabilityOf(target winmd.CodedIndex) win32meta.Availability {
	var availability win32meta.Availability
	for _, attr := range in.file.AttributesFor(target) {
		switch attr.Name {
		case "SupportedOSPlatformAttribute":
			if len(attr.Fixed) == 1 {
				availability.Platform, _ = attr.Fixed[0].(string)
			}
		case "SupportedArchitectureAttribute":
			if len(attr.Fixed) == 1 {
				mask, _ := attr.Fixed[0].(int32)
				availability.Architectures = archNames(mask)
			}
		case "DocumentationAttribute":
			if len(attr.Fixed) == 1 {
				availability.DocURL, _ = attr.Fixed[0].(string)
			}
		}
	}
	return availability
}

// archNames expands the win32metadata Architecture bitmask (X86=1, X64=2,
// Arm64=4) into Go GOARCH names.
func archNames(mask int32) []string {
	var names []string
	if mask&1 != 0 {
		names = append(names, "386")
	}
	if mask&2 != 0 {
		names = append(names, "amd64")
	}
	if mask&4 != 0 {
		names = append(names, "arm64")
	}
	return names
}

// ── TypeRef conversion ────────────────────────────────────────────────────────

// primitiveNames maps ECMA element types to IR Native names (win32json's
// vocabulary).
var primitiveNames = map[winmd.ElementType]string{
	winmd.ElemVoid:    "Void",
	winmd.ElemBoolean: "Boolean",
	winmd.ElemChar:    "Char",
	winmd.ElemInt8:    "SByte",
	winmd.ElemUInt8:   "Byte",
	winmd.ElemInt16:   "Int16",
	winmd.ElemUInt16:  "UInt16",
	winmd.ElemInt32:   "Int32",
	winmd.ElemUInt32:  "UInt32",
	winmd.ElemInt64:   "Int64",
	winmd.ElemUInt64:  "UInt64",
	winmd.ElemFloat32: "Single",
	winmd.ElemFloat64: "Double",
	winmd.ElemIntPtr:  "IntPtr",
	winmd.ElemUIntPtr: "UIntPtr",
	winmd.ElemString:  "String",
	winmd.ElemObject:  "Object",
}

// typeRefOf converts a decoded winmd type signature into the IR TypeRef.
func (in *Ingester) typeRefOf(sig *winmd.TypeSig) win32meta.TypeRef {
	switch sig.Kind {
	case winmd.SigPrimitive:
		name, ok := primitiveNames[sig.Primitive]
		if !ok {
			in.Diagnostics = append(in.Diagnostics, fmt.Sprintf("unmapped primitive 0x%02x", byte(sig.Primitive)))
			name = "UIntPtr"
		}
		return win32meta.TypeRef{Kind: "Native", Name: name, IsConst: sig.IsConst}

	case winmd.SigNamed:
		if sig.Namespace == "System" {
			if sig.Name == "Guid" {
				return win32meta.TypeRef{Kind: "Native", Name: "Guid", IsConst: sig.IsConst}
			}
			in.Diagnostics = append(in.Diagnostics, "unmapped System type "+sig.Name)
			return win32meta.TypeRef{Kind: "Native", Name: "UIntPtr", IsConst: sig.IsConst}
		}
		if sig.Namespace == "" {
			// A nested anonymous type (union/struct) declared inside a
			// struct. Nested CLR types carry no namespace; the field
			// references the nested type by name. Api "" tells the struct
			// emitter and the layout calculator to resolve it against the
			// enclosing struct's nested types.
			return win32meta.TypeRef{Kind: "ApiRef", Name: sig.Name, Api: "", TargetKind: "Struct", IsConst: sig.IsConst}
		}
		if !strings.HasPrefix(sig.Namespace, namespacePrefix) {
			// WinRT types (Windows.Foundation.*, Windows.UI.*) referenced
			// from Win32 interop APIs: pointer-sized object references the
			// Win32 projection cannot bind.
			in.Diagnostics = append(in.Diagnostics, "WinRT type "+sig.Namespace+"."+sig.Name+" degraded to UIntPtr")
			return win32meta.TypeRef{Kind: "Native", Name: "UIntPtr", IsConst: sig.IsConst}
		}
		fullName := sig.Namespace + "." + sig.Name
		return win32meta.TypeRef{
			Kind:       "ApiRef",
			Name:       sig.Name,
			Api:        strings.TrimPrefix(sig.Namespace, namespacePrefix),
			TargetKind: in.kindIndex[fullName],
			IsConst:    sig.IsConst,
		}

	case winmd.SigPointer:
		child := in.typeRefOf(sig.Child)
		return win32meta.TypeRef{Kind: "PointerTo", Child: &child, IsConst: sig.IsConst}

	case winmd.SigArray:
		child := in.typeRefOf(sig.Child)
		return win32meta.TypeRef{Kind: "Array", Child: &child, ArrayLen: sig.ArrayLen, IsConst: sig.IsConst}

	case winmd.SigSZArray:
		child := in.typeRefOf(sig.Child)
		return win32meta.TypeRef{Kind: "Array", Child: &child, IsConst: sig.IsConst}

	case winmd.SigFuncPtr:
		// Named delegates cover the Win32 surface; a raw FNPTR degrades to
		// an untyped pointer with a diagnostic.
		in.Diagnostics = append(in.Diagnostics, "raw function-pointer signature degraded to *void")
		void := win32meta.TypeRef{Kind: "Native", Name: "Void"}
		return win32meta.TypeRef{Kind: "PointerTo", Child: &void}
	}
	in.Diagnostics = append(in.Diagnostics, fmt.Sprintf("unmapped signature kind %d", sig.Kind))
	return win32meta.TypeRef{Kind: "Native", Name: "UIntPtr"}
}

// ── Params ────────────────────────────────────────────────────────────────────

// paramsOf assembles IR params for a method: signature types matched with
// Param rows by sequence number, plus per-param attributes.
func (in *Ingester) paramsOf(method *winmd.MethodDefRow, sig *winmd.MethodSig) []win32meta.Param {
	// Param rows with Sequence n correspond to sig.Params[n-1].
	params := make([]win32meta.Param, len(sig.Params))
	for i := range sig.Params {
		params[i] = win32meta.Param{
			Name:                       fmt.Sprintf("param%d", i),
			Type:                       in.typeRefOf(&sig.Params[i]),
			NativeArrayCountParamIndex: -1,
			MemorySizeBytesParamIndex:  -1,
			IidParamIndex:              -1,
		}
	}
	tables := &in.file.Tables
	for row := method.ParamFirst; row < method.ParamEnd && int(row) <= len(tables.Params); row++ {
		paramRow := &tables.Params[row-1]
		if paramRow.Sequence == 0 || int(paramRow.Sequence) > len(params) {
			continue // sequence 0 carries return-value attributes
		}
		param := &params[paramRow.Sequence-1]
		param.Name = paramRow.Name
		param.IsIn = paramRow.Flags&paramFlagIn != 0
		param.IsOut = paramRow.Flags&paramFlagOut != 0
		param.IsOptional = paramRow.Flags&paramFlagOptional != 0
		in.applyParamAttributes(param, winmd.CodedIndex{Table: winmd.TableParam, Row: row})
	}
	return params
}

func (in *Ingester) applyParamAttributes(param *win32meta.Param, target winmd.CodedIndex) {
	for _, attr := range in.file.AttributesFor(target) {
		switch attr.Name {
		case "ConstAttribute":
			param.IsConst = true
		case "ReservedAttribute":
			param.IsReserved = true
		case "ComOutPtrAttribute":
			param.IsComOutPtr = true
		case "RetValAttribute":
			param.IsRetVal = true
		case "FreeWithAttribute":
			if len(attr.Fixed) == 1 {
				param.FreeWith, _ = attr.Fixed[0].(string)
			}
		case "NativeArrayInfoAttribute":
			if v, ok := attr.Named["CountParamIndex"]; ok {
				param.NativeArrayCountParamIndex = intValue(v)
			}
			if v, ok := attr.Named["CountConst"]; ok {
				param.NativeArrayCountConst = uint32(intValue(v))
			}
		case "MemorySizeAttribute":
			if v, ok := attr.Named["BytesParamIndex"]; ok {
				param.MemorySizeBytesParamIndex = intValue(v)
			}
		case "IidParameterIndexAttribute":
			if len(attr.Fixed) == 1 {
				param.IidParamIndex = intValue(attr.Fixed[0])
			}
		}
	}
}

// intValue normalizes the integer types the attribute decoder produces.
func intValue(v any) int {
	switch value := v.(type) {
	case int16:
		return int(value)
	case uint16:
		return int(value)
	case int32:
		return int(value)
	case uint32:
		return int(value)
	case int64:
		return int(value)
	}
	return -1
}

// ── Apis (functions + constants) ──────────────────────────────────────────────

func (in *Ingester) projectApis(meta *win32meta.NamespaceMeta, typeDef *winmd.TypeDefRow) error {
	tables := &in.file.Tables

	for row := typeDef.MethodFirst; row < typeDef.MethodEnd && int(row) <= len(tables.Methods); row++ {
		method := &tables.Methods[row-1]
		implMap := in.implMapIndex[row]
		if implMap == nil {
			// Inline [Constant] helper functions have no import; skip for now.
			continue
		}
		sig, err := in.file.MethodSignature(method.Signature)
		if err != nil {
			return fmt.Errorf("method %s: %w", method.Name, err)
		}
		target := winmd.CodedIndex{Table: winmd.TableMethodDef, Row: row}
		function := win32meta.Function{
			Name:         method.Name,
			DLL:          tables.ModuleRefs[implMap.ImportScope-1],
			SetLastError: implMap.MappingFlags&implMapFlagSupportsLastError != 0,
			Return:       in.typeRefOf(&sig.Return),
			Params:       in.paramsOf(method, &sig),
			Availability: in.availabilityOf(target),
		}
		if implMap.ImportName != method.Name {
			function.EntryPoint = implMap.ImportName
		}
		if in.hasAttribute(target, "UnicodeAttribute") && strings.HasSuffix(method.Name, "W") {
			function.UnsuffixedName = strings.TrimSuffix(method.Name, "W")
		}
		meta.Functions = append(meta.Functions, function)
	}

	for row := typeDef.FieldFirst; row < typeDef.FieldEnd && int(row) <= len(tables.Fields); row++ {
		if constant, ok := in.projectConstant(row); ok {
			meta.Constants = append(meta.Constants, constant)
		}
	}
	return nil
}

// projectConstant projects one Apis field into an IR constant.
func (in *Ingester) projectConstant(fieldRow uint32) (win32meta.Constant, bool) {
	field := &in.file.Tables.Fields[fieldRow-1]
	fieldSig, err := in.file.FieldSignature(field.Signature)
	if err != nil {
		in.Diagnostics = append(in.Diagnostics, "constant "+field.Name+": "+err.Error())
		return win32meta.Constant{}, false
	}
	target := winmd.CodedIndex{Table: winmd.TableField, Row: fieldRow}
	constant := win32meta.Constant{
		Name:         field.Name,
		Type:         in.typeRefOf(&fieldSig),
		Availability: in.availabilityOf(target),
	}

	if field.Flags&fieldFlagLiteral != 0 {
		constantRow := in.constantIndex[fieldRow]
		if constantRow == nil {
			return win32meta.Constant{}, false
		}
		value, kind := in.decodeConstantValue(constantRow)
		constant.Value, constant.ValueKind = value, kind
		return constant, true
	}
	// Static non-literal constants: GUIDs and struct initializers.
	if guid := in.guidOf(target); guid != "" {
		constant.Value, constant.ValueKind = guid, "Guid"
		return constant, true
	}
	for _, attr := range in.file.AttributesFor(target) {
		if attr.Name == "ConstantAttribute" && len(attr.Fixed) == 1 {
			initializer, _ := attr.Fixed[0].(string)
			constant.Value, constant.ValueKind = initializer, "Struct"
			return constant, true
		}
	}
	return win32meta.Constant{}, false
}

// decodeConstantValue renders a Constant table value as (string, kind).
func (in *Ingester) decodeConstantValue(row *winmd.ConstantRow) (string, string) {
	blob := in.file.Blobs.Get(row.Value)
	value := winmd.DecodeConstant(winmd.ElementType(row.Type), blob)
	switch typed := value.(type) {
	case string:
		return typed, "String"
	case int64:
		return strconv.FormatInt(typed, 10), "Int"
	case uint64:
		return strconv.FormatUint(typed, 10), "UInt"
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), "Float"
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), "Float"
	case bool:
		if typed {
			return "1", "UInt"
		}
		return "0", "UInt"
	}
	in.Diagnostics = append(in.Diagnostics, fmt.Sprintf("constant type 0x%02x undecoded", row.Type))
	return "0", "UInt"
}

// ── Enums ─────────────────────────────────────────────────────────────────────

// enumBaseNames maps the value__ field's primitive to a Go integral type.
var enumBaseNames = map[winmd.ElementType]string{
	winmd.ElemInt8:   "int8",
	winmd.ElemUInt8:  "uint8",
	winmd.ElemInt16:  "int16",
	winmd.ElemUInt16: "uint16",
	winmd.ElemInt32:  "int32",
	winmd.ElemUInt32: "uint32",
	winmd.ElemInt64:  "int64",
	winmd.ElemUInt64: "uint64",
}

func (in *Ingester) projectEnum(typeDef *winmd.TypeDefRow, row uint32) win32meta.Enum {
	tables := &in.file.Tables
	enum := win32meta.Enum{
		BaseType:     "uint32",
		IsFlags:      in.hasAttribute(typeDefTarget(row), "FlagsAttribute"),
		IsScoped:     in.hasAttribute(typeDefTarget(row), "ScopedEnumAttribute"),
		Availability: in.availabilityOf(typeDefTarget(row)),
	}
	for fieldRow := typeDef.FieldFirst; fieldRow < typeDef.FieldEnd && int(fieldRow) <= len(tables.Fields); fieldRow++ {
		field := &tables.Fields[fieldRow-1]
		if field.Name == "value__" {
			if sig, err := in.file.FieldSignature(field.Signature); err == nil && sig.Kind == winmd.SigPrimitive {
				if base, ok := enumBaseNames[sig.Primitive]; ok {
					enum.BaseType = base
				}
			}
			continue
		}
		constantRow := in.constantIndex[fieldRow]
		if constantRow == nil {
			continue
		}
		value, _ := in.decodeConstantValue(constantRow)
		enum.Members = append(enum.Members, win32meta.EnumMember{Name: field.Name, Value: value})
	}
	return enum
}

// ── Structs and unions ────────────────────────────────────────────────────────

// projectStructInto adds a struct, folding arch-specific duplicates into
// ArchVariants of the first-seen entry.
func (in *Ingester) projectStructInto(meta *win32meta.NamespaceMeta, typeDef *winmd.TypeDefRow, row uint32) {
	projected := in.projectStruct(typeDef, row)
	existing, exists := meta.Structs[typeDef.Name]
	if !exists {
		meta.Structs[typeDef.Name] = projected
		return
	}
	existing.ArchVariants = append(existing.ArchVariants, projected)
	meta.Structs[typeDef.Name] = existing
}

func (in *Ingester) projectStruct(typeDef *winmd.TypeDefRow, row uint32) win32meta.Struct {
	tables := &in.file.Tables
	projected := win32meta.Struct{
		IsUnion:      typeDef.Flags&typeFlagExplicitLayout != 0,
		Availability: in.availabilityOf(typeDefTarget(row)),
	}
	if layout := in.classLayoutIndex[row]; layout != nil {
		projected.PackingSize = layout.PackingSize
		projected.Size = layout.ClassSize
	}
	for _, attr := range in.file.AttributesFor(typeDefTarget(row)) {
		if attr.Name == "StructSizeFieldAttribute" && len(attr.Fixed) == 1 {
			projected.StructSizeField, _ = attr.Fixed[0].(string)
		}
	}

	for fieldRow := typeDef.FieldFirst; fieldRow < typeDef.FieldEnd && int(fieldRow) <= len(tables.Fields); fieldRow++ {
		field := &tables.Fields[fieldRow-1]
		if field.Flags&fieldFlagStatic != 0 {
			continue
		}
		fieldSig, err := in.file.FieldSignature(field.Signature)
		if err != nil {
			in.Diagnostics = append(in.Diagnostics, "field "+field.Name+": "+err.Error())
			continue
		}
		structField := win32meta.StructField{Name: field.Name, Type: in.typeRefOf(&fieldSig)}
		fieldTarget := winmd.CodedIndex{Table: winmd.TableField, Row: fieldRow}
		for _, attr := range in.file.AttributesFor(fieldTarget) {
			switch attr.Name {
			case "NativeBitfieldAttribute":
				if len(attr.Fixed) == 3 {
					name, _ := attr.Fixed[0].(string)
					structField.Bitfields = append(structField.Bitfields, win32meta.Bitfield{
						Name:   name,
						Offset: int64Value(attr.Fixed[1]),
						Length: int64Value(attr.Fixed[2]),
					})
				}
			case "FlexibleArrayAttribute":
				structField.IsFlexibleArray = true
			}
		}
		projected.Fields = append(projected.Fields, structField)
	}

	// Nested anonymous structs/unions.
	for _, nestedRow := range in.nestedIndex[row] {
		nestedTypeDef := &tables.TypeDefs[nestedRow-1]
		if projected.NestedTypes == nil {
			projected.NestedTypes = map[string]win32meta.Struct{}
		}
		projected.NestedTypes[nestedTypeDef.Name] = in.projectStruct(nestedTypeDef, nestedRow)
	}
	return projected
}

func int64Value(v any) int64 {
	switch value := v.(type) {
	case int32:
		return int64(value)
	case int64:
		return value
	case uint32:
		return int64(value)
	case uint64:
		return int64(value)
	}
	return 0
}

// ── Typedefs ──────────────────────────────────────────────────────────────────

func (in *Ingester) projectTypedef(typeDef *winmd.TypeDefRow, row uint32) win32meta.Typedef {
	tables := &in.file.Tables
	typedef := win32meta.Typedef{
		IsMetadataTypedef: in.hasAttribute(typeDefTarget(row), "MetadataTypedefAttribute"),
		Availability:      in.availabilityOf(typeDefTarget(row)),
	}
	for fieldRow := typeDef.FieldFirst; fieldRow < typeDef.FieldEnd && int(fieldRow) <= len(tables.Fields); fieldRow++ {
		field := &tables.Fields[fieldRow-1]
		if field.Flags&fieldFlagStatic != 0 {
			continue
		}
		if sig, err := in.file.FieldSignature(field.Signature); err == nil {
			typedef.Underlying = in.typeRefOf(&sig)
		}
		break
	}
	for _, attr := range in.file.AttributesFor(typeDefTarget(row)) {
		switch attr.Name {
		case "RAIIFreeAttribute":
			if len(attr.Fixed) == 1 {
				typedef.FreeFunc, _ = attr.Fixed[0].(string)
			}
		case "InvalidHandleValueAttribute":
			if len(attr.Fixed) == 1 {
				typedef.InvalidValues = append(typedef.InvalidValues, strconv.FormatInt(int64Value(attr.Fixed[0]), 10))
			}
		case "AlsoUsableForAttribute":
			if len(attr.Fixed) == 1 {
				typedef.AlsoUsableFor, _ = attr.Fixed[0].(string)
			}
		}
	}
	return typedef
}

// ── Delegates ─────────────────────────────────────────────────────────────────

func (in *Ingester) projectDelegate(typeDef *winmd.TypeDefRow, row uint32) (win32meta.FuncPointer, bool) {
	tables := &in.file.Tables
	for methodRow := typeDef.MethodFirst; methodRow < typeDef.MethodEnd && int(methodRow) <= len(tables.Methods); methodRow++ {
		method := &tables.Methods[methodRow-1]
		if method.Name != "Invoke" {
			continue
		}
		sig, err := in.file.MethodSignature(method.Signature)
		if err != nil {
			in.Diagnostics = append(in.Diagnostics, "delegate "+typeDef.Name+": "+err.Error())
			return win32meta.FuncPointer{}, false
		}
		return win32meta.FuncPointer{
			Return:       in.typeRefOf(&sig.Return),
			Params:       in.paramsOf(method, &sig),
			Availability: in.availabilityOf(typeDefTarget(row)),
		}, true
	}
	return win32meta.FuncPointer{}, false
}

// ── COM interfaces ────────────────────────────────────────────────────────────

func (in *Ingester) projectInterface(typeDef *winmd.TypeDefRow, row uint32) win32meta.ComInterface {
	tables := &in.file.Tables
	comInterface := win32meta.ComInterface{
		GUID:         in.guidOf(typeDefTarget(row)),
		Availability: in.availabilityOf(typeDefTarget(row)),
	}
	for _, base := range in.interfaceImplIndex[row] {
		switch base.Table {
		case winmd.TableTypeRef:
			ref := &tables.TypeRefs[base.Row-1]
			comInterface.BaseInterface = ref.Name
			comInterface.BaseInterfaceApi = strings.TrimPrefix(ref.Namespace, namespacePrefix)
		case winmd.TableTypeDef:
			def := &tables.TypeDefs[base.Row-1]
			comInterface.BaseInterface = def.Name
			comInterface.BaseInterfaceApi = strings.TrimPrefix(def.Namespace, namespacePrefix)
		}
	}
	for methodRow := typeDef.MethodFirst; methodRow < typeDef.MethodEnd && int(methodRow) <= len(tables.Methods); methodRow++ {
		method := &tables.Methods[methodRow-1]
		sig, err := in.file.MethodSignature(method.Signature)
		if err != nil {
			in.Diagnostics = append(in.Diagnostics, "interface method "+method.Name+": "+err.Error())
			continue
		}
		comInterface.Methods = append(comInterface.Methods, win32meta.ComMethod{
			Name:   method.Name,
			Return: in.typeRefOf(&sig.Return),
			Params: in.paramsOf(method, &sig),
		})
	}
	return comInterface
}
