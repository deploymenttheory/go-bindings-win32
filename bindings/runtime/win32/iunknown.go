package win32

import (
	"syscall"
	"unsafe"
)

// IUnknown is the root COM object shape. Generated COM interface structs
// share this exact layout (a vtable pointer as the first and only word), so
// any generated interface pointer (*IFoo) can be cast to and from *IUnknown
// with unsafe.Pointer. Generated bindings use **IUnknown for [ComOutPtr] /
// [IidParameterIndex] out-params, whose concrete interface is selected at
// runtime by the accompanying riid argument.
type IUnknown struct {
	LpVtbl *[1024]uintptr
}

// QueryInterface dispatches through IUnknown's vtable slot 0.
func (u *IUnknown) QueryInterface(riid *GUID, ppv **IUnknown) error {
	r1, _, _ := syscall.SyscallN(u.LpVtbl[0], uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(riid)), uintptr(unsafe.Pointer(ppv)))
	return HRESULTError(int32(r1))
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
