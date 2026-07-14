package winmd

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

// DecodeConstant decodes a Constant-table value blob by its declared element
// type (ECMA-335 §II.22.9). Integers widen to int64/uint64; strings decode
// from UTF-16LE. Returns nil for unsupported types or truncated blobs.
func DecodeConstant(elem ElementType, blob []byte) any {
	need := func(n int) bool { return len(blob) >= n }
	switch elem {
	case ElemBoolean:
		return need(1) && blob[0] != 0
	case ElemChar:
		if !need(2) {
			return nil
		}
		return uint64(binary.LittleEndian.Uint16(blob))
	case ElemInt8:
		if !need(1) {
			return nil
		}
		return int64(int8(blob[0]))
	case ElemUInt8:
		if !need(1) {
			return nil
		}
		return uint64(blob[0])
	case ElemInt16:
		if !need(2) {
			return nil
		}
		return int64(int16(binary.LittleEndian.Uint16(blob)))
	case ElemUInt16:
		if !need(2) {
			return nil
		}
		return uint64(binary.LittleEndian.Uint16(blob))
	case ElemInt32:
		if !need(4) {
			return nil
		}
		return int64(int32(binary.LittleEndian.Uint32(blob)))
	case ElemUInt32:
		if !need(4) {
			return nil
		}
		return uint64(binary.LittleEndian.Uint32(blob))
	case ElemInt64:
		if !need(8) {
			return nil
		}
		return int64(binary.LittleEndian.Uint64(blob))
	case ElemUInt64:
		if !need(8) {
			return nil
		}
		return binary.LittleEndian.Uint64(blob)
	case ElemFloat32:
		if !need(4) {
			return nil
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(blob))
	case ElemFloat64:
		if !need(8) {
			return nil
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(blob))
	case ElemString:
		codeUnits := make([]uint16, len(blob)/2)
		for i := range codeUnits {
			codeUnits[i] = binary.LittleEndian.Uint16(blob[i*2:])
		}
		return string(utf16.Decode(codeUnits))
	}
	return nil
}
