// Package winmd is a native Go reader for ECMA-335 metadata files (.winmd).
//
// It parses the PE container, the CLI metadata root, the metadata heaps
// (#Strings, #Blob, #GUID), and decodes the metadata tables needed to
// project the Windows.Win32 API surface. No .NET runtime, no cgo.
//
// Section references marked § cite ECMA-335 6th edition, partition II:
// §II.22 (metadata tables), §II.23 (blobs, flags, and signatures), §II.24
// (metadata physical layout), §II.25 (PE file format).
//
// # Non-goals
//
// The reader is deliberately scoped to what the Windows.Win32 projection
// needs. The following are intentional omissions (evaluated against
// github.com/microsoft/go-winmd), not oversights:
//
//   - No dependency on microsoft/go-winmd: it has no tagged releases,
//     depends on x/tools, and lacks the custom-attribute value decoding,
//     Constant decoding, and #- stream handling this projection requires.
//   - No lazy per-row table access: the consumer scans every row of every
//     materialized table, so eager typed slices are simpler and faster.
//   - No per-group coded-index tag types (go-winmd's CodedIndex[T]): coded
//     indices resolve eagerly to a concrete (Table, Row) pair, which is
//     strictly more informative than an encoded tag+index.
//   - No table-layout code generation: tableSchemas is the hand-transcribed
//     §II.22 column layout for all 45 tables.
//   - No #US heap: user strings are IL plumbing, never referenced by winmd
//     projections.
//   - No generics/BYREF/multi-rank-array signature decoding: absent from
//     the Win32 winmd (the brute-force test suites prove it); such
//     constructs fail with a structured error rather than silently
//     mis-decoding.
package winmd

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
)

// File is a parsed .winmd metadata file.
type File struct {
	// Version is the metadata version string from the metadata root
	// (e.g. "v4.0.30319").
	Version string

	Strings StringHeap
	Blobs   BlobHeap
	GUIDs   GUIDHeap

	Tables Tables

	// attrIndex maps attrTarget(Parent) → CustomAttribute row numbers.
	// Built lazily by AttributesFor.
	attrIndex map[uint64][]int
	// methodOwnerIndex maps 1-based MethodDef rows → 1-based TypeDef rows.
	// Built lazily by declaringType.
	methodOwnerIndex []uint32
}

// Open reads and parses the .winmd file at path.
func Open(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("winmd: %w", err)
	}
	file, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("winmd: parsing %s: %w", path, err)
	}
	return file, nil
}

// Parse parses an in-memory .winmd (PE) image.
func Parse(data []byte) (*File, error) {
	metadataRoot, err := locateMetadata(data)
	if err != nil {
		return nil, err
	}
	return parseMetadataRoot(metadataRoot)
}

// locateMetadata walks the PE headers to the CLI (COR20) header (§II.25.3.3) and returns
// the metadata root as a sub-slice of data.
func locateMetadata(data []byte) ([]byte, error) {
	peFile, err := pe.NewFile(newSliceReaderAt(data))
	if err != nil {
		return nil, fmt.Errorf("reading PE container: %w", err)
	}
	defer peFile.Close()

	var comDir pe.DataDirectory
	switch optionalHeader := peFile.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		if optionalHeader.NumberOfRvaAndSizes <= pe.IMAGE_DIRECTORY_ENTRY_COM_DESCRIPTOR {
			return nil, fmt.Errorf("PE optional header has no COM descriptor directory")
		}
		comDir = optionalHeader.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_COM_DESCRIPTOR]
	case *pe.OptionalHeader64:
		if optionalHeader.NumberOfRvaAndSizes <= pe.IMAGE_DIRECTORY_ENTRY_COM_DESCRIPTOR {
			return nil, fmt.Errorf("PE optional header has no COM descriptor directory")
		}
		comDir = optionalHeader.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_COM_DESCRIPTOR]
	default:
		return nil, fmt.Errorf("PE file has no optional header")
	}
	if comDir.VirtualAddress == 0 || comDir.Size == 0 {
		return nil, fmt.Errorf("PE file has no CLI header (not a .winmd/managed image)")
	}

	cliHeader, err := sliceAtRVA(data, peFile, comDir.VirtualAddress, comDir.Size)
	if err != nil {
		return nil, fmt.Errorf("reading CLI header: %w", err)
	}
	// IMAGE_COR20_HEADER (§II.25.3.3): cb(4) MajorRuntimeVersion(2) MinorRuntimeVersion(2)
	// MetaData RVA(4) + Size(4) at offset 8.
	if len(cliHeader) < 16 {
		return nil, fmt.Errorf("CLI header too short: %d bytes", len(cliHeader))
	}
	metadataRVA := binary.LittleEndian.Uint32(cliHeader[8:])
	metadataSize := binary.LittleEndian.Uint32(cliHeader[12:])
	metadataRoot, err := sliceAtRVA(data, peFile, metadataRVA, metadataSize)
	if err != nil {
		return nil, fmt.Errorf("reading metadata root: %w", err)
	}
	return metadataRoot, nil
}

// sliceAtRVA converts an RVA+size to a file-offset sub-slice via the PE
// section table.
func sliceAtRVA(data []byte, peFile *pe.File, rva, size uint32) ([]byte, error) {
	for _, section := range peFile.Sections {
		if rva >= section.VirtualAddress && rva < section.VirtualAddress+section.VirtualSize {
			offset := int64(section.Offset) + int64(rva-section.VirtualAddress)
			end := offset + int64(size)
			if end > int64(len(data)) {
				return nil, fmt.Errorf("RVA 0x%x+0x%x extends past end of file", rva, size)
			}
			return data[offset:end], nil
		}
	}
	return nil, fmt.Errorf("RVA 0x%x not covered by any PE section", rva)
}

// parseMetadataRoot parses the metadata root (§II.24.2.1) and its streams.
func parseMetadataRoot(root []byte) (*File, error) {
	const metadataSignature = 0x424A5342 // "BSJB"
	if len(root) < 20 {
		return nil, fmt.Errorf("metadata root too short: %d bytes", len(root))
	}
	if sig := binary.LittleEndian.Uint32(root); sig != metadataSignature {
		return nil, fmt.Errorf("bad metadata signature 0x%08x (want BSJB)", sig)
	}
	versionLength := binary.LittleEndian.Uint32(root[12:])
	if versionLength > 255 || 16+int(versionLength) > len(root) {
		return nil, fmt.Errorf("implausible metadata version length %d", versionLength)
	}
	version := cstring(root[16 : 16+versionLength])

	// Flags(2) + Streams(2) follow the (4-byte-aligned) version string.
	pos := 16 + int(versionLength)
	if len(root) < pos+4 {
		return nil, fmt.Errorf("metadata root truncated before stream count")
	}
	streamCount := int(binary.LittleEndian.Uint16(root[pos+2:]))
	pos += 4

	file := &File{Version: version}
	var tablesStream []byte
	for i := 0; i < streamCount; i++ {
		if len(root) < pos+8 {
			return nil, fmt.Errorf("stream header %d truncated", i)
		}
		streamOffset := binary.LittleEndian.Uint32(root[pos:])
		streamSize := binary.LittleEndian.Uint32(root[pos+4:])
		pos += 8
		nameStart := pos
		for pos < len(root) && root[pos] != 0 {
			pos++
		}
		name := string(root[nameStart:pos])
		// Name is null-terminated and padded to the next 4-byte boundary.
		pos = nameStart + (pos-nameStart+1+3)&^3

		if int(streamOffset)+int(streamSize) > len(root) {
			return nil, fmt.Errorf("stream %q extends past metadata root", name)
		}
		streamData := root[streamOffset : streamOffset+streamSize]
		switch name {
		case "#~", "#-":
			tablesStream = streamData
		case "#Strings":
			file.Strings = StringHeap(streamData)
		case "#Blob":
			file.Blobs = BlobHeap(streamData)
		case "#GUID":
			file.GUIDs = GUIDHeap(streamData)
		case "#US":
			// User-string heap: unused by winmd projection.
		}
	}
	if tablesStream == nil {
		return nil, fmt.Errorf("metadata root has no #~ tables stream")
	}
	if err := file.Tables.parse(tablesStream, file.Strings, file.Blobs, file.GUIDs); err != nil {
		return nil, err
	}
	return file, nil
}

// cstring returns the bytes up to the first NUL as a string.
func cstring(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// sliceReaderAt adapts a []byte to io.ReaderAt for debug/pe.
type sliceReaderAt struct{ data []byte }

func newSliceReaderAt(data []byte) *sliceReaderAt { return &sliceReaderAt{data: data} }

func (r *sliceReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.data)) {
		return 0, fmt.Errorf("read at 0x%x past end of image", off)
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, fmt.Errorf("short read at 0x%x", off)
	}
	return n, nil
}
