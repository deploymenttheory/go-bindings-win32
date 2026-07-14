package winmd

import "testing"

// blobHeapWith builds a #Blob heap holding a single blob at offset 0.
func blobHeapWith(blob []byte) BlobHeap {
	if len(blob) >= 0x80 {
		panic("test blob too long for a 1-byte length prefix")
	}
	return BlobHeap(append([]byte{byte(len(blob))}, blob...))
}

// TestResolveAttributeCtorHostileRows feeds resolveAttributeCtor coded
// indices that a corrupt file could carry: zero rows (would underflow the
// Row-1 indexing) and out-of-range rows. All must error, never panic.
func TestResolveAttributeCtorHostileRows(t *testing.T) {
	file := &File{}
	file.Tables.Methods = []MethodDefRow{{Name: "ctor"}}
	file.Tables.MemberRefs = []MemberRefRow{
		{Class: CodedIndex{Table: TableTypeRef, Row: 99}, Name: "bad-typeref"},
		{Class: CodedIndex{Table: TableTypeDef, Row: 99}, Name: "bad-typedef"},
	}

	cases := []struct {
		name string
		ctor CodedIndex
	}{
		{"MethodDef row 0", CodedIndex{Table: TableMethodDef, Row: 0}},
		{"MethodDef row out of range", CodedIndex{Table: TableMethodDef, Row: 7}},
		{"MemberRef row 0", CodedIndex{Table: TableMemberRef, Row: 0}},
		{"MemberRef row out of range", CodedIndex{Table: TableMemberRef, Row: 7}},
		{"MemberRef parent TypeRef out of range", CodedIndex{Table: TableMemberRef, Row: 1}},
		{"MemberRef parent TypeDef out of range", CodedIndex{Table: TableMemberRef, Row: 2}},
		{"unsupported table", CodedIndex{Table: TableParam, Row: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, _, err := file.resolveAttributeCtor(tc.ctor); err == nil {
				t.Fatalf("resolveAttributeCtor(%+v) succeeded, want error", tc.ctor)
			}
		})
	}
}

// TestMethodSignatureHostileParamCount decodes a signature blob whose
// compressed param count claims 2²⁹−1 params but whose body is 3 bytes.
// The decode must fail with an error and must not allocate for the claimed
// count (the capacity is clamped to the bytes remaining).
func TestMethodSignatureHostileParamCount(t *testing.T) {
	// callConv=0x00, paramCount=0x1FFFFFFF (4-byte compressed 0xDF FF FF FF),
	// return type Void, then nothing — every param read hits end-of-blob.
	file := &File{Blobs: blobHeapWith([]byte{0x00, 0xDF, 0xFF, 0xFF, 0xFF, byte(ElemVoid)})}
	if _, err := file.MethodSignature(0); err == nil {
		t.Fatal("MethodSignature succeeded on truncated 2²⁹-param blob, want error")
	}
}

// TestFixedArgHostileArrayCount decodes an SZARRAY fixed argument whose count
// claims 2³²−2 elements with a 2-byte body. The reader must stop with an
// error and the clamped allocation must not track the claimed count.
func TestFixedArgHostileArrayCount(t *testing.T) {
	file := &File{}
	sig := TypeSig{Kind: SigSZArray, Child: &TypeSig{Kind: SigPrimitive, Primitive: ElemUInt8}}
	reader := blobReader{data: []byte{0xFE, 0xFF, 0xFF, 0xFF, 0xAB, 0xCD}}
	value := file.readFixedArg(&reader, &sig)
	if !reader.failed() {
		t.Fatal("readFixedArg consumed a 2³²−2-element array from 2 bytes without error")
	}
	// At most remaining+1 elements: one per available byte plus the final
	// failing read that appends a zero value before the loop notices the error.
	if values, ok := value.([]any); ok && len(values) > 3 {
		t.Fatalf("decoded %d elements from 2 bytes", len(values))
	}
}

// TestBlobHeapHostileLength reads a blob whose compressed length runs past
// the end of the heap; Get must return nil rather than slicing out of range.
func TestBlobHeapHostileLength(t *testing.T) {
	heap := BlobHeap{0x7F, 0x01, 0x02} // claims 127 bytes, has 2
	if got := heap.Get(0); got != nil {
		t.Fatalf("BlobHeap.Get returned %d bytes from a truncated blob, want nil", len(got))
	}
}
