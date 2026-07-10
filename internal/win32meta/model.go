// Package win32meta defines the intermediate representation (IR) for the
// Win32 API surface, projected from Windows.Win32.winmd. One NamespaceMeta is
// serialized per namespace as metadata/win32/<namespace>.w32meta.json.
//
// The IR is the seam between the winmd reader (producer) and the codegen
// pipeline (consumer): everything downstream of this package is independent
// of ECMA-335.
package win32meta

// CurrentSchemaVersion is bumped when the IR changes incompatibly; readers
// reject files with a different version so stale caches are re-ingested.
const CurrentSchemaVersion = 1

// NamespaceMeta is the serialized unit: the full API surface of one
// Windows.Win32 namespace.
type NamespaceMeta struct {
	// Namespace is the short namespace path without the "Windows.Win32."
	// prefix, e.g. "System.Threading".
	Namespace     string `json:"namespace"`
	SchemaVersion int    `json:"schema_version"`

	// Provenance of the winmd this namespace was projected from.
	WinmdVersion string `json:"winmd_version,omitempty"`

	Structs    map[string]Struct       `json:"structs,omitempty"`
	Enums      map[string]Enum         `json:"enums,omitempty"`
	Functions  []Function              `json:"functions,omitempty"`
	Constants  []Constant              `json:"constants,omitempty"`
	Interfaces map[string]ComInterface `json:"interfaces,omitempty"`
	Delegates  map[string]FuncPointer  `json:"delegates,omitempty"`
	Typedefs   map[string]Typedef      `json:"typedefs,omitempty"`
}

// TypeRef is the recursive type reference grammar, mirroring the structured
// ECMA-335 signature (and the win32json Kind/Child schema).
type TypeRef struct {
	// Kind is one of "Native", "ApiRef", "PointerTo", "Array".
	Kind string `json:"kind"`

	// Name: for Native, the primitive ("Void", "Byte", "SByte", "Char",
	// "Int16", "UInt16", "Int32", "UInt32", "Int64", "UInt64", "Single",
	// "Double", "IntPtr", "UIntPtr", "Guid", "String"); for ApiRef, the
	// referenced type's name.
	Name string `json:"name,omitempty"`

	// Api is the owning namespace of an ApiRef (short form, e.g.
	// "Foundation"), used for cross-package import resolution.
	Api string `json:"api,omitempty"`

	// TargetKind classifies an ApiRef target: "Struct", "Union", "Enum",
	// "Com", "FunctionPointer", "Typedef".
	TargetKind string `json:"target_kind,omitempty"`

	Child    *TypeRef `json:"child,omitempty"`     // PointerTo / Array element
	ArrayLen uint32   `json:"array_len,omitempty"` // Array fixed length

	// IsConst is set when the signature carried an IsConst modifier.
	IsConst bool `json:"is_const,omitempty"`
}

// Availability carries platform/architecture constraints from the metadata.
type Availability struct {
	// Platform is the minimum supported OS, e.g. "windows5.1.2600".
	Platform string `json:"platform,omitempty"`
	// Architectures is a subset of {"x86", "amd64", "arm64"}; empty means
	// all architectures.
	Architectures []string `json:"architectures,omitempty"`
	// DocURL is the learn.microsoft.com link from [Documentation].
	DocURL string `json:"doc_url,omitempty"`
}

// Param is one function/method parameter.
type Param struct {
	Name string  `json:"name"`
	Type TypeRef `json:"type"`

	IsIn       bool `json:"is_in,omitempty"`
	IsOut      bool `json:"is_out,omitempty"`
	IsOptional bool `json:"is_optional,omitempty"`
	IsReserved bool `json:"is_reserved,omitempty"`
	IsConst    bool `json:"is_const,omitempty"`
	IsComOutPtr bool `json:"is_com_out_ptr,omitempty"`
	IsRetVal    bool `json:"is_ret_val,omitempty"`

	// NativeArrayCountParamIndex is the 0-based index of the parameter
	// carrying this array parameter's element count (-1 when absent).
	NativeArrayCountParamIndex int `json:"native_array_count_param_index,omitempty"`
	// NativeArrayCountConst is a fixed array length (0 when absent).
	NativeArrayCountConst uint32 `json:"native_array_count_const,omitempty"`
	// MemorySizeBytesParamIndex is the 0-based index of the parameter
	// carrying this pointer parameter's byte size (-1 when absent).
	MemorySizeBytesParamIndex int `json:"memory_size_bytes_param_index,omitempty"`
	// FreeWith names the function that releases this out param's resource.
	FreeWith string `json:"free_with,omitempty"`
}

// Function is a flat Win32 function exported from a DLL.
type Function struct {
	Name string  `json:"name"`
	DLL  string  `json:"dll"`
	// EntryPoint is the DLL export name when it differs from Name.
	EntryPoint     string  `json:"entry_point,omitempty"`
	SetLastError   bool    `json:"set_last_error,omitempty"`
	Return         TypeRef `json:"return"`
	Params         []Param `json:"params,omitempty"`
	Availability   Availability `json:"availability,omitempty"`
	// UnsuffixedName is set on -W functions to the header macro name
	// (e.g. CreateEventW → CreateEvent) when the metadata declares a
	// Unicode alias.
	UnsuffixedName string `json:"unsuffixed_name,omitempty"`
}

// Struct is a value struct (or union) with C layout.
type Struct struct {
	Fields       []StructField `json:"fields,omitempty"`
	IsUnion      bool          `json:"is_union,omitempty"`
	PackingSize  uint16        `json:"packing_size,omitempty"`
	Size         uint32        `json:"size,omitempty"`
	// NestedTypes holds anonymous member structs/unions declared inside
	// this struct (names like "_Anonymous_e__Union").
	NestedTypes  map[string]Struct `json:"nested_types,omitempty"`
	Availability Availability      `json:"availability,omitempty"`
	// StructSizeField names the field auto-populated with sizeof(struct).
	StructSizeField string `json:"struct_size_field,omitempty"`
	// ArchVariants holds additional architecture-specific layouts of the
	// same struct (the first-seen layout is the map entry itself; check
	// Availability.Architectures on each).
	ArchVariants []Struct `json:"arch_variants,omitempty"`
}

// StructField is one field of a Struct.
type StructField struct {
	Name string  `json:"name"`
	Type TypeRef `json:"type"`
	// Bitfields: backing fields named _bitfieldN carry the members.
	Bitfields []Bitfield `json:"bitfields,omitempty"`
	// IsFlexibleArray marks a trailing C flexible array member.
	IsFlexibleArray bool `json:"is_flexible_array,omitempty"`
}

// Bitfield is one [NativeBitfield] member of a backing field.
type Bitfield struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

// Enum is a C enum (or a lifted #define group).
type Enum struct {
	// BaseType is the Go-facing integral base: "int32", "uint32", etc.
	BaseType     string       `json:"base_type"`
	IsFlags      bool         `json:"is_flags,omitempty"`
	IsScoped     bool         `json:"is_scoped,omitempty"`
	Members      []EnumMember `json:"members,omitempty"`
	Availability Availability `json:"availability,omitempty"`
}

// EnumMember is one enum value; Value is its decimal string representation.
type EnumMember struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Constant is a namespace-level constant from the Apis class.
type Constant struct {
	Name string  `json:"name"`
	Type TypeRef `json:"type"`
	// Value is the literal: decimal for integers, verbatim for strings,
	// canonical GUID form for GUIDs, initializer text for struct constants.
	Value string `json:"value"`
	// ValueKind is "Int", "UInt", "Float", "String", "Guid", "Struct".
	ValueKind    string       `json:"value_kind"`
	Availability Availability `json:"availability,omitempty"`
}

// ComInterface is a COM interface: vtable methods in declaration order.
type ComInterface struct {
	// GUID is the IID in canonical form.
	GUID string `json:"guid,omitempty"`
	// BaseInterface names the inherited interface (empty for IUnknown).
	BaseInterface string `json:"base_interface,omitempty"`
	// BaseInterfaceApi is the namespace owning BaseInterface.
	BaseInterfaceApi string      `json:"base_interface_api,omitempty"`
	Methods          []ComMethod `json:"methods,omitempty"`
	Availability     Availability `json:"availability,omitempty"`
}

// ComMethod is one vtable slot.
type ComMethod struct {
	Name   string  `json:"name"`
	Return TypeRef `json:"return"`
	Params []Param `json:"params,omitempty"`
}

// FuncPointer is a callback function-pointer type (a CLI delegate).
type FuncPointer struct {
	Return       TypeRef      `json:"return"`
	Params       []Param      `json:"params,omitempty"`
	Availability Availability `json:"availability,omitempty"`
}

// Typedef is a NativeTypedef/MetadataTypedef wrapper (handles, PWSTR, …).
type Typedef struct {
	// Underlying is the wrapped type (e.g. IntPtr for HANDLE).
	Underlying TypeRef `json:"underlying"`
	// IsMetadataTypedef distinguishes metadata-only conveniences from
	// typedefs that exist in the C headers.
	IsMetadataTypedef bool `json:"is_metadata_typedef,omitempty"`
	// FreeFunc is the [RAIIFree] close function (e.g. CloseHandle).
	FreeFunc string `json:"free_func,omitempty"`
	// InvalidValues are [InvalidHandleValue] sentinels (decimal strings).
	InvalidValues []string `json:"invalid_values,omitempty"`
	// AlsoUsableFor names a type this one implicitly converts to.
	AlsoUsableFor string       `json:"also_usable_for,omitempty"`
	Availability  Availability `json:"availability,omitempty"`
}
