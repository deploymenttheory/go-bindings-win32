package winmd

import (
	"fmt"
	"math"
)

// CustomAttr is a decoded custom attribute: the attribute type's name plus
// its constructor (fixed) and named arguments (ECMA-335 §II.23.3).
//
// Argument values are one of: string, bool, byte, int8, uint16, int16,
// uint32, int32, uint64, int64, float32, float64, or []any (SZARRAY).
// Enum-typed arguments decode as int32 (every enum used by the Win32
// metadata attribute set is int32-backed).
type CustomAttr struct {
	Namespace string
	Name      string // e.g. "SupportedArchitectureAttribute"
	Fixed     []any
	Named     map[string]any
}

// attrTarget produces a compact comparable key for a CodedIndex.
func attrTarget(index CodedIndex) uint64 {
	return uint64(index.Table)<<32 | uint64(index.Row)
}

// AttributesFor returns the decoded custom attributes attached to the given
// metadata element. Attributes whose blobs fail to decode are skipped.
func (f *File) AttributesFor(target CodedIndex) []CustomAttr {
	f.buildAttrIndexOnce()
	rows := f.attrIndex[attrTarget(target)]
	if len(rows) == 0 {
		return nil
	}
	attrs := make([]CustomAttr, 0, len(rows))
	for _, row := range rows {
		attr, err := f.decodeAttribute(&f.Tables.CustomAttributes[row])
		if err != nil {
			continue // tolerate exotic blobs; the projection reads a known set
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

// buildAttrIndexOnce lazily builds the Parent → CustomAttribute-rows index.
func (f *File) buildAttrIndexOnce() {
	if f.attrIndex != nil {
		return
	}
	index := make(map[uint64][]int, len(f.Tables.CustomAttributes))
	for row := range f.Tables.CustomAttributes {
		key := attrTarget(f.Tables.CustomAttributes[row].Parent)
		index[key] = append(index[key], row)
	}
	f.attrIndex = index
}

// decodeAttribute resolves the attribute's constructor and decodes the value
// blob against the constructor's parameter types.
func (f *File) decodeAttribute(row *CustomAttributeRow) (CustomAttr, error) {
	namespace, name, ctorSig, err := f.resolveAttributeCtor(row.Type)
	if err != nil {
		return CustomAttr{}, err
	}
	attr := CustomAttr{Namespace: namespace, Name: name}

	blob := f.Blobs.Get(row.Value)
	if blob == nil {
		return CustomAttr{}, fmt.Errorf("attribute value blob 0x%x out of range", row.Value)
	}
	reader := blobReader{data: blob}
	const attrProlog = 0x0001
	if prolog := reader.uint16(); prolog != attrProlog {
		return CustomAttr{}, fmt.Errorf("attribute blob prolog 0x%04x, want 0x0001", prolog)
	}
	for i := range ctorSig.Params {
		attr.Fixed = append(attr.Fixed, f.readFixedArg(&reader, &ctorSig.Params[i]))
	}
	namedCount := int(reader.uint16())
	if namedCount > 0 {
		// Size hint clamped to the bytes left: each named arg is ≥3 bytes
		// (kind, type, name), so a corrupt count cannot force an outsized map.
		attr.Named = make(map[string]any, min(namedCount, reader.remaining()))
		for i := 0; i < namedCount && !reader.failed(); i++ {
			argName, value := f.readNamedArg(&reader)
			attr.Named[argName] = value
		}
	}
	if reader.err != nil {
		return CustomAttr{}, reader.err
	}
	return attr, nil
}

// resolveAttributeCtor resolves a CustomAttributeType coded index (MethodDef
// or MemberRef) to the attribute type's namespace/name and ctor signature.
func (f *File) resolveAttributeCtor(ctor CodedIndex) (namespace, name string, sig MethodSig, err error) {
	switch ctor.Table {
	case TableMemberRef:
		if ctor.Row == 0 || int(ctor.Row) > len(f.Tables.MemberRefs) {
			return "", "", MethodSig{}, fmt.Errorf("MemberRef row %d out of range", ctor.Row)
		}
		memberRef := &f.Tables.MemberRefs[ctor.Row-1]
		switch memberRef.Class.Table {
		case TableTypeRef:
			if memberRef.Class.Row == 0 || int(memberRef.Class.Row) > len(f.Tables.TypeRefs) {
				return "", "", MethodSig{}, fmt.Errorf("MemberRef parent TypeRef row %d out of range", memberRef.Class.Row)
			}
			typeRef := &f.Tables.TypeRefs[memberRef.Class.Row-1]
			namespace, name = typeRef.Namespace, typeRef.Name
		case TableTypeDef:
			if memberRef.Class.Row == 0 || int(memberRef.Class.Row) > len(f.Tables.TypeDefs) {
				return "", "", MethodSig{}, fmt.Errorf("MemberRef parent TypeDef row %d out of range", memberRef.Class.Row)
			}
			typeDef := &f.Tables.TypeDefs[memberRef.Class.Row-1]
			namespace, name = typeDef.Namespace, typeDef.Name
		default:
			return "", "", MethodSig{}, fmt.Errorf("unsupported MemberRef parent table 0x%02x", memberRef.Class.Table)
		}
		sig, err = f.MethodSignature(memberRef.Signature)
		return namespace, name, sig, err

	case TableMethodDef:
		if ctor.Row == 0 || int(ctor.Row) > len(f.Tables.Methods) {
			return "", "", MethodSig{}, fmt.Errorf("MethodDef row %d out of range", ctor.Row)
		}
		method := &f.Tables.Methods[ctor.Row-1]
		namespace, name = f.declaringType(ctor.Row)
		sig, err = f.MethodSignature(method.Signature)
		return namespace, name, sig, err
	}
	return "", "", MethodSig{}, fmt.Errorf("unsupported CustomAttributeType table 0x%02x", ctor.Table)
}

// declaringType finds the TypeDef whose method range contains the given
// 1-based MethodDef row, via a lazily built reverse index.
func (f *File) declaringType(methodRow uint32) (namespace, name string) {
	if f.methodOwnerIndex == nil {
		f.methodOwnerIndex = make([]uint32, len(f.Tables.Methods)+1)
		for typeDefRow := range f.Tables.TypeDefs {
			typeDef := &f.Tables.TypeDefs[typeDefRow]
			for row := typeDef.MethodFirst; row < typeDef.MethodEnd && int(row) <= len(f.Tables.Methods); row++ {
				f.methodOwnerIndex[row] = uint32(typeDefRow + 1)
			}
		}
	}
	if int(methodRow) >= len(f.methodOwnerIndex) || f.methodOwnerIndex[methodRow] == 0 {
		return "", ""
	}
	// Row bounds: index entries are typeDefRow+1 for rows of TypeDefs, so the
	// nonzero value is always in [1, len(TypeDefs)].
	typeDef := &f.Tables.TypeDefs[f.methodOwnerIndex[methodRow]-1]
	return typeDef.Namespace, typeDef.Name
}

// readFixedArg reads one fixed argument value driven by the ctor param type.
func (f *File) readFixedArg(reader *blobReader, sig *TypeSig) any {
	switch sig.Kind {
	case SigPrimitive:
		return readPrimitiveValue(reader, sig.Primitive)
	case SigNamed:
		if sig.Namespace == "System" && sig.Name == "Type" {
			return reader.serString()
		}
		if sig.IsValueType {
			// Enum argument: int32-backed throughout the Win32 attribute set.
			return int32(reader.uint32())
		}
		if sig.Namespace == "System" && sig.Name == "String" {
			return reader.serString()
		}
		reader.setErr(fmt.Sprintf("unsupported named fixed-arg type %s.%s", sig.Namespace, sig.Name))
		return nil
	case SigSZArray:
		count := reader.uint32()
		if count == 0xFFFFFFFF { // null array
			return []any(nil)
		}
		// Capacity clamped to the bytes left: each element is ≥1 byte, so a
		// corrupt count cannot force an outsized allocation.
		values := make([]any, 0, min(int(count), reader.remaining()))
		for i := uint32(0); i < count && !reader.failed(); i++ {
			values = append(values, f.readFixedArg(reader, sig.Child))
		}
		return values
	}
	reader.setErr(fmt.Sprintf("unsupported fixed-arg kind %d", sig.Kind))
	return nil
}

// readNamedArg reads one FIELD/PROPERTY named argument.
func (f *File) readNamedArg(reader *blobReader) (string, any) {
	const (
		namedField    = 0x53
		namedProperty = 0x54
	)
	kind := reader.byte()
	if kind != namedField && kind != namedProperty {
		reader.setErr(fmt.Sprintf("bad named-arg kind 0x%02x", kind))
		return "", nil
	}
	valueType := reader.byte()
	// ENUM is followed by the enum type's SerString name.
	const (
		typeSystemType = 0x50
		typeBoxed      = 0x51
		typeEnum       = 0x55
	)
	var elemType byte
	switch valueType {
	case typeEnum:
		reader.serString() // enum type name; value is int32-backed
	case typeBoxed, typeSystemType:
		// Boxed object / System.Type named args are not used by the Win32
		// attribute set; decode System.Type as a SerString if seen.
	default:
		elemType = valueType
	}
	name := reader.serString()

	switch valueType {
	case typeEnum:
		return name, int32(reader.uint32())
	case typeSystemType:
		return name, reader.serString()
	case typeBoxed:
		reader.setErr("boxed named args unsupported")
		return name, nil
	default:
		return name, readPrimitiveValue(reader, ElementType(elemType))
	}
}

// readPrimitiveValue reads a primitive attribute value by element type.
func readPrimitiveValue(reader *blobReader, elem ElementType) any {
	switch elem {
	case ElemBoolean:
		return reader.byte() != 0
	case ElemChar:
		return uint16(reader.uint16())
	case ElemInt8:
		return int8(reader.byte())
	case ElemUInt8:
		return reader.byte()
	case ElemInt16:
		return int16(reader.uint16())
	case ElemUInt16:
		return reader.uint16()
	case ElemInt32:
		return int32(reader.uint32())
	case ElemUInt32:
		return reader.uint32()
	case ElemInt64:
		return int64(reader.uint64())
	case ElemUInt64:
		return reader.uint64()
	case ElemFloat32:
		return math.Float32frombits(reader.uint32())
	case ElemFloat64:
		return math.Float64frombits(reader.uint64())
	case ElemString:
		return reader.serString()
	}
	reader.setErr(fmt.Sprintf("unsupported primitive attr value 0x%02x", byte(elem)))
	return nil
}
