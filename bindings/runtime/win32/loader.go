//go:build windows

package win32

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// System32-only DLL loading, stdlib-only.
//
// Every load goes through LoadLibraryExW with LOAD_LIBRARY_SEARCH_SYSTEM32,
// so a DLL planted next to the executable or in the working directory is
// never picked up (DLL-preloading defense — the same policy as
// golang.org/x/sys/windows.NewLazySystemDLL, without the pre-Windows-10
// fallbacks: Go itself requires Windows 10+, where the flag always works).
//
// The loader bootstraps from kernel32.dll, which is a KnownDLL mapped into
// every process before user code runs, so resolving it by name is safe.

// loadLibrarySearchSystem32 restricts the search to %windows%\system32
// (LOAD_LIBRARY_SEARCH_SYSTEM32).
const loadLibrarySearchSystem32 = 0x00000800

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procLoadLibraryExW = kernel32.NewProc("LoadLibraryExW")
)

// Sentinels matched by errors.Is against a *ProcError.
var (
	// ErrDLLNotFound reports a DLL that could not be loaded from System32
	// (an optional component, or a newer Windows than this one).
	ErrDLLNotFound = errors.New("win32: DLL not found in System32")
	// ErrProcNotFound reports a DLL that loaded but lacks the export (the API
	// is newer than this Windows build).
	ErrProcNotFound = errors.New("win32: procedure not found")
)

// ProcError is the resolution failure of a lazily loaded export: Proc.Find
// returns it and Proc.Addr panics with it, so a recover handler can
// errors.As the recovered value to learn which DLL or export was missing. It
// matches ErrDLLNotFound (Proc == "") or ErrProcNotFound with errors.Is, and
// unwraps to the underlying syscall.Errno (ERROR_MOD_NOT_FOUND,
// ERROR_PROC_NOT_FOUND, …).
type ProcError struct {
	// DLL is the module name as declared in the metadata ("KERNEL32.dll").
	DLL string
	// Proc is the export name, or "" when the DLL itself failed to load.
	Proc string
	// Err is the underlying LoadLibraryExW / GetProcAddress error.
	Err error
}

func (e *ProcError) Error() string {
	if e.Proc == "" {
		return fmt.Sprintf("win32: loading %s: %v", e.DLL, e.Err)
	}
	return fmt.Sprintf("win32: %s: procedure %s not found: %v", e.DLL, e.Proc, e.Err)
}

func (e *ProcError) Unwrap() error { return e.Err }

// Is matches the ErrDLLNotFound / ErrProcNotFound sentinels.
func (e *ProcError) Is(target error) bool {
	if e.Proc == "" {
		return target == ErrDLLNotFound
	}
	return target == ErrProcNotFound
}

// loadSystemDLL loads name from System32 only.
func loadSystemDLL(name string) (syscall.Handle, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, &ProcError{DLL: name, Err: err}
	}
	handle, _, callErr := procLoadLibraryExW.Call(
		uintptr(unsafe.Pointer(namePtr)), 0, loadLibrarySearchSystem32)
	if handle == 0 {
		return 0, &ProcError{DLL: name, Err: callErr}
	}
	return syscall.Handle(handle), nil
}

// DLL is a lazily loaded system DLL. Generated <pkg>_runtime.go files declare
// one package-level *DLL per imported DLL.
type DLL struct {
	name   string
	once   sync.Once
	handle syscall.Handle
	err    error
}

// NewDLL returns a lazy handle to a system DLL (loaded from System32 only,
// never the application directory, to prevent DLL preloading attacks).
func NewDLL(name string) *DLL {
	return &DLL{name: name}
}

func (d *DLL) load() error {
	d.once.Do(func() {
		d.handle, d.err = loadSystemDLL(d.name)
	})
	return d.err
}

// Proc is a lazily resolved exported procedure.
type Proc struct {
	dll  *DLL
	name string
	once sync.Once
	addr uintptr
	err  error
}

// NewProc returns a lazy reference to an export of the DLL.
func (d *DLL) NewProc(name string) *Proc {
	return &Proc{dll: d, name: name}
}

func (p *Proc) find() error {
	p.once.Do(func() {
		if err := p.dll.load(); err != nil {
			p.err = err
			return
		}
		addr, err := syscall.GetProcAddress(p.dll.handle, p.name)
		if err != nil {
			p.err = &ProcError{DLL: p.dll.name, Proc: p.name, Err: err}
			return
		}
		p.addr = addr
	})
	return p.err
}

// Addr resolves and returns the procedure address. A syscall cannot proceed
// without one, so an unavailable DLL or export panics with the *ProcError
// that Find would return; probe with Find first (every generated package
// exposes its procs as Procs.<Function>) when the API may be absent on the
// running Windows build.
func (p *Proc) Addr() uintptr {
	if err := p.find(); err != nil {
		panic(err)
	}
	return p.addr
}

// Find resolves the procedure, reporting a *ProcError instead of panicking;
// use it to probe for APIs that are not present on older Windows versions or
// optional components.
func (p *Proc) Find() error {
	return p.find()
}

// Name returns the export name.
func (p *Proc) Name() string { return p.name }

// DLLName returns the module the export is resolved from.
func (p *Proc) DLLName() string { return p.dll.name }
