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
	// Shape: FuncErrorOnly (HRESULT→error) or FuncPassthrough (other).
	Shape   int
	RetExpr string
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
)
