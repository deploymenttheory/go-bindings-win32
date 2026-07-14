// Package win32 is the runtime layer for the generated Win32 bindings.
//
// It is the only package the generated code (and its consumers) need beyond
// the standard library: DLL/proc dispatch, GetLastError surfacing, UTF-16
// string conversion, and the GUID value type. The runtime itself is
// stdlib-only — the module has no external dependencies.
package win32

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Errno is the Win32 error code captured from GetLastError, as an error.
// It is the standard library's syscall.Errno (which is also the type of
// golang.org/x/sys/windows' ERROR_* constants, so errors.Is comparisons
// against those work without this module requiring x/sys).
type Errno = syscall.Errno

// Win32 error sentinels the runtime returns itself (WIN32_ERROR values; the
// generated foundation package carries the full set, but the runtime cannot
// import the generated tree without a cycle).
const (
	errnoInvalidFunction  = syscall.Errno(1)  // ERROR_INVALID_FUNCTION
	errnoInvalidParameter = syscall.Errno(87) // ERROR_INVALID_PARAMETER
)

// LastError normalizes a captured GetLastError value for a call that FAILED:
// a zero errno still yields a non-nil error, so failure is never silently
// reported as success.
func LastError(errno Errno) error {
	if errno != 0 {
		return errno
	}
	return errnoInvalidParameter
}

// Bool32 converts a Go bool to the Win32 BOOL word (1/0).
func Bool32(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// BoolErr converts a Win32 BOOL result (no GetLastError) into an error: nil
// when non-zero (success), a generic failure error when zero.
func BoolErr(b int32) error {
	if b != 0 {
		return nil
	}
	return errnoInvalidFunction
}

// Succeeded reports whether an HRESULT indicates success (top bit clear).
// Generated COM methods return raw HRESULT values (as the typed
// foundation.HRESULT); pass them here as int32.
func Succeeded(hresult int32) bool {
	return hresult >= 0
}

// GUID is the Win32 GUID structure.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// String renders the GUID in canonical "xxxxxxxx-xxxx-xxxx-xxxx-…" form.
func (g GUID) String() string {
	return fmt.Sprintf("%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		g.Data1, g.Data2, g.Data3,
		g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3],
		g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

// UTF16Ptr converts a Go string to a NUL-terminated UTF-16 pointer for PWSTR
// parameters. It panics if s contains a NUL byte.
func UTF16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic("win32: string contains NUL byte")
	}
	return p
}

// UTF16ToString converts a NUL-terminated UTF-16 pointer back to a Go string.
// A nil pointer yields "".
func UTF16ToString(p *uint16) string {
	if p == nil {
		return ""
	}
	// Find the NUL terminator to size the slice.
	length := 0
	for cursor := unsafe.Pointer(p); *(*uint16)(cursor) != 0; cursor = unsafe.Add(cursor, unsafe.Sizeof(uint16(0))) {
		length++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, length))
}
