// Package view is the pure-data IR for the idiomatic tier's render
// templates. It imports nothing from meta/typemap — the render firewall.
package view

// FunctionModel is one idiomatic function wrapper over a raw-tier call.
type FunctionModel struct {
	CommentLines []string
	GoName       string
	// ParamStr is the idiomatic parameter list ("name string, flags uint32").
	ParamStr string
	// ReturnSig is the idiomatic return signature ("(bool, error)", "error",
	// "uint32", or "").
	ReturnSig string

	// Preamble holds statements that convert idiomatic params into raw args
	// (UTF-16 conversion, bool→BOOL) before the call.
	Preamble []string
	// RawCall is the fully qualified raw-tier call expression with its
	// argument list, e.g. "threading.CreateEventW(nil, _bManualReset, …)".
	RawCall string

	// Shape selects the body/return handling.
	Shape int
	// RetExpr converts the raw result to the idiomatic return (Shape uses it).
	RetExpr string
	// ReturnValues are elevated [out,retval] locals returned before the
	// natural return (FuncRetVal* shapes only).
	ReturnValues []string
}

// InterfaceModel is an idiomatic COM wrapper: a struct holding the raw
// interface pointer, embedding its idiomatic base, with error-returning
// methods.
type InterfaceModel struct {
	CommentLines []string
	TypeName     string
	// RawType is the qualified raw interface type ("systemcomraw.IStream").
	RawType string
	// BaseType is the embedded idiomatic base wrapper type ("com.IUnknown"
	// or a same-package name); "" for a root.
	BaseType string
	// BaseFieldName is the embedded field's Go name (the bare type name,
	// unqualified), used as the struct-literal key.
	BaseFieldName string
	// BaseWrapCall constructs the embedded base from the raw base pointer,
	// e.g. "WrapIUnknown(&raw.IUnknown)"; "" for a root.
	BaseWrapCall string
	// BaseRawField is the raw base embedded-field selector ("IUnknown").
	BaseRawField string
	Methods      []InterfaceMethodModel
}

// InterfaceMethodModel is one idiomatic COM method forwarding to the raw one.
type InterfaceMethodModel struct {
	CommentLines []string
	GoName       string
	// RawGoName is the exact raw method name to call.
	RawGoName string
	ParamStr  string
	ReturnSig string
	Preamble  []string
	// RawArgs are the arguments passed to the raw method call.
	RawArgs []string
	// Shape: FuncErrorOnly (HRESULT→error), FuncRetValError (elevated
	// [out,retval] values + error), or FuncPassthrough (other).
	Shape int
	// ReturnValues are the elevated out-param locals returned before the
	// error (FuncRetValError only).
	ReturnValues []string
	RetExpr      string
}

// Function body shapes.
const (
	// FuncPassthrough: `return <RawCall>` (or bare call for void).
	FuncPassthrough = 0
	// FuncErrorOnly: raw returns error; `return <RawCall>`.
	FuncErrorOnly = 1
	// FuncValueError: raw returns (T, error); wrap value via RetExpr.
	FuncValueError = 2
	// FuncBoolResult: raw returns BOOL value (no error); `return <RawCall> != 0`.
	FuncBoolResult = 3
	// FuncRetValError: HRESULT call with elevated [out,retval] returns:
	// preamble declares the out locals, the call fills them, then
	// `return <values...>, win32.HRESULTError(...)`.
	FuncRetValError = 4
	// FuncRetValRawError: raw call returns error directly (BOOL+SetLastError
	// or HRESULT already lowered); `return <values...>, <RawCall>`.
	FuncRetValRawError = 5
	// FuncRetValHRESULT: raw call returns HRESULT; wrap to error.
	// `return <values...>, win32.HRESULTError(int32(<RawCall>))`.
	FuncRetValHRESULT = 6
	// FuncRetValVoid: raw call is void; call it, then `return <values...>`.
	FuncRetValVoid = 7
)
