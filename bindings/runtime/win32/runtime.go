// Package win32 is the runtime layer for the generated Win32 bindings.
//
// It is the only package the generated code (and its consumers) need beyond
// the standard library: DLL/proc dispatch, GetLastError surfacing, UTF-16
// string conversion, and the GUID value type. golang.org/x/sys/windows is the
// single external dependency, mirroring the macOS bindings' single-dependency
// rule.
package win32

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// DLL is a lazily loaded system DLL. Generated <pkg>_runtime.go files declare
// one package-level *DLL per imported DLL.
type DLL struct {
	lazy *windows.LazyDLL
}

// NewDLL returns a lazy handle to a system DLL (loaded from System32 only,
// never the application directory, to prevent DLL preloading attacks).
func NewDLL(name string) *DLL {
	return &DLL{lazy: windows.NewLazySystemDLL(name)}
}

// Proc is a lazily resolved exported procedure.
type Proc struct {
	lazy *windows.LazyProc
}

// NewProc returns a lazy reference to an export of the DLL.
func (d *DLL) NewProc(name string) *Proc {
	return &Proc{lazy: d.lazy.NewProc(name)}
}

// Addr resolves and returns the procedure address, panicking with a clear
// message if the DLL or export is unavailable on this system.
func (p *Proc) Addr() uintptr {
	return p.lazy.Addr()
}

// Find resolves the procedure, reporting an error instead of panicking; use
// it to probe for APIs that are not present on older Windows versions.
func (p *Proc) Find() error {
	return p.lazy.Find()
}

// Errno is the Win32 error code captured from GetLastError, as an error.
type Errno = windows.Errno

// LastError normalizes a captured GetLastError value for a call that FAILED:
// a zero errno still yields a non-nil error (windows.EINVAL), so failure is
// never silently reported as success.
func LastError(errno Errno) error {
	if errno != 0 {
		return errno
	}
	return windows.ERROR_INVALID_PARAMETER
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
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic("win32: string contains NUL byte")
	}
	return p
}

// UTF16ToString converts a NUL-terminated UTF-16 pointer back to a Go string.
// A nil pointer yields "".
func UTF16ToString(p *uint16) string {
	return windows.UTF16PtrToString(p)
}
