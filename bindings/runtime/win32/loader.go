package win32

import (
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

// loadSystemDLL loads name from System32 only.
func loadSystemDLL(name string) (syscall.Handle, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, fmt.Errorf("win32: DLL name %q: %w", name, err)
	}
	handle, _, callErr := procLoadLibraryExW.Call(
		uintptr(unsafe.Pointer(namePtr)), 0, loadLibrarySearchSystem32)
	if handle == 0 {
		return 0, fmt.Errorf("win32: loading %s: %w", name, callErr)
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
			p.err = fmt.Errorf("win32: %s: procedure %s not found: %w", p.dll.name, p.name, err)
			return
		}
		p.addr = addr
	})
	return p.err
}

// Addr resolves and returns the procedure address, panicking with a clear
// message if the DLL or export is unavailable on this system.
func (p *Proc) Addr() uintptr {
	if err := p.find(); err != nil {
		panic(err)
	}
	return p.addr
}

// Find resolves the procedure, reporting an error instead of panicking; use
// it to probe for APIs that are not present on older Windows versions.
func (p *Proc) Find() error {
	return p.find()
}
