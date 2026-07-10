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
