// Package view is the pure-data IR consumed by the raw-tier render
// templates. It imports no metadata or typemap packages — every field is a
// fully resolved fragment, so templates only branch and interpolate, never
// decide. (The render firewall.)
package view

// EnumModel is one named enum type with its members.
type EnumModel struct {
	TypeName string
	BaseType string
	IsFlags  bool
	DocURL   string
	Members  []EnumMemberModel
}

// EnumMemberModel is one enum constant.
type EnumMemberModel struct {
	Name  string
	Value string
}

// StructModel is one value struct (or union blob).
type StructModel struct {
	TypeName string
	DocURL   string
	// IsUnionBlob marks a union emitted as an opaque, correctly sized and
	// aligned backing array; Fields then holds the single backing field.
	IsUnionBlob bool
	Fields      []StructFieldModel
}

// StructFieldModel is one struct field, fully resolved.
type StructFieldModel struct {
	Name   string
	GoType string
}

// TypedefModel is a named handle/scalar/pointer typedef.
type TypedefModel struct {
	TypeName string
	// Backing is the Go backing type ("uintptr", "int32", "*uint16").
	Backing string
	DocURL  string
	// InvalidValues renders failure sentinels as a doc line when non-empty.
	InvalidValues []string
	// FreeFunc names the releasing API for the doc comment.
	FreeFunc string
}

// ConstantModel is one package-level constant or variable.
type ConstantModel struct {
	Name   string
	GoType string
	// Literal is the rendered Go literal expression.
	Literal string
	// IsVar emits a var instead of a const (GUIDs, negative-in-unsigned).
	IsVar bool
}

// DelegateModel is a callback function-pointer type (uintptr-backed at the
// raw tier; syscall.NewCallback produces its values).
type DelegateModel struct {
	TypeName string
	DocURL   string
	// Signature documents the callback's Go shape in a comment.
	Signature string
}

// DLLModel declares one lazily loaded DLL and its procs.
type DLLModel struct {
	// VarName is the package-level variable ("modKERNEL32").
	VarName string
	// FileName is the DLL file name ("KERNEL32.dll").
	FileName string
	Procs    []ProcModel
}

// ProcModel is one lazily resolved export.
type ProcModel struct {
	// VarName is the package-level variable ("procCreateEventW").
	VarName string
	// ExportName is the DLL export ("CreateEventW").
	ExportName string
}

// Return-shape discriminants for FunctionModel.ReturnKind. The template
// branches on these and nothing else.
const (
	RetVoid    = 0 // no return value
	RetBoolErr = 1 // BOOL + SetLastError → error only
	RetValErr  = 2 // value + SetLastError, known failure sentinels → (T, error)
	RetVal     = 3 // plain value → T
	RetValLast = 4 // value + SetLastError, unknown sentinel → (T, error); err advisory
)

// InterfaceModel is one COM interface: a pointer-sized struct dispatching
// through its vtable.
type InterfaceModel struct {
	TypeName string
	DocURL   string
	// GUID is the IID string for the doc comment ("" when absent).
	GUID string
	// IIDVar/IIDLiteral declare the IID constant (skipped when GUID is "").
	IIDVar     string
	IIDLiteral string
	// BaseType is the embedded base interface type ("com.IUnknown"); ""
	// makes this a root that declares the raw vtable field itself.
	BaseType string
	// BaseNote documents a severed base embedding ("" when none).
	BaseNote string
	Methods  []ComMethodModel
}

// ComMethodModel is one vtable method, shaped like FunctionModel but
// dispatched through a vtable slot.
type ComMethodModel struct {
	CommentLines []string
	GoName       string
	ParamStr     string
	ReturnSig    string
	// Slot is the absolute vtable index (base chain included).
	Slot     int
	ArgExprs []string
	// ReturnKind reuses the Ret* constants (RetVoid / RetVal only: COM
	// methods carry status in their HRESULT return, not GetLastError).
	ReturnKind int
	RetExpr    string
}

// FunctionModel is one flat DLL function, fully resolved for rendering.
type FunctionModel struct {
	CommentLines []string
	GoName       string
	// ParamStr is the complete parameter list ("a *foo.Bar, b uint32").
	ParamStr string
	// ReturnSig is the complete return signature ("(foundation.HANDLE, error)",
	// "error", or "" for void).
	ReturnSig string
	// ProcVar is the proc variable dispatched through.
	ProcVar string
	// ArgExprs are the rendered SyscallN argument words.
	ArgExprs []string
	// ReturnKind selects the body shape (Ret* constants).
	ReturnKind int
	// RetExpr converts r1 to the Go return value ("foundation.HANDLE(r1)").
	RetExpr string
	// FailureChecks are boolean expressions over `ret` that indicate
	// failure ("ret == 0"); used by RetValErr.
	FailureChecks []string
}
