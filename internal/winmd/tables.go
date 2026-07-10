package winmd

import (
	"encoding/binary"
	"fmt"
	"math/bits"
)

// Table IDs (ECMA-335 II.22).
const (
	tableModule                 = 0x00
	tableTypeRef                = 0x01
	tableTypeDef                = 0x02
	tableFieldPtr               = 0x03
	tableField                  = 0x04
	tableMethodPtr              = 0x05
	tableMethodDef              = 0x06
	tableParamPtr               = 0x07
	tableParam                  = 0x08
	tableInterfaceImpl          = 0x09
	tableMemberRef              = 0x0A
	tableConstant               = 0x0B
	tableCustomAttribute        = 0x0C
	tableFieldMarshal           = 0x0D
	tableDeclSecurity           = 0x0E
	tableClassLayout            = 0x0F
	tableFieldLayout            = 0x10
	tableStandAloneSig          = 0x11
	tableEventMap               = 0x12
	tableEventPtr               = 0x13
	tableEvent                  = 0x14
	tablePropertyMap            = 0x15
	tablePropertyPtr            = 0x16
	tableProperty               = 0x17
	tableMethodSemantics        = 0x18
	tableMethodImpl             = 0x19
	tableModuleRef              = 0x1A
	tableTypeSpec               = 0x1B
	tableImplMap                = 0x1C
	tableFieldRVA               = 0x1D
	tableAssembly               = 0x20
	tableAssemblyProcessor      = 0x21
	tableAssemblyOS             = 0x22
	tableAssemblyRef            = 0x23
	tableAssemblyRefProcessor   = 0x24
	tableAssemblyRefOS          = 0x25
	tableFile                   = 0x26
	tableExportedType           = 0x27
	tableManifestResource       = 0x28
	tableNestedClass            = 0x29
	tableGenericParam           = 0x2A
	tableMethodSpec             = 0x2B
	tableGenericParamConstraint = 0x2C

	tableCount = 0x2D
	// nullTable marks a null coded index target.
	nullTable = 0xFF
)

// Exported table IDs for consumers interpreting CodedIndex targets.
const (
	TableTypeRef   = tableTypeRef
	TableTypeDef   = tableTypeDef
	TableField     = tableField
	TableMethodDef = tableMethodDef
	TableParam     = tableParam
	TableMemberRef = tableMemberRef
	TableModuleRef = tableModuleRef
)

// Coded index groups (II.24.2.6).
const (
	codedTypeDefOrRef = iota
	codedHasConstant
	codedHasCustomAttribute
	codedHasFieldMarshal
	codedHasDeclSecurity
	codedMemberRefParent
	codedHasSemantics
	codedMethodDefOrRef
	codedMemberForwarded
	codedImplementation
	codedCustomAttributeType
	codedResolutionScope
	codedTypeOrMethodDef
	codedGroupCount
)

// codedGroups maps each coded-index group to its member tables in tag order.
// nullTable entries are tags that no table occupies.
var codedGroups = [codedGroupCount][]uint8{
	codedTypeDefOrRef:        {tableTypeDef, tableTypeRef, tableTypeSpec},
	codedHasConstant:         {tableField, tableParam, tableProperty},
	codedHasCustomAttribute:  {tableMethodDef, tableField, tableTypeRef, tableTypeDef, tableParam, tableInterfaceImpl, tableMemberRef, tableModule, tableDeclSecurity, tableProperty, tableEvent, tableStandAloneSig, tableModuleRef, tableTypeSpec, tableAssembly, tableAssemblyRef, tableFile, tableExportedType, tableManifestResource, tableGenericParam, tableGenericParamConstraint, tableMethodSpec},
	codedHasFieldMarshal:     {tableField, tableParam},
	codedHasDeclSecurity:     {tableTypeDef, tableMethodDef, tableAssembly},
	codedMemberRefParent:     {tableTypeDef, tableTypeRef, tableModuleRef, tableMethodDef, tableTypeSpec},
	codedHasSemantics:        {tableEvent, tableProperty},
	codedMethodDefOrRef:      {tableMethodDef, tableMemberRef},
	codedMemberForwarded:     {tableField, tableMethodDef},
	codedImplementation:      {tableFile, tableAssemblyRef, tableExportedType},
	codedCustomAttributeType: {nullTable, nullTable, tableMethodDef, tableMemberRef, nullTable},
	codedResolutionScope:     {tableModule, tableModuleRef, tableAssemblyRef, tableTypeRef},
	codedTypeOrMethodDef:     {tableTypeDef, tableMethodDef},
}

// Column kinds for table schemas.
type columnKind uint8

const (
	colUint16 columnKind = iota
	colUint32
	colString // #Strings index
	colGUID   // #GUID index
	colBlob   // #Blob index
	colIndex  // simple table index; aux = table ID
	colCoded  // coded index; aux = coded group
)

type column struct {
	kind columnKind
	aux  uint8
}

// tableSchemas defines every ECMA-335 table's columns so that row sizes are
// computed correctly even for tables this reader never materializes.
var tableSchemas = [tableCount][]column{
	tableModule:                 {{colUint16, 0}, {colString, 0}, {colGUID, 0}, {colGUID, 0}, {colGUID, 0}},
	tableTypeRef:                {{colCoded, codedResolutionScope}, {colString, 0}, {colString, 0}},
	tableTypeDef:                {{colUint32, 0}, {colString, 0}, {colString, 0}, {colCoded, codedTypeDefOrRef}, {colIndex, tableField}, {colIndex, tableMethodDef}},
	tableFieldPtr:               {{colIndex, tableField}},
	tableField:                  {{colUint16, 0}, {colString, 0}, {colBlob, 0}},
	tableMethodPtr:              {{colIndex, tableMethodDef}},
	tableMethodDef:              {{colUint32, 0}, {colUint16, 0}, {colUint16, 0}, {colString, 0}, {colBlob, 0}, {colIndex, tableParam}},
	tableParamPtr:               {{colIndex, tableParam}},
	tableParam:                  {{colUint16, 0}, {colUint16, 0}, {colString, 0}},
	tableInterfaceImpl:          {{colIndex, tableTypeDef}, {colCoded, codedTypeDefOrRef}},
	tableMemberRef:              {{colCoded, codedMemberRefParent}, {colString, 0}, {colBlob, 0}},
	tableConstant:               {{colUint16, 0}, {colCoded, codedHasConstant}, {colBlob, 0}},
	tableCustomAttribute:        {{colCoded, codedHasCustomAttribute}, {colCoded, codedCustomAttributeType}, {colBlob, 0}},
	tableFieldMarshal:           {{colCoded, codedHasFieldMarshal}, {colBlob, 0}},
	tableDeclSecurity:           {{colUint16, 0}, {colCoded, codedHasDeclSecurity}, {colBlob, 0}},
	tableClassLayout:            {{colUint16, 0}, {colUint32, 0}, {colIndex, tableTypeDef}},
	tableFieldLayout:            {{colUint32, 0}, {colIndex, tableField}},
	tableStandAloneSig:          {{colBlob, 0}},
	tableEventMap:               {{colIndex, tableTypeDef}, {colIndex, tableEvent}},
	tableEventPtr:               {{colIndex, tableEvent}},
	tableEvent:                  {{colUint16, 0}, {colString, 0}, {colCoded, codedTypeDefOrRef}},
	tablePropertyMap:            {{colIndex, tableTypeDef}, {colIndex, tableProperty}},
	tablePropertyPtr:            {{colIndex, tableProperty}},
	tableProperty:               {{colUint16, 0}, {colString, 0}, {colBlob, 0}},
	tableMethodSemantics:        {{colUint16, 0}, {colIndex, tableMethodDef}, {colCoded, codedHasSemantics}},
	tableMethodImpl:             {{colIndex, tableTypeDef}, {colCoded, codedMethodDefOrRef}, {colCoded, codedMethodDefOrRef}},
	tableModuleRef:              {{colString, 0}},
	tableTypeSpec:               {{colBlob, 0}},
	tableImplMap:                {{colUint16, 0}, {colCoded, codedMemberForwarded}, {colString, 0}, {colIndex, tableModuleRef}},
	tableFieldRVA:               {{colUint32, 0}, {colIndex, tableField}},
	tableAssembly:               {{colUint32, 0}, {colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint32, 0}, {colBlob, 0}, {colString, 0}, {colString, 0}},
	tableAssemblyProcessor:      {{colUint32, 0}},
	tableAssemblyOS:             {{colUint32, 0}, {colUint32, 0}, {colUint32, 0}},
	tableAssemblyRef:            {{colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint32, 0}, {colBlob, 0}, {colString, 0}, {colString, 0}, {colBlob, 0}},
	tableAssemblyRefProcessor:   {{colUint32, 0}, {colIndex, tableAssemblyRef}},
	tableAssemblyRefOS:          {{colUint32, 0}, {colUint32, 0}, {colUint32, 0}, {colIndex, tableAssemblyRef}},
	tableFile:                   {{colUint32, 0}, {colString, 0}, {colBlob, 0}},
	tableExportedType:           {{colUint32, 0}, {colUint32, 0}, {colString, 0}, {colString, 0}, {colCoded, codedImplementation}},
	tableManifestResource:       {{colUint32, 0}, {colUint32, 0}, {colString, 0}, {colCoded, codedImplementation}},
	tableNestedClass:            {{colIndex, tableTypeDef}, {colIndex, tableTypeDef}},
	tableGenericParam:           {{colUint16, 0}, {colUint16, 0}, {colCoded, codedTypeOrMethodDef}, {colString, 0}},
	tableMethodSpec:             {{colCoded, codedMethodDefOrRef}, {colBlob, 0}},
	tableGenericParamConstraint: {{colIndex, tableGenericParam}, {colCoded, codedTypeDefOrRef}},
}

// CodedIndex is a resolved coded index: a (table, 1-based row) pair.
// A zero Row means null; Table is nullTable in that case.
type CodedIndex struct {
	Table uint8
	Row   uint32
}

// IsNull reports whether the coded index refers to nothing.
func (c CodedIndex) IsNull() bool { return c.Row == 0 }

// Typed rows for the tables the projection consumes. String columns are
// resolved eagerly; blob columns keep the heap offset (decoded on demand).

type TypeRefRow struct {
	ResolutionScope CodedIndex
	Name            string
	Namespace       string
}

type TypeDefRow struct {
	Flags     uint32
	Name      string
	Namespace string
	Extends   CodedIndex
	// FieldFirst/FieldEnd and MethodFirst/MethodEnd are 1-based half-open
	// row ranges into Fields and Methods.
	FieldFirst, FieldEnd   uint32
	MethodFirst, MethodEnd uint32
}

type FieldRow struct {
	Flags     uint16
	Name      string
	Signature uint32 // #Blob offset
}

type MethodDefRow struct {
	RVA       uint32
	ImplFlags uint16
	Flags     uint16
	Name      string
	Signature uint32 // #Blob offset
	// ParamFirst/ParamEnd is a 1-based half-open row range into Params.
	ParamFirst, ParamEnd uint32
}

type ParamRow struct {
	Flags    uint16
	Sequence uint16
	Name     string
}

type InterfaceImplRow struct {
	Class     uint32 // TypeDef row
	Interface CodedIndex
}

type MemberRefRow struct {
	Class     CodedIndex
	Name      string
	Signature uint32 // #Blob offset
}

type ConstantRow struct {
	Type   uint16 // ELEMENT_TYPE_* of the value
	Parent CodedIndex
	Value  uint32 // #Blob offset
}

type CustomAttributeRow struct {
	Parent CodedIndex
	Type   CodedIndex // MethodDef or MemberRef of the .ctor
	Value  uint32     // #Blob offset
}

type ClassLayoutRow struct {
	PackingSize uint16
	ClassSize   uint32
	Parent      uint32 // TypeDef row
}

type FieldLayoutRow struct {
	Offset uint32
	Field  uint32 // Field row
}

type ImplMapRow struct {
	MappingFlags    uint16
	MemberForwarded CodedIndex
	ImportName      string
	ImportScope     uint32 // ModuleRef row
}

type NestedClassRow struct {
	NestedClass    uint32 // TypeDef row
	EnclosingClass uint32 // TypeDef row
}

// Tables holds the decoded metadata tables.
type Tables struct {
	rowCounts [tableCount]uint32

	TypeRefs         []TypeRefRow
	TypeDefs         []TypeDefRow
	Fields           []FieldRow
	Methods          []MethodDefRow
	Params           []ParamRow
	InterfaceImpls   []InterfaceImplRow
	MemberRefs       []MemberRefRow
	Constants        []ConstantRow
	CustomAttributes []CustomAttributeRow
	ClassLayouts     []ClassLayoutRow
	FieldLayouts     []FieldLayoutRow
	ModuleRefs       []string
	TypeSpecs        []uint32 // #Blob offsets
	ImplMaps         []ImplMapRow
	NestedClasses    []NestedClassRow
}

// tableDecoder walks the raw #~ table data with pre-computed column sizes.
type tableDecoder struct {
	data       []byte
	pos        int
	rowCounts  [tableCount]uint32
	stringWide bool
	guidWide   bool
	blobWide   bool
	codedWide  [codedGroupCount]bool
	strings    StringHeap
	err        error
}

func (t *Tables) parse(stream []byte, strings StringHeap, blobs BlobHeap, guids GUIDHeap) error {
	// #~ header (II.24.2.6): Reserved(4) MajorVersion(1) MinorVersion(1)
	// HeapSizes(1) Reserved(1) Valid(8) Sorted(8) Rows(4*n) Tables.
	if len(stream) < 24 {
		return fmt.Errorf("#~ stream too short: %d bytes", len(stream))
	}
	heapSizes := stream[6]
	valid := binary.LittleEndian.Uint64(stream[8:])
	pos := 24

	decoder := &tableDecoder{
		data:       stream,
		stringWide: heapSizes&0x01 != 0,
		guidWide:   heapSizes&0x02 != 0,
		blobWide:   heapSizes&0x04 != 0,
		strings:    strings,
	}
	presentCount := bits.OnesCount64(valid)
	if len(stream) < pos+4*presentCount {
		return fmt.Errorf("#~ stream truncated in row counts")
	}
	for tableID := 0; tableID < 64; tableID++ {
		if valid&(1<<tableID) == 0 {
			continue
		}
		count := binary.LittleEndian.Uint32(stream[pos:])
		pos += 4
		if tableID >= tableCount {
			return fmt.Errorf("#~ stream declares unknown table 0x%02x", tableID)
		}
		decoder.rowCounts[tableID] = count
	}
	decoder.pos = pos
	t.rowCounts = decoder.rowCounts

	// Coded index widths depend on the max row count in each group.
	for group, members := range codedGroups {
		tagBits := bits.Len(uint(len(members) - 1))
		var maxRows uint32
		for _, member := range members {
			if member != nullTable && decoder.rowCounts[member] > maxRows {
				maxRows = decoder.rowCounts[member]
			}
		}
		decoder.codedWide[group] = maxRows >= 1<<(16-tagBits)
	}

	// Decode tables in ID order; skip the ones the projection never reads.
	for tableID := uint8(0); tableID < tableCount; tableID++ {
		count := int(decoder.rowCounts[tableID])
		if count == 0 {
			continue
		}
		switch tableID {
		case tableTypeRef:
			t.TypeRefs = decodeRows(decoder, tableID, count, func(r *rowReader) TypeRefRow {
				return TypeRefRow{
					ResolutionScope: r.coded(codedResolutionScope),
					Name:            r.string(),
					Namespace:       r.string(),
				}
			})
		case tableTypeDef:
			t.TypeDefs = decodeRows(decoder, tableID, count, func(r *rowReader) TypeDefRow {
				return TypeDefRow{
					Flags:       r.uint32(),
					Name:        r.string(),
					Namespace:   r.string(),
					Extends:     r.coded(codedTypeDefOrRef),
					FieldFirst:  r.index(tableField),
					MethodFirst: r.index(tableMethodDef),
				}
			})
		case tableField:
			t.Fields = decodeRows(decoder, tableID, count, func(r *rowReader) FieldRow {
				return FieldRow{Flags: r.uint16(), Name: r.string(), Signature: r.blob()}
			})
		case tableMethodDef:
			t.Methods = decodeRows(decoder, tableID, count, func(r *rowReader) MethodDefRow {
				return MethodDefRow{
					RVA:        r.uint32(),
					ImplFlags:  r.uint16(),
					Flags:      r.uint16(),
					Name:       r.string(),
					Signature:  r.blob(),
					ParamFirst: r.index(tableParam),
				}
			})
		case tableParam:
			t.Params = decodeRows(decoder, tableID, count, func(r *rowReader) ParamRow {
				return ParamRow{Flags: r.uint16(), Sequence: r.uint16(), Name: r.string()}
			})
		case tableInterfaceImpl:
			t.InterfaceImpls = decodeRows(decoder, tableID, count, func(r *rowReader) InterfaceImplRow {
				return InterfaceImplRow{Class: r.index(tableTypeDef), Interface: r.coded(codedTypeDefOrRef)}
			})
		case tableMemberRef:
			t.MemberRefs = decodeRows(decoder, tableID, count, func(r *rowReader) MemberRefRow {
				return MemberRefRow{Class: r.coded(codedMemberRefParent), Name: r.string(), Signature: r.blob()}
			})
		case tableConstant:
			t.Constants = decodeRows(decoder, tableID, count, func(r *rowReader) ConstantRow {
				return ConstantRow{Type: r.uint16(), Parent: r.coded(codedHasConstant), Value: r.blob()}
			})
		case tableCustomAttribute:
			t.CustomAttributes = decodeRows(decoder, tableID, count, func(r *rowReader) CustomAttributeRow {
				return CustomAttributeRow{
					Parent: r.coded(codedHasCustomAttribute),
					Type:   r.coded(codedCustomAttributeType),
					Value:  r.blob(),
				}
			})
		case tableClassLayout:
			t.ClassLayouts = decodeRows(decoder, tableID, count, func(r *rowReader) ClassLayoutRow {
				return ClassLayoutRow{PackingSize: r.uint16(), ClassSize: r.uint32(), Parent: r.index(tableTypeDef)}
			})
		case tableFieldLayout:
			t.FieldLayouts = decodeRows(decoder, tableID, count, func(r *rowReader) FieldLayoutRow {
				return FieldLayoutRow{Offset: r.uint32(), Field: r.index(tableField)}
			})
		case tableModuleRef:
			t.ModuleRefs = decodeRows(decoder, tableID, count, func(r *rowReader) string {
				return r.string()
			})
		case tableTypeSpec:
			t.TypeSpecs = decodeRows(decoder, tableID, count, func(r *rowReader) uint32 {
				return r.blob()
			})
		case tableImplMap:
			t.ImplMaps = decodeRows(decoder, tableID, count, func(r *rowReader) ImplMapRow {
				return ImplMapRow{
					MappingFlags:    r.uint16(),
					MemberForwarded: r.coded(codedMemberForwarded),
					ImportName:      r.string(),
					ImportScope:     r.index(tableModuleRef),
				}
			})
		case tableNestedClass:
			t.NestedClasses = decodeRows(decoder, tableID, count, func(r *rowReader) NestedClassRow {
				return NestedClassRow{NestedClass: r.index(tableTypeDef), EnclosingClass: r.index(tableTypeDef)}
			})
		default:
			decoder.skipTable(tableID, count)
		}
		if decoder.err != nil {
			return fmt.Errorf("decoding table 0x%02x: %w", tableID, decoder.err)
		}
	}

	// Resolve the half-open list ranges now that all row counts are known.
	fixListRanges(t.TypeDefs, uint32(len(t.Fields)), func(row *TypeDefRow) (*uint32, *uint32) {
		return &row.FieldFirst, &row.FieldEnd
	})
	fixListRanges(t.TypeDefs, uint32(len(t.Methods)), func(row *TypeDefRow) (*uint32, *uint32) {
		return &row.MethodFirst, &row.MethodEnd
	})
	fixListRanges(t.Methods, uint32(len(t.Params)), func(row *MethodDefRow) (*uint32, *uint32) {
		return &row.ParamFirst, &row.ParamEnd
	})
	return nil
}

// rowSize computes the byte width of one row of the given table.
func (d *tableDecoder) rowSize(tableID uint8) int {
	size := 0
	for _, col := range tableSchemas[tableID] {
		size += d.columnSize(col)
	}
	return size
}

func (d *tableDecoder) columnSize(col column) int {
	switch col.kind {
	case colUint16:
		return 2
	case colUint32:
		return 4
	case colString:
		if d.stringWide {
			return 4
		}
		return 2
	case colGUID:
		if d.guidWide {
			return 4
		}
		return 2
	case colBlob:
		if d.blobWide {
			return 4
		}
		return 2
	case colIndex:
		if d.rowCounts[col.aux] > 0xFFFF {
			return 4
		}
		return 2
	case colCoded:
		if d.codedWide[col.aux] {
			return 4
		}
		return 2
	}
	panic("unreachable column kind")
}

func (d *tableDecoder) skipTable(tableID uint8, count int) {
	total := d.rowSize(tableID) * count
	if d.pos+total > len(d.data) {
		d.err = fmt.Errorf("table data truncated (need %d bytes at %d)", total, d.pos)
		return
	}
	d.pos += total
}

// decodeRows decodes count rows of tableID via the per-row builder.
func decodeRows[T any](d *tableDecoder, tableID uint8, count int, build func(*rowReader) T) []T {
	if d.err != nil {
		return nil
	}
	rowSize := d.rowSize(tableID)
	if d.pos+rowSize*count > len(d.data) {
		d.err = fmt.Errorf("table data truncated (need %d rows × %d bytes at %d)", count, rowSize, d.pos)
		return nil
	}
	rows := make([]T, count)
	reader := rowReader{decoder: d}
	for i := 0; i < count; i++ {
		reader.pos = d.pos + i*rowSize
		rows[i] = build(&reader)
	}
	d.pos += rowSize * count
	return rows
}

// rowReader reads one row's columns in schema order.
type rowReader struct {
	decoder *tableDecoder
	pos     int
}

func (r *rowReader) uint16() uint16 {
	v := binary.LittleEndian.Uint16(r.decoder.data[r.pos:])
	r.pos += 2
	return v
}

func (r *rowReader) uint32() uint32 {
	v := binary.LittleEndian.Uint32(r.decoder.data[r.pos:])
	r.pos += 4
	return v
}

func (r *rowReader) narrowOrWide(wide bool) uint32 {
	if wide {
		return r.uint32()
	}
	return uint32(r.uint16())
}

func (r *rowReader) string() string {
	offset := r.narrowOrWide(r.decoder.stringWide)
	return r.decoder.strings.Get(offset)
}

func (r *rowReader) blob() uint32 {
	return r.narrowOrWide(r.decoder.blobWide)
}

func (r *rowReader) index(tableID uint8) uint32 {
	return r.narrowOrWide(r.decoder.rowCounts[tableID] > 0xFFFF)
}

func (r *rowReader) coded(group uint8) CodedIndex {
	raw := r.narrowOrWide(r.decoder.codedWide[group])
	members := codedGroups[group]
	tagBits := bits.Len(uint(len(members) - 1))
	tag := raw & (1<<tagBits - 1)
	row := raw >> tagBits
	if int(tag) >= len(members) || members[tag] == nullTable || row == 0 {
		return CodedIndex{Table: nullTable, Row: 0}
	}
	return CodedIndex{Table: members[tag], Row: row}
}

// fixListRanges converts ECMA-335 "list" columns (start index of a run that
// ends where the next row's run begins) into explicit half-open ranges.
func fixListRanges[T any](rows []T, totalRows uint32, access func(*T) (first, end *uint32)) {
	for i := range rows {
		first, end := access(&rows[i])
		if i+1 < len(rows) {
			next, _ := access(&rows[i+1])
			*end = *next
		} else {
			*end = totalRows + 1
		}
		// Clamp degenerate ranges (null list → first==end).
		if *first == 0 || *first > *end {
			*first = *end
		}
	}
}
