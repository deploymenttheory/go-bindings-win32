package winmd

import (
	"fmt"
	"strings"
)

// Typed metadata bitmask columns (ECMA-335 §II.23.1). Constant names follow
// the specification's member names, prefixed by their owning type.

// TypeAttributes is the TypeDef Flags column (§II.23.1.15).
type TypeAttributes uint32

const (
	// Visibility (mask 0x00000007).
	TypeAttrVisibilityMask    TypeAttributes = 0x00000007
	TypeAttrNotPublic         TypeAttributes = 0x00000000
	TypeAttrPublic            TypeAttributes = 0x00000001
	TypeAttrNestedPublic      TypeAttributes = 0x00000002
	TypeAttrNestedPrivate     TypeAttributes = 0x00000003
	TypeAttrNestedFamily      TypeAttributes = 0x00000004
	TypeAttrNestedAssembly    TypeAttributes = 0x00000005
	TypeAttrNestedFamANDAssem TypeAttributes = 0x00000006
	TypeAttrNestedFamORAssem  TypeAttributes = 0x00000007

	// Class layout (mask 0x00000018).
	TypeAttrLayoutMask       TypeAttributes = 0x00000018
	TypeAttrAutoLayout       TypeAttributes = 0x00000000
	TypeAttrSequentialLayout TypeAttributes = 0x00000008
	TypeAttrExplicitLayout   TypeAttributes = 0x00000010

	// Class semantics (mask 0x00000020).
	TypeAttrClassSemanticsMask TypeAttributes = 0x00000020
	TypeAttrClass              TypeAttributes = 0x00000000
	TypeAttrInterface          TypeAttributes = 0x00000020

	TypeAttrAbstract       TypeAttributes = 0x00000080
	TypeAttrSealed         TypeAttributes = 0x00000100
	TypeAttrSpecialName    TypeAttributes = 0x00000400
	TypeAttrRTSpecialName  TypeAttributes = 0x00000800
	TypeAttrImport         TypeAttributes = 0x00001000
	TypeAttrSerializable   TypeAttributes = 0x00002000
	TypeAttrWindowsRuntime TypeAttributes = 0x00004000

	// String formatting for native interop (mask 0x00030000).
	TypeAttrStringFormatMask  TypeAttributes = 0x00030000
	TypeAttrAnsiClass         TypeAttributes = 0x00000000
	TypeAttrUnicodeClass      TypeAttributes = 0x00010000
	TypeAttrAutoClass         TypeAttributes = 0x00020000
	TypeAttrCustomFormatClass TypeAttributes = 0x00030000

	TypeAttrHasSecurity     TypeAttributes = 0x00040000
	TypeAttrBeforeFieldInit TypeAttributes = 0x00100000
	TypeAttrIsTypeForwarder TypeAttributes = 0x00200000
)

// String renders the set attributes in specification vocabulary.
func (a TypeAttributes) String() string {
	var parts []string
	switch a & TypeAttrVisibilityMask {
	case TypeAttrNotPublic:
		parts = append(parts, "NotPublic")
	case TypeAttrPublic:
		parts = append(parts, "Public")
	case TypeAttrNestedPublic:
		parts = append(parts, "NestedPublic")
	case TypeAttrNestedPrivate:
		parts = append(parts, "NestedPrivate")
	case TypeAttrNestedFamily:
		parts = append(parts, "NestedFamily")
	case TypeAttrNestedAssembly:
		parts = append(parts, "NestedAssembly")
	case TypeAttrNestedFamANDAssem:
		parts = append(parts, "NestedFamANDAssem")
	case TypeAttrNestedFamORAssem:
		parts = append(parts, "NestedFamORAssem")
	}
	switch a & TypeAttrLayoutMask {
	case TypeAttrSequentialLayout:
		parts = append(parts, "SequentialLayout")
	case TypeAttrExplicitLayout:
		parts = append(parts, "ExplicitLayout")
	}
	if a&TypeAttrInterface != 0 {
		parts = append(parts, "Interface")
	}
	switch a & TypeAttrStringFormatMask {
	case TypeAttrUnicodeClass:
		parts = append(parts, "UnicodeClass")
	case TypeAttrAutoClass:
		parts = append(parts, "AutoClass")
	case TypeAttrCustomFormatClass:
		parts = append(parts, "CustomFormatClass")
	}
	parts = appendFlagNames(parts, uint32(a), []flagName{
		{uint32(TypeAttrAbstract), "Abstract"},
		{uint32(TypeAttrSealed), "Sealed"},
		{uint32(TypeAttrSpecialName), "SpecialName"},
		{uint32(TypeAttrRTSpecialName), "RTSpecialName"},
		{uint32(TypeAttrImport), "Import"},
		{uint32(TypeAttrSerializable), "Serializable"},
		{uint32(TypeAttrWindowsRuntime), "WindowsRuntime"},
		{uint32(TypeAttrHasSecurity), "HasSecurity"},
		{uint32(TypeAttrBeforeFieldInit), "BeforeFieldInit"},
		{uint32(TypeAttrIsTypeForwarder), "IsTypeForwarder"},
	})
	return joinFlags(parts, uint32(a))
}

// FieldAttributes is the Field Flags column (§II.23.1.5).
type FieldAttributes uint16

const (
	// Accessibility (mask 0x0007).
	FieldAttrFieldAccessMask    FieldAttributes = 0x0007
	FieldAttrCompilerControlled FieldAttributes = 0x0000
	FieldAttrPrivate            FieldAttributes = 0x0001
	FieldAttrFamANDAssem        FieldAttributes = 0x0002
	FieldAttrAssembly           FieldAttributes = 0x0003
	FieldAttrFamily             FieldAttributes = 0x0004
	FieldAttrFamORAssem         FieldAttributes = 0x0005
	FieldAttrPublic             FieldAttributes = 0x0006

	FieldAttrStatic          FieldAttributes = 0x0010
	FieldAttrInitOnly        FieldAttributes = 0x0020
	FieldAttrLiteral         FieldAttributes = 0x0040
	FieldAttrNotSerialized   FieldAttributes = 0x0080
	FieldAttrHasFieldRVA     FieldAttributes = 0x0100
	FieldAttrSpecialName     FieldAttributes = 0x0200
	FieldAttrRTSpecialName   FieldAttributes = 0x0400
	FieldAttrHasFieldMarshal FieldAttributes = 0x1000
	FieldAttrPInvokeImpl     FieldAttributes = 0x2000
	FieldAttrHasDefault      FieldAttributes = 0x8000
)

// String renders the set attributes in specification vocabulary.
func (a FieldAttributes) String() string {
	var parts []string
	switch a & FieldAttrFieldAccessMask {
	case FieldAttrCompilerControlled:
		parts = append(parts, "CompilerControlled")
	case FieldAttrPrivate:
		parts = append(parts, "Private")
	case FieldAttrFamANDAssem:
		parts = append(parts, "FamANDAssem")
	case FieldAttrAssembly:
		parts = append(parts, "Assembly")
	case FieldAttrFamily:
		parts = append(parts, "Family")
	case FieldAttrFamORAssem:
		parts = append(parts, "FamORAssem")
	case FieldAttrPublic:
		parts = append(parts, "Public")
	}
	parts = appendFlagNames(parts, uint32(a), []flagName{
		{uint32(FieldAttrStatic), "Static"},
		{uint32(FieldAttrInitOnly), "InitOnly"},
		{uint32(FieldAttrLiteral), "Literal"},
		{uint32(FieldAttrNotSerialized), "NotSerialized"},
		{uint32(FieldAttrHasFieldRVA), "HasFieldRVA"},
		{uint32(FieldAttrSpecialName), "SpecialName"},
		{uint32(FieldAttrRTSpecialName), "RTSpecialName"},
		{uint32(FieldAttrHasFieldMarshal), "HasFieldMarshal"},
		{uint32(FieldAttrPInvokeImpl), "PInvokeImpl"},
		{uint32(FieldAttrHasDefault), "HasDefault"},
	})
	return joinFlags(parts, uint32(a))
}

// MethodAttributes is the MethodDef Flags column (§II.23.1.10).
type MethodAttributes uint16

const (
	// Accessibility (mask 0x0007).
	MethodAttrMemberAccessMask   MethodAttributes = 0x0007
	MethodAttrCompilerControlled MethodAttributes = 0x0000
	MethodAttrPrivate            MethodAttributes = 0x0001
	MethodAttrFamANDAssem        MethodAttributes = 0x0002
	MethodAttrAssem              MethodAttributes = 0x0003
	MethodAttrFamily             MethodAttributes = 0x0004
	MethodAttrFamORAssem         MethodAttributes = 0x0005
	MethodAttrPublic             MethodAttributes = 0x0006

	MethodAttrUnmanagedExport MethodAttributes = 0x0008
	MethodAttrStatic          MethodAttributes = 0x0010
	MethodAttrFinal           MethodAttributes = 0x0020
	MethodAttrVirtual         MethodAttributes = 0x0040
	MethodAttrHideBySig       MethodAttributes = 0x0080

	// Vtable layout (mask 0x0100).
	MethodAttrVtableLayoutMask MethodAttributes = 0x0100
	MethodAttrReuseSlot        MethodAttributes = 0x0000
	MethodAttrNewSlot          MethodAttributes = 0x0100

	MethodAttrStrict           MethodAttributes = 0x0200
	MethodAttrAbstract         MethodAttributes = 0x0400
	MethodAttrSpecialName      MethodAttributes = 0x0800
	MethodAttrRTSpecialName    MethodAttributes = 0x1000
	MethodAttrPInvokeImpl      MethodAttributes = 0x2000
	MethodAttrHasSecurity      MethodAttributes = 0x4000
	MethodAttrRequireSecObject MethodAttributes = 0x8000
)

// String renders the set attributes in specification vocabulary.
func (a MethodAttributes) String() string {
	var parts []string
	switch a & MethodAttrMemberAccessMask {
	case MethodAttrCompilerControlled:
		parts = append(parts, "CompilerControlled")
	case MethodAttrPrivate:
		parts = append(parts, "Private")
	case MethodAttrFamANDAssem:
		parts = append(parts, "FamANDAssem")
	case MethodAttrAssem:
		parts = append(parts, "Assem")
	case MethodAttrFamily:
		parts = append(parts, "Family")
	case MethodAttrFamORAssem:
		parts = append(parts, "FamORAssem")
	case MethodAttrPublic:
		parts = append(parts, "Public")
	}
	parts = appendFlagNames(parts, uint32(a), []flagName{
		{uint32(MethodAttrUnmanagedExport), "UnmanagedExport"},
		{uint32(MethodAttrStatic), "Static"},
		{uint32(MethodAttrFinal), "Final"},
		{uint32(MethodAttrVirtual), "Virtual"},
		{uint32(MethodAttrHideBySig), "HideBySig"},
		{uint32(MethodAttrNewSlot), "NewSlot"},
		{uint32(MethodAttrStrict), "Strict"},
		{uint32(MethodAttrAbstract), "Abstract"},
		{uint32(MethodAttrSpecialName), "SpecialName"},
		{uint32(MethodAttrRTSpecialName), "RTSpecialName"},
		{uint32(MethodAttrPInvokeImpl), "PInvokeImpl"},
		{uint32(MethodAttrHasSecurity), "HasSecurity"},
		{uint32(MethodAttrRequireSecObject), "RequireSecObject"},
	})
	return joinFlags(parts, uint32(a))
}

// MethodImplAttributes is the MethodDef ImplFlags column (§II.23.1.11).
type MethodImplAttributes uint16

const (
	// Code type (mask 0x0003).
	MethodImplAttrCodeTypeMask MethodImplAttributes = 0x0003
	MethodImplAttrIL           MethodImplAttributes = 0x0000
	MethodImplAttrNative       MethodImplAttributes = 0x0001
	MethodImplAttrOPTIL        MethodImplAttributes = 0x0002
	MethodImplAttrRuntime      MethodImplAttributes = 0x0003

	// Managed (mask 0x0004).
	MethodImplAttrManagedMask MethodImplAttributes = 0x0004
	MethodImplAttrManaged     MethodImplAttributes = 0x0000
	MethodImplAttrUnmanaged   MethodImplAttributes = 0x0004

	MethodImplAttrNoInlining     MethodImplAttributes = 0x0008
	MethodImplAttrForwardRef     MethodImplAttributes = 0x0010
	MethodImplAttrSynchronized   MethodImplAttributes = 0x0020
	MethodImplAttrNoOptimization MethodImplAttributes = 0x0040
	MethodImplAttrPreserveSig    MethodImplAttributes = 0x0080
	MethodImplAttrInternalCall   MethodImplAttributes = 0x1000
)

// String renders the set attributes in specification vocabulary.
func (a MethodImplAttributes) String() string {
	var parts []string
	switch a & MethodImplAttrCodeTypeMask {
	case MethodImplAttrIL:
		parts = append(parts, "IL")
	case MethodImplAttrNative:
		parts = append(parts, "Native")
	case MethodImplAttrOPTIL:
		parts = append(parts, "OPTIL")
	case MethodImplAttrRuntime:
		parts = append(parts, "Runtime")
	}
	if a&MethodImplAttrUnmanaged != 0 {
		parts = append(parts, "Unmanaged")
	}
	parts = appendFlagNames(parts, uint32(a), []flagName{
		{uint32(MethodImplAttrNoInlining), "NoInlining"},
		{uint32(MethodImplAttrForwardRef), "ForwardRef"},
		{uint32(MethodImplAttrSynchronized), "Synchronized"},
		{uint32(MethodImplAttrNoOptimization), "NoOptimization"},
		{uint32(MethodImplAttrPreserveSig), "PreserveSig"},
		{uint32(MethodImplAttrInternalCall), "InternalCall"},
	})
	return joinFlags(parts, uint32(a))
}

// ParamAttributes is the Param Flags column (§II.23.1.13).
type ParamAttributes uint16

const (
	ParamAttrIn              ParamAttributes = 0x0001
	ParamAttrOut             ParamAttributes = 0x0002
	ParamAttrOptional        ParamAttributes = 0x0010
	ParamAttrHasDefault      ParamAttributes = 0x1000
	ParamAttrHasFieldMarshal ParamAttributes = 0x2000
)

// String renders the set attributes in specification vocabulary.
func (a ParamAttributes) String() string {
	parts := appendFlagNames(nil, uint32(a), []flagName{
		{uint32(ParamAttrIn), "In"},
		{uint32(ParamAttrOut), "Out"},
		{uint32(ParamAttrOptional), "Optional"},
		{uint32(ParamAttrHasDefault), "HasDefault"},
		{uint32(ParamAttrHasFieldMarshal), "HasFieldMarshal"},
	})
	return joinFlags(parts, uint32(a))
}

// PInvokeAttributes is the ImplMap MappingFlags column (§II.23.1.8).
type PInvokeAttributes uint16

const (
	PInvokeAttrNoMangle PInvokeAttributes = 0x0001

	// Character set (mask 0x0006).
	PInvokeAttrCharSetMask    PInvokeAttributes = 0x0006
	PInvokeAttrCharSetNotSpec PInvokeAttributes = 0x0000
	PInvokeAttrCharSetAnsi    PInvokeAttributes = 0x0002
	PInvokeAttrCharSetUnicode PInvokeAttributes = 0x0004
	PInvokeAttrCharSetAuto    PInvokeAttributes = 0x0006

	PInvokeAttrSupportsLastError PInvokeAttributes = 0x0040

	// Calling convention (mask 0x0700).
	PInvokeAttrCallConvMask        PInvokeAttributes = 0x0700
	PInvokeAttrCallConvPlatformapi PInvokeAttributes = 0x0100
	PInvokeAttrCallConvCdecl       PInvokeAttributes = 0x0200
	PInvokeAttrCallConvStdcall     PInvokeAttributes = 0x0300
	PInvokeAttrCallConvThiscall    PInvokeAttributes = 0x0400
	PInvokeAttrCallConvFastcall    PInvokeAttributes = 0x0500
)

// String renders the set attributes in specification vocabulary.
func (a PInvokeAttributes) String() string {
	var parts []string
	if a&PInvokeAttrNoMangle != 0 {
		parts = append(parts, "NoMangle")
	}
	switch a & PInvokeAttrCharSetMask {
	case PInvokeAttrCharSetAnsi:
		parts = append(parts, "CharSetAnsi")
	case PInvokeAttrCharSetUnicode:
		parts = append(parts, "CharSetUnicode")
	case PInvokeAttrCharSetAuto:
		parts = append(parts, "CharSetAuto")
	}
	if a&PInvokeAttrSupportsLastError != 0 {
		parts = append(parts, "SupportsLastError")
	}
	switch a & PInvokeAttrCallConvMask {
	case PInvokeAttrCallConvPlatformapi:
		parts = append(parts, "CallConvPlatformapi")
	case PInvokeAttrCallConvCdecl:
		parts = append(parts, "CallConvCdecl")
	case PInvokeAttrCallConvStdcall:
		parts = append(parts, "CallConvStdcall")
	case PInvokeAttrCallConvThiscall:
		parts = append(parts, "CallConvThiscall")
	case PInvokeAttrCallConvFastcall:
		parts = append(parts, "CallConvFastcall")
	}
	return joinFlags(parts, uint32(a))
}

// flagName pairs a single-bit flag with its specification name.
type flagName struct {
	bit  uint32
	name string
}

func appendFlagNames(parts []string, value uint32, flags []flagName) []string {
	for _, f := range flags {
		if value&f.bit != 0 {
			parts = append(parts, f.name)
		}
	}
	return parts
}

func joinFlags(parts []string, value uint32) string {
	if len(parts) == 0 {
		return fmt.Sprintf("0x%X", value)
	}
	return strings.Join(parts, "|")
}
