//go:build windows && (amd64 || arm64)

package win32

// callTrampoline is implemented in call_$GOARCH.s. It is never called from
// Go: syscall.SyscallN enters it as a foreign function via callTrampolineABI0.
func callTrampoline()
