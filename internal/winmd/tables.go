package winmd

import (
	"encoding/binary"
	"fmt"
	"math/bits"
)

// Table identifies an ECMA-335 metadata table (§II.22).
type Table uint8

// Table IDs in specification order and naming (§II.22).
const (
	TableModule                 Table = 0x00 // §II.22.30
	TableTypeRef                Table = 0x01 // §II.22.38
	TableTypeDef                Table = 0x02 // §II.22.37
	TableFieldPtr               Table = 0x03 // §II.24.2.6 (#- indirection)
	TableField                  Table = 0x04 // §II.22.15
	TableMethodPtr              Table = 0x05 // §II.24.2.6 (#- indirection)
	TableMethodDef              Table = 0x06 // §II.22.26
	TableParamPtr               Table = 0x07 // §II.24.2.6 (#- indirection)
	TableParam                  Table = 0x08 // §II.22.33
	TableInterfaceImpl          Table = 0x09 // §II.22.23
	TableMemberRef              Table = 0x0A // §II.22.25
	TableConstant               Table = 0x0B // §II.22.9
	TableCustomAttribute        Table = 0x0C // §II.22.10
	TableFieldMarshal           Table = 0x0D // §II.22.17
	TableDeclSecurity           Table = 0x0E // §II.22.11
	TableClassLayout            Table = 0x0F // §II.22.8
	TableFieldLayout            Table = 0x10 // §II.22.16
	TableStandAloneSig          Table = 0x11 // §II.22.36
	TableEventMap               Table = 0x12 // §II.22.12
	TableEventPtr               Table = 0x13 // §II.24.2.6 (#- indirection)
	TableEvent                  Table = 0x14 // §II.22.13
	TablePropertyMap            Table = 0x15 // §II.22.35
	TablePropertyPtr            Table = 0x16 // §II.24.2.6 (#- indirection)
	TableProperty               Table = 0x17 // §II.22.34
	TableMethodSemantics        Table = 0x18 // §II.22.28
	TableMethodImpl             Table = 0x19 // §II.22.27
	TableModuleRef              Table = 0x1A // §II.22.31
	TableTypeSpec               Table = 0x1B // §II.22.39
	TableImplMap                Table = 0x1C // §II.22.22
	TableFieldRVA               Table = 0x1D // §II.22.18
	TableAssembly               Table = 0x20 // §II.22.2
	TableAssemblyProcessor      Table = 0x21 // §II.22.4
	TableAssemblyOS             Table = 0x22 // §II.22.3
	TableAssemblyRef            Table = 0x23 // §II.22.5
	TableAssemblyRefProcessor   Table = 0x24 // §II.22.7
	TableAssemblyRefOS          Table = 0x25 // §II.22.6
	TableFile                   Table = 0x26 // §II.22.19
	TableExportedType           Table = 0x27 // §II.22.14
	TableManifestResource       Table = 0x28 // §II.22.24
	TableNestedClass            Table = 0x29 // §II.22.32
	TableGenericParam           Table = 0x2A // §II.22.20
	TableMethodSpec             Table = 0x2B // §II.22.29
	TableGenericParamConstraint Table = 0x2C // §II.22.21

	// TableNull marks a null coded-index target.
	TableNull Table = 0xFF
)

// tableCount is one past the highest defined table ID.
const tableCount = 0x2D

// tableNames indexes spec table names by ID for Table.String.
var tableNames = [tableCount]string{
	TableModule:                 "Module",
	TableTypeRef:                "TypeRef",
	TableTypeDef:                "TypeDef",
	TableFieldPtr:               "FieldPtr",
	TableField:                  "Field",
	TableMethodPtr:              "MethodPtr",
	TableMethodDef:              "MethodDef",
	TableParamPtr:               "ParamPtr",
	TableParam:                  "Param",
	TableInterfaceImpl:          "InterfaceImpl",
	TableMemberRef:              "MemberRef",
	TableConstant:               "Constant",
	TableCustomAttribute:        "CustomAttribute",
	TableFieldMarshal:           "FieldMarshal",
	TableDeclSecurity:           "DeclSecurity",
	TableClassLayout:            "ClassLayout",
	TableFieldLayout:            "FieldLayout",
	TableStandAloneSig:          "StandAloneSig",
	TableEventMap:               "EventMap",
	TableEventPtr:               "EventPtr",
	TableEvent:                  "Event",
	TablePropertyMap:            "PropertyMap",
	TablePropertyPtr:            "PropertyPtr",
	TableProperty:               "Property",
	TableMethodSemantics:        "MethodSemantics",
	TableMethodImpl:             "MethodImpl",
	TableModuleRef:              "ModuleRef",
	TableTypeSpec:               "TypeSpec",
	TableImplMap:                "ImplMap",
	TableFieldRVA:               "FieldRVA",
	TableAssembly:               "Assembly",
	TableAssemblyProcessor:      "AssemblyProcessor",
	TableAssemblyOS:             "AssemblyOS",
	TableAssemblyRef:            "AssemblyRef",
	TableAssemblyRefProcessor:   "AssemblyRefProcessor",
	TableAssemblyRefOS:          "AssemblyRefOS",
	TableFile:                   "File",
	TableExportedType:           "ExportedType",
	TableManifestResource:       "ManifestResource",
	TableNestedClass:            "NestedClass",
	TableGenericParam:           "GenericParam",
	TableMethodSpec:             "MethodSpec",
	TableGenericParamConstraint: "GenericParamConstraint",
}

// String returns the specification name of the table (§II.22).
func (t Table) String() string {
	if t == TableNull {
		return "Null"
	}
	if int(t) < len(tableNames) && tableNames[t] != "" {
		return tableNames[t]
	}
	return fmt.Sprintf("Table(0x%02X)", uint8(t))
}

// Coded index groups (§II.24.2.6).
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
// TableNull entries are tags that no table occupies.
var codedGroups = [codedGroupCount][]Table{
	codedTypeDefOrRef:        {TableTypeDef, TableTypeRef, TableTypeSpec},
	codedHasConstant:         {TableField, TableParam, TableProperty},
	codedHasCustomAttribute:  {TableMethodDef, TableField, TableTypeRef, TableTypeDef, TableParam, TableInterfaceImpl, TableMemberRef, TableModule, TableDeclSecurity, TableProperty, TableEvent, TableStandAloneSig, TableModuleRef, TableTypeSpec, TableAssembly, TableAssemblyRef, TableFile, TableExportedType, TableManifestResource, TableGenericParam, TableGenericParamConstraint, TableMethodSpec},
	codedHasFieldMarshal:     {TableField, TableParam},
	codedHasDeclSecurity:     {TableTypeDef, TableMethodDef, TableAssembly},
	codedMemberRefParent:     {TableTypeDef, TableTypeRef, TableModuleRef, TableMethodDef, TableTypeSpec},
	codedHasSemantics:        {TableEvent, TableProperty},
	codedMethodDefOrRef:      {TableMethodDef, TableMemberRef},
	codedMemberForwarded:     {TableField, TableMethodDef},
	codedImplementation:      {TableFile, TableAssemblyRef, TableExportedType},
	codedCustomAttributeType: {TableNull, TableNull, TableMethodDef, TableMemberRef, TableNull},
	codedResolutionScope:     {TableModule, TableModuleRef, TableAssemblyRef, TableTypeRef},
	codedTypeOrMethodDef:     {TableTypeDef, TableMethodDef},
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

// column describes one schema column. aux is the colIndex target table, or
// the colCoded group (stored in the same field; disambiguated by kind).
type column struct {
	kind columnKind
	aux  Table
}

// tableSchemas transcribes every §II.22 table's columns so that row sizes are
// computed correctly even for tables this reader never materializes.
var tableSchemas = [tableCount][]column{
	TableModule:                 {{colUint16, 0}, {colString, 0}, {colGUID, 0}, {colGUID, 0}, {colGUID, 0}},
	TableTypeRef:                {{colCoded, codedResolutionScope}, {colString, 0}, {colString, 0}},
	TableTypeDef:                {{colUint32, 0}, {colString, 0}, {colString, 0}, {colCoded, codedTypeDefOrRef}, {colIndex, TableField}, {colIndex, TableMethodDef}},
	TableFieldPtr:               {{colIndex, TableField}},
	TableField:                  {{colUint16, 0}, {colString, 0}, {colBlob, 0}},
	TableMethodPtr:              {{colIndex, TableMethodDef}},
	TableMethodDef:              {{colUint32, 0}, {colUint16, 0}, {colUint16, 0}, {colString, 0}, {colBlob, 0}, {colIndex, TableParam}},
	TableParamPtr:               {{colIndex, TableParam}},
	TableParam:                  {{colUint16, 0}, {colUint16, 0}, {colString, 0}},
	TableInterfaceImpl:          {{colIndex, TableTypeDef}, {colCoded, codedTypeDefOrRef}},
	TableMemberRef:              {{colCoded, codedMemberRefParent}, {colString, 0}, {colBlob, 0}},
	TableConstant:               {{colUint16, 0}, {colCoded, codedHasConstant}, {colBlob, 0}},
	TableCustomAttribute:        {{colCoded, codedHasCustomAttribute}, {colCoded, codedCustomAttributeType}, {colBlob, 0}},
	TableFieldMarshal:           {{colCoded, codedHasFieldMarshal}, {colBlob, 0}},
	TableDeclSecurity:           {{colUint16, 0}, {colCoded, codedHasDeclSecurity}, {colBlob, 0}},
	TableClassLayout:            {{colUint16, 0}, {colUint32, 0}, {colIndex, TableTypeDef}},
	TableFieldLayout:            {{colUint32, 0}, {colIndex, TableField}},
	TableStandAloneSig:          {{colBlob, 0}},
	TableEventMap:               {{colIndex, TableTypeDef}, {colIndex, TableEvent}},
	TableEventPtr:               {{colIndex, TableEvent}},
	TableEvent:                  {{colUint16, 0}, {colString, 0}, {colCoded, codedTypeDefOrRef}},
	TablePropertyMap:            {{colIndex, TableTypeDef}, {colIndex, TableProperty}},
	TablePropertyPtr:            {{colIndex, TableProperty}},
	TableProperty:               {{colUint16, 0}, {colString, 0}, {colBlob, 0}},
	TableMethodSemantics:        {{colUint16, 0}, {colIndex, TableMethodDef}, {colCoded, codedHasSemantics}},
	TableMethodImpl:             {{colIndex, TableTypeDef}, {colCoded, codedMethodDefOrRef}, {colCoded, codedMethodDefOrRef}},
	TableModuleRef:              {{colString, 0}},
	TableTypeSpec:               {{colBlob, 0}},
	TableImplMap:                {{colUint16, 0}, {colCoded, codedMemberForwarded}, {colString, 0}, {colIndex, TableModuleRef}},
	TableFieldRVA:               {{colUint32, 0}, {colIndex, TableField}},
	TableAssembly:               {{colUint32, 0}, {colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint32, 0}, {colBlob, 0}, {colString, 0}, {colString, 0}},
	TableAssemblyProcessor:      {{colUint32, 0}},
	TableAssemblyOS:             {{colUint32, 0}, {colUint32, 0}, {colUint32, 0}},
	TableAssemblyRef:            {{colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint16, 0}, {colUint32, 0}, {colBlob, 0}, {colString, 0}, {colString, 0}, {colBlob, 0}},
	TableAssemblyRefProcessor:   {{colUint32, 0}, {colIndex, TableAssemblyRef}},
	TableAssemblyRefOS:          {{colUint32, 0}, {colUint32, 0}, {colUint32, 0}, {colIndex, TableAssemblyRef}},
	TableFile:                   {{colUint32, 0}, {colString, 0}, {colBlob, 0}},
	TableExportedType:           {{colUint32, 0}, {colUint32, 0}, {colString, 0}, {colString, 0}, {colCoded, codedImplementation}},
	TableManifestResource:       {{colUint32, 0}, {colUint32, 0}, {colString, 0}, {colCoded, codedImplementation}},
	TableNestedClass:            {{colIndex, TableTypeDef}, {colIndex, TableTypeDef}},
	TableGenericParam:           {{colUint16, 0}, {colUint16, 0}, {colCoded, codedTypeOrMethodDef}, {colString, 0}},
	TableMethodSpec:             {{colCoded, codedMethodDefOrRef}, {colBlob, 0}},
	TableGenericParamConstraint: {{colIndex, TableGenericParam}, {colCoded, codedTypeDefOrRef}},
}

// CodedIndex is a resolved coded index (§II.24.2.6): a (table, 1-based row) pair.
// A zero Row means null; Table is TableNull in that case.
type CodedIndex struct {
	Table Table
	Row   uint32
}

// IsNull reports whether the coded index refers to nothing.
func (c CodedIndex) IsNull() bool { return c.Row == 0 }

// Typed rows for the tables the projection consumes. String columns are
// resolved eagerly; blob columns keep the heap offset (decoded on demand).

// TypeRefRow is a TypeRef table row (§II.22.38).
type TypeRefRow struct {
	ResolutionScope CodedIndex
	Name            string
	Namespace       string
}

// TypeDefRow is a TypeDef table row (§II.22.37).
type TypeDefRow struct {
	Flags     TypeAttributes
	Name      string
	Namespace string
	Extends   CodedIndex
	// FieldFirst/FieldEnd and MethodFirst/MethodEnd are 1-based half-open
	// row ranges into Fields and Methods.
	FieldFirst, FieldEnd   uint32
	MethodFirst, MethodEnd uint32
}

// FieldRow is a Field table row (§II.22.15).
type FieldRow struct {
	Flags     FieldAttributes
	Name      string
	Signature uint32 // #Blob offset
}

// MethodDefRow is a MethodDef table row (§II.22.26).
type MethodDefRow struct {
	RVA       uint32
	ImplFlags MethodImplAttributes
	Flags     MethodAttributes
	Name      string
	Signature uint32 // #Blob offset
	// ParamFirst/ParamEnd is a 1-based half-open row range into Params.
	ParamFirst, ParamEnd uint32
}

// ParamRow is a Param table row (§II.22.33).
type ParamRow struct {
	Flags    ParamAttributes
	Sequence uint16
	Name     string
}

// InterfaceImplRow is an InterfaceImpl table row (§II.22.23).
type InterfaceImplRow struct {
	Class     uint32 // TypeDef row
	Interface CodedIndex
}

// MemberRefRow is a MemberRef table row (§II.22.25).
type MemberRefRow struct {
	Class     CodedIndex
	Name      string
	Signature uint32 // #Blob offset
}

// ConstantRow is a Constant table row (§II.22.9).
type ConstantRow struct {
	Type   ElementType // ELEMENT_TYPE_* of the value (padded to 2 bytes on disk)
	Parent CodedIndex
	Value  uint32 // #Blob offset
}

// CustomAttributeRow is a CustomAttribute table row (§II.22.10).
type CustomAttributeRow struct {
	Parent CodedIndex
	Type   CodedIndex // MethodDef or MemberRef of the .ctor
	Value  uint32     // #Blob offset
}

// ClassLayoutRow is a ClassLayout table row (§II.22.8).
type ClassLayoutRow struct {
	PackingSize uint16
	ClassSize   uint32
	Parent      uint32 // TypeDef row
}

// FieldLayoutRow is a FieldLayout table row (§II.22.16).
type FieldLayoutRow struct {
	Offset uint32
	Field  uint32 // Field row
}

// ImplMapRow is an ImplMap table row (§II.22.22).
type ImplMapRow struct {
	MappingFlags    PInvokeAttributes
	MemberForwarded CodedIndex
	ImportName      string
	ImportScope     uint32 // ModuleRef row
}

// NestedClassRow is a NestedClass table row (§II.22.32).
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
	// #~ header (§II.24.2.6): Reserved(4) MajorVersion(1) MinorVersion(1)
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
			if member != TableNull && decoder.rowCounts[member] > maxRows {
				maxRows = decoder.rowCounts[member]
			}
		}
		decoder.codedWide[group] = maxRows >= 1<<(16-tagBits)
	}

	// Decode tables in ID order; skip the ones the projection never reads.
	for tableID := Table(0); tableID < tableCount; tableID++ {
		count := int(decoder.rowCounts[tableID])
		if count == 0 {
			continue
		}
		switch tableID {
		case TableTypeRef:
			t.TypeRefs = decodeRows(decoder, tableID, count, func(r *rowReader) TypeRefRow {
				return TypeRefRow{
					ResolutionScope: r.coded(codedResolutionScope),
					Name:            r.string(),
					Namespace:       r.string(),
				}
			})
		case TableTypeDef:
			t.TypeDefs = decodeRows(decoder, tableID, count, func(r *rowReader) TypeDefRow {
				return TypeDefRow{
					Flags:       TypeAttributes(r.uint32()),
					Name:        r.string(),
					Namespace:   r.string(),
					Extends:     r.coded(codedTypeDefOrRef),
					FieldFirst:  r.index(TableField),
					MethodFirst: r.index(TableMethodDef),
				}
			})
		case TableField:
			t.Fields = decodeRows(decoder, tableID, count, func(r *rowReader) FieldRow {
				return FieldRow{Flags: FieldAttributes(r.uint16()), Name: r.string(), Signature: r.blob()}
			})
		case TableMethodDef:
			t.Methods = decodeRows(decoder, tableID, count, func(r *rowReader) MethodDefRow {
				return MethodDefRow{
					RVA:        r.uint32(),
					ImplFlags:  MethodImplAttributes(r.uint16()),
					Flags:      MethodAttributes(r.uint16()),
					Name:       r.string(),
					Signature:  r.blob(),
					ParamFirst: r.index(TableParam),
				}
			})
		case TableParam:
			t.Params = decodeRows(decoder, tableID, count, func(r *rowReader) ParamRow {
				return ParamRow{Flags: ParamAttributes(r.uint16()), Sequence: r.uint16(), Name: r.string()}
			})
		case TableInterfaceImpl:
			t.InterfaceImpls = decodeRows(decoder, tableID, count, func(r *rowReader) InterfaceImplRow {
				return InterfaceImplRow{Class: r.index(TableTypeDef), Interface: r.coded(codedTypeDefOrRef)}
			})
		case TableMemberRef:
			t.MemberRefs = decodeRows(decoder, tableID, count, func(r *rowReader) MemberRefRow {
				return MemberRefRow{Class: r.coded(codedMemberRefParent), Name: r.string(), Signature: r.blob()}
			})
		case TableConstant:
			t.Constants = decodeRows(decoder, tableID, count, func(r *rowReader) ConstantRow {
				return ConstantRow{Type: ElementType(r.uint16()), Parent: r.coded(codedHasConstant), Value: r.blob()}
			})
		case TableCustomAttribute:
			t.CustomAttributes = decodeRows(decoder, tableID, count, func(r *rowReader) CustomAttributeRow {
				return CustomAttributeRow{
					Parent: r.coded(codedHasCustomAttribute),
					Type:   r.coded(codedCustomAttributeType),
					Value:  r.blob(),
				}
			})
		case TableClassLayout:
			t.ClassLayouts = decodeRows(decoder, tableID, count, func(r *rowReader) ClassLayoutRow {
				return ClassLayoutRow{PackingSize: r.uint16(), ClassSize: r.uint32(), Parent: r.index(TableTypeDef)}
			})
		case TableFieldLayout:
			t.FieldLayouts = decodeRows(decoder, tableID, count, func(r *rowReader) FieldLayoutRow {
				return FieldLayoutRow{Offset: r.uint32(), Field: r.index(TableField)}
			})
		case TableModuleRef:
			t.ModuleRefs = decodeRows(decoder, tableID, count, func(r *rowReader) string {
				return r.string()
			})
		case TableTypeSpec:
			t.TypeSpecs = decodeRows(decoder, tableID, count, func(r *rowReader) uint32 {
				return r.blob()
			})
		case TableImplMap:
			t.ImplMaps = decodeRows(decoder, tableID, count, func(r *rowReader) ImplMapRow {
				return ImplMapRow{
					MappingFlags:    PInvokeAttributes(r.uint16()),
					MemberForwarded: r.coded(codedMemberForwarded),
					ImportName:      r.string(),
					ImportScope:     r.index(TableModuleRef),
				}
			})
		case TableNestedClass:
			t.NestedClasses = decodeRows(decoder, tableID, count, func(r *rowReader) NestedClassRow {
				return NestedClassRow{NestedClass: r.index(TableTypeDef), EnclosingClass: r.index(TableTypeDef)}
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
func (d *tableDecoder) rowSize(tableID Table) int {
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

func (d *tableDecoder) skipTable(tableID Table, count int) {
	total := d.rowSize(tableID) * count
	if d.pos+total > len(d.data) {
		d.err = fmt.Errorf("table data truncated (need %d bytes at %d)", total, d.pos)
		return
	}
	d.pos += total
}

// decodeRows decodes count rows of tableID via the per-row builder.
func decodeRows[T any](d *tableDecoder, tableID Table, count int, build func(*rowReader) T) []T {
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

func (r *rowReader) index(tableID Table) uint32 {
	return r.narrowOrWide(r.decoder.rowCounts[tableID] > 0xFFFF)
}

func (r *rowReader) coded(group uint8) CodedIndex {
	raw := r.narrowOrWide(r.decoder.codedWide[group])
	members := codedGroups[group]
	tagBits := bits.Len(uint(len(members) - 1))
	tag := raw & (1<<tagBits - 1)
	row := raw >> tagBits
	if int(tag) >= len(members) || members[tag] == TableNull || row == 0 {
		return CodedIndex{Table: TableNull, Row: 0}
	}
	return CodedIndex{Table: members[tag], Row: row}
}

// fixListRanges converts §II.22 "list" columns (start index of a run that
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
