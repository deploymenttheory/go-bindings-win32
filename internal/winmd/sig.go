package winmd

import "fmt"

// ELEMENT_TYPE_* constants (ECMA-335 II.23.1.16).
type ElementType byte

const (
	ElemEnd         ElementType = 0x00
	ElemVoid        ElementType = 0x01
	ElemBoolean     ElementType = 0x02
	ElemChar        ElementType = 0x03
	ElemInt8        ElementType = 0x04
	ElemUInt8       ElementType = 0x05
	ElemInt16       ElementType = 0x06
	ElemUInt16      ElementType = 0x07
	ElemInt32       ElementType = 0x08
	ElemUInt32      ElementType = 0x09
	ElemInt64       ElementType = 0x0A
	ElemUInt64      ElementType = 0x0B
	ElemFloat32     ElementType = 0x0C
	ElemFloat64     ElementType = 0x0D
	ElemString      ElementType = 0x0E
	ElemPtr         ElementType = 0x0F
	ElemByRef       ElementType = 0x10
	ElemValueType   ElementType = 0x11
	ElemClass       ElementType = 0x12
	ElemVar         ElementType = 0x13
	ElemArray       ElementType = 0x14
	ElemGenericInst ElementType = 0x15
	ElemTypedByRef  ElementType = 0x16
	ElemIntPtr      ElementType = 0x18
	ElemUIntPtr     ElementType = 0x19
	ElemFnPtr       ElementType = 0x1B
	ElemObject      ElementType = 0x1C
	ElemSZArray     ElementType = 0x1D
	ElemMVar        ElementType = 0x1E
	ElemCModReqd    ElementType = 0x1F
	ElemCModOpt     ElementType = 0x20
	ElemSentinel    ElementType = 0x41
)

// TypeSigKind discriminates TypeSig.
type TypeSigKind uint8

const (
	SigPrimitive TypeSigKind = iota // Primitive holds the element type
	SigNamed                        // Namespace/Name refer to a TypeDef/TypeRef
	SigPointer                      // Child is the pointee
	SigArray                        // Child is the element, ArrayLen the fixed size
	SigSZArray                      // Child is the element (no fixed size)
	SigFuncPtr                      // FuncSig holds the target signature
)

// TypeSig is the decoded, recursive form of an ECMA-335 type signature —
// the native structured analogue of the win32json Kind/Child type grammar.
type TypeSig struct {
	Kind      TypeSigKind
	Primitive ElementType // SigPrimitive

	// SigNamed: the referenced type. IsValueType records whether the token
	// used VALUETYPE (struct/enum) or CLASS (COM interface, Attribute, …).
	Namespace   string
	Name        string
	IsValueType bool

	Child    *TypeSig // SigPointer / SigArray / SigSZArray element
	ArrayLen uint32   // SigArray fixed length

	FuncSig *MethodSig // SigFuncPtr target

	// IsConst is set when the signature carried a modreq/modopt of
	// System.Runtime.CompilerServices.IsConst.
	IsConst bool
}

// MethodSig is a decoded MethodDefSig (II.23.2.1).
type MethodSig struct {
	HasThis bool
	Return  TypeSig
	Params  []TypeSig
}

// FieldSignature decodes the FieldSig blob at the given #Blob offset.
func (f *File) FieldSignature(blobOffset uint32) (TypeSig, error) {
	blob := f.Blobs.Get(blobOffset)
	if blob == nil {
		return TypeSig{}, fmt.Errorf("field signature blob 0x%x out of range", blobOffset)
	}
	reader := blobReader{data: blob}
	const fieldSigMarker = 0x06
	if marker := reader.byte(); marker != fieldSigMarker {
		return TypeSig{}, fmt.Errorf("field signature starts with 0x%02x, want 0x06", marker)
	}
	sig := f.decodeTypeSig(&reader)
	if reader.err != nil {
		return TypeSig{}, reader.err
	}
	return sig, nil
}

// MethodSignature decodes the MethodDefSig blob at the given #Blob offset.
func (f *File) MethodSignature(blobOffset uint32) (MethodSig, error) {
	blob := f.Blobs.Get(blobOffset)
	if blob == nil {
		return MethodSig{}, fmt.Errorf("method signature blob 0x%x out of range", blobOffset)
	}
	reader := blobReader{data: blob}
	const sigHasThis = 0x20
	callConv := reader.byte()
	paramCount := reader.compressedUint()
	sig := MethodSig{HasThis: callConv&sigHasThis != 0}
	sig.Return = f.decodeTypeSig(&reader)
	sig.Params = make([]TypeSig, 0, paramCount)
	for i := uint32(0); i < paramCount && !reader.failed(); i++ {
		sig.Params = append(sig.Params, f.decodeTypeSig(&reader))
	}
	if reader.err != nil {
		return MethodSig{}, reader.err
	}
	return sig, nil
}

// decodeTypeSig decodes one Type production (II.23.2.12).
func (f *File) decodeTypeSig(reader *blobReader) TypeSig {
	var isConst bool
	// Skip leading custom modifiers, remembering IsConst.
	for {
		elem := ElementType(reader.peek())
		if elem != ElemCModReqd && elem != ElemCModOpt {
			break
		}
		reader.byte()
		namespace, name := f.resolveTypeToken(reader.compressedUint())
		if name == "IsConst" && namespace == "System.Runtime.CompilerServices" {
			isConst = true
		}
	}

	elem := ElementType(reader.byte())
	sig := TypeSig{IsConst: isConst}
	switch elem {
	case ElemVoid, ElemBoolean, ElemChar,
		ElemInt8, ElemUInt8, ElemInt16, ElemUInt16,
		ElemInt32, ElemUInt32, ElemInt64, ElemUInt64,
		ElemFloat32, ElemFloat64, ElemString, ElemObject,
		ElemIntPtr, ElemUIntPtr, ElemTypedByRef:
		sig.Kind = SigPrimitive
		sig.Primitive = elem

	case ElemPtr, ElemByRef:
		// The winmd projection never uses managed byref for Win32 types,
		// but decode it identically to a pointer for robustness.
		child := f.decodeTypeSig(reader)
		sig.Kind = SigPointer
		sig.Child = &child

	case ElemValueType, ElemClass:
		sig.Kind = SigNamed
		sig.IsValueType = elem == ElemValueType
		sig.Namespace, sig.Name = f.resolveTypeToken(reader.compressedUint())

	case ElemSZArray:
		child := f.decodeTypeSig(reader)
		sig.Kind = SigSZArray
		sig.Child = &child

	case ElemArray:
		// Type Rank NumSizes Size* NumLoBounds LoBound* (II.23.2.13).
		child := f.decodeTypeSig(reader)
		rank := reader.compressedUint()
		numSizes := reader.compressedUint()
		var size uint32
		for i := uint32(0); i < numSizes; i++ {
			s := reader.compressedUint()
			if i == 0 {
				size = s
			}
		}
		numLoBounds := reader.compressedUint()
		for i := uint32(0); i < numLoBounds; i++ {
			reader.compressedUint()
		}
		if rank != 1 {
			reader.setErr(fmt.Sprintf("unsupported array rank %d", rank))
		}
		sig.Kind = SigArray
		sig.Child = &child
		sig.ArrayLen = size

	case ElemFnPtr:
		// Full method signature inline.
		const sigHasThis = 0x20
		callConv := reader.byte()
		paramCount := reader.compressedUint()
		funcSig := MethodSig{HasThis: callConv&sigHasThis != 0}
		funcSig.Return = f.decodeTypeSig(reader)
		for i := uint32(0); i < paramCount && !reader.failed(); i++ {
			funcSig.Params = append(funcSig.Params, f.decodeTypeSig(reader))
		}
		sig.Kind = SigFuncPtr
		sig.FuncSig = &funcSig

	default:
		reader.setErr(fmt.Sprintf("unsupported element type 0x%02x", byte(elem)))
	}
	return sig
}

// resolveTypeToken resolves a TypeDefOrRefEncoded compressed token
// (II.23.2.8) to a (namespace, name) pair.
func (f *File) resolveTypeToken(encoded uint32) (namespace, name string) {
	row := encoded >> 2
	if row == 0 {
		return "", ""
	}
	switch encoded & 0x3 {
	case 0: // TypeDef
		if int(row) <= len(f.Tables.TypeDefs) {
			typeDef := &f.Tables.TypeDefs[row-1]
			return typeDef.Namespace, typeDef.Name
		}
	case 1: // TypeRef
		if int(row) <= len(f.Tables.TypeRefs) {
			typeRef := &f.Tables.TypeRefs[row-1]
			return typeRef.Namespace, typeRef.Name
		}
	case 2: // TypeSpec — not used by the Win32 projection for named types.
	}
	return "", ""
}
