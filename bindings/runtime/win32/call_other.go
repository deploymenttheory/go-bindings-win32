//go:build windows && !amd64 && !arm64

package win32

// The generated bindings are amd64/arm64 only; on other Windows
// architectures the runtime still compiles, without a trampoline
// (callTrampolineABI0 stays zero and Call panics).
