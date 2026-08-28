//go:build windows

package win32

import (
	"syscall"
	"unsafe"
)

// IUnknown is the root COM object shape, and the type the generated
// System.Com package aliases (`com.IUnknown = win32.IUnknown`). Every
// generated interface struct embeds it (directly or through its base), so a
// *IFoo is one vtable word with QueryInterface/AddRef/Release promoted, and
// `&foo.IUnknown` is the object itself upcast to the root.
//
// Generated bindings use **IUnknown for [ComOutPtr] / riid-paired void**
// out-params, whose concrete interface is selected at runtime by the riid
// argument: Cast[T] reinterprets such a result as the *T the riid selected,
// and QueryInterface[T] asks any object for another interface as *T.
type IUnknown struct {
	LpVtbl *[1024]uintptr
}

// Unknown is satisfied by *IUnknown and by every generated COM interface
// pointer (the root's methods are promoted through embedding).
type Unknown interface {
	QueryInterface(riid *GUID, ppv **IUnknown) error
}

// QueryInterface asks obj for the interface iid identifies and returns it as
// *T, where T is the generated interface struct that iid selects:
//
//	stream, err := win32.QueryInterface[com.IStream](obj, &com.IID_IStream)
//
// The caller owns the returned reference (Release it). The out slot is
// heap-escaped (see outparam.go). T must be a single-word interface struct.
func QueryInterface[T any](obj Unknown, iid *GUID) (*T, error) {
	assertInterfaceStruct[T]()
	out := new(*IUnknown)
	if err := obj.QueryInterface(iid, (**IUnknown)(OutParam(unsafe.Pointer(out)))); err != nil {
		return nil, err
	}
	return (*T)(unsafe.Pointer(*out)), nil
}

// Cast reinterprets a COM object received as *IUnknown — the type of every
// [ComOutPtr] / riid-paired factory out-param — as the concrete interface *T
// the accompanying riid selected:
//
//	var out *win32.IUnknown
//	err := xmllite.CreateXmlReader(&xmllite.IID_IXmlReader, &out, nil)
//	reader := win32.Cast[xmllite.IXmlReader](out)
//
// No AddRef occurs: the reference the factory handed out is now held through
// reader. Upcasting needs no helper — `&reader.IUnknown` is the same object
// as the root. T must be a single-word interface struct.
func Cast[T any](unk *IUnknown) *T {
	assertInterfaceStruct[T]()
	return (*T)(unsafe.Pointer(unk))
}

// assertInterfaceStruct panics unless T has the one-vtable-word layout every
// generated interface struct shares (a compile-time-constant comparison).
func assertInterfaceStruct[T any]() {
	var zero T
	if unsafe.Sizeof(zero) != unsafe.Sizeof(uintptr(0)) {
		panic("win32: type argument is not a single-word COM interface struct")
	}
}

// QueryInterface dispatches through IUnknown's vtable slot 0.
func (u *IUnknown) QueryInterface(riid *GUID, ppv **IUnknown) error {
	r1, _, _ := syscall.SyscallN(u.LpVtbl[0], uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(riid)), uintptr(unsafe.Pointer(ppv)))
	return ErrIfFailed(int32(r1))
}

// AddRef dispatches through IUnknown's vtable slot 1.
func (u *IUnknown) AddRef() uint32 {
	r1, _, _ := syscall.SyscallN(u.LpVtbl[1], uintptr(unsafe.Pointer(u)))
	return uint32(r1)
}

// Release dispatches through IUnknown's vtable slot 2.
func (u *IUnknown) Release() uint32 {
	r1, _, _ := syscall.SyscallN(u.LpVtbl[2], uintptr(unsafe.Pointer(u)))
	return uint32(r1)
}
