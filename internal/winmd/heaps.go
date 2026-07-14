package winmd

import (
	"encoding/binary"
	"fmt"
)

// StringHeap is the #Strings heap: UTF-8, NUL-terminated strings addressed by
// byte offset (§II.24.2.3).
type StringHeap []byte

// Get returns the string at the given heap offset.
func (h StringHeap) Get(offset uint32) string {
	if offset >= uint32(len(h)) {
		return ""
	}
	return cstring(h[offset:])
}

// GUIDHeap is the #GUID heap: a sequence of 16-byte GUIDs addressed by
// 1-based index (§II.24.2.5).
type GUIDHeap []byte

// Get returns the GUID at the given 1-based index in canonical
// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" form, or "" for index 0.
func (h GUIDHeap) Get(index uint32) string {
	if index == 0 {
		return ""
	}
	offset := (index - 1) * 16
	if offset+16 > uint32(len(h)) {
		return ""
	}
	g := h[offset : offset+16]
	return formatGUID(
		binary.LittleEndian.Uint32(g[0:]),
		binary.LittleEndian.Uint16(g[4:]),
		binary.LittleEndian.Uint16(g[6:]),
		g[8:16],
	)
}

func formatGUID(data1 uint32, data2, data3 uint16, data4 []byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		data1, data2, data3,
		data4[0], data4[1], data4[2], data4[3], data4[4], data4[5], data4[6], data4[7])
}

// BlobHeap is the #Blob heap: length-prefixed binary blobs addressed by byte
// offset (§II.24.2.4).
type BlobHeap []byte

// Get returns the blob at the given heap offset (without its length prefix).
func (h BlobHeap) Get(offset uint32) []byte {
	if offset >= uint32(len(h)) {
		return nil
	}
	b := blobReader{data: h, pos: int(offset)}
	length := b.compressedUint()
	if b.err != nil || b.pos+int(length) > len(h) {
		return nil
	}
	return h[b.pos : b.pos+int(length)]
}

// blobReader is a cursor over blob/signature bytes with ECMA-335 compressed
// integer decoding (§II.23.2).
type blobReader struct {
	data []byte
	pos  int
	err  error
}

func (b *blobReader) failed() bool { return b.err != nil }

func (b *blobReader) remaining() int { return len(b.data) - b.pos }

func (b *blobReader) byte() byte {
	if b.err != nil || b.pos >= len(b.data) {
		b.setErr("unexpected end of blob")
		return 0
	}
	v := b.data[b.pos]
	b.pos++
	return v
}

func (b *blobReader) peek() byte {
	if b.err != nil || b.pos >= len(b.data) {
		return 0
	}
	return b.data[b.pos]
}

func (b *blobReader) bytes(n int) []byte {
	if b.err != nil || b.pos+n > len(b.data) {
		b.setErr("unexpected end of blob")
		return nil
	}
	v := b.data[b.pos : b.pos+n]
	b.pos += n
	return v
}

func (b *blobReader) uint16() uint16 {
	v := b.bytes(2)
	if v == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(v)
}

func (b *blobReader) uint32() uint32 {
	v := b.bytes(4)
	if v == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(v)
}

func (b *blobReader) uint64() uint64 {
	v := b.bytes(8)
	if v == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(v)
}

// compressedUint decodes an ECMA-335 compressed unsigned integer (§II.23.2):
// 1 byte (0xxxxxxx), 2 bytes (10xxxxxx x), or 4 bytes (110xxxxx x x x).
func (b *blobReader) compressedUint() uint32 {
	first := b.byte()
	switch {
	case first&0x80 == 0:
		return uint32(first)
	case first&0xC0 == 0x80:
		second := b.byte()
		return uint32(first&0x3F)<<8 | uint32(second)
	default:
		rest := b.bytes(3)
		if rest == nil {
			return 0
		}
		return uint32(first&0x1F)<<24 | uint32(rest[0])<<16 | uint32(rest[1])<<8 | uint32(rest[2])
	}
}

// serString decodes a custom-attribute SerString (§II.23.3): compressed length
// + UTF-8 bytes; 0xFF means null.
func (b *blobReader) serString() string {
	if b.peek() == 0xFF {
		b.byte()
		return ""
	}
	length := b.compressedUint()
	raw := b.bytes(int(length))
	return string(raw)
}

func (b *blobReader) setErr(msg string) {
	if b.err == nil {
		b.err = fmt.Errorf("%s at offset %d", msg, b.pos)
	}
}
