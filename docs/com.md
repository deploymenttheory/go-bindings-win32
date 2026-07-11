# Using COM interfaces

COM interfaces are generated as **method-bearing structs**: the generated
interface struct *is* the COM object. It carries the vtable pointer and
dispatches through it, with `HRESULT` methods surfaced as `error`.

## The shape

For an interface `IFoo`, the package emits:

- `type IFoo struct { … }` — a **root** interface carries
  `LpVtbl *[1024]uintptr`; a **derived** interface embeds its base struct (so
  inherited methods like `QueryInterface` are promoted). The struct *is* the
  COM object — there is no separate wrapper and no `.Raw` field.
- methods that dispatch through the vtable, returning `error` for `HRESULT` and
  lifting `[out, retval]` parameters into Go return values.

## Getting an interface

Most objects come from a factory function (or `QueryInterface`) that hands back
a typed pointer via an out parameter:

```go
import (
	com "…/bindings/win32/system/com"
	"…/bindings/win32/system/com/structuredstorage"
)

var stream *com.IStream
if err := structuredstorage.CreateStreamOnHGlobal(0, true, &stream); err != nil {
	return err
}
defer stream.Release()
```

## Calling methods

Methods return `error`; base-interface methods are promoted through embedding;
`[out, retval]` values become returns:

```go
// promoted from ISequentialStream; returns error
if err := stream.Write(unsafe.Pointer(&data[0]), uint32(len(data)), &written); err != nil {
	return err
}

// an automation getter: [out,retval] BSTR lifted to a return value
name, err := eventClass.Get_EventClassID() // (foundation.BSTR, error)
```

Interface **parameters** take the typed pointer directly — pass the interface
struct pointer:

```go
appVisibility.Advise(myCallback /* an *IAppVisibilityEvents */, &cookie)
```

Under the hood, each method dispatches through its vtable slot:

```go
syscall.SyscallN(self.LpVtbl[slot], uintptr(unsafe.Pointer(self)), args…)
```

## Lifetimes

COM uses reference counting. The rules the bindings expect:

- **`Release` what you own.** A factory or `QueryInterface` hands you a
  reference; release it (`stream.Release()`) when done — `defer` is idiomatic.
- **`AddRef` if you keep an extra copy.** Storing a second long-lived
  reference means an extra `AddRef` and a matching `Release`.
- **`QueryInterface`** returns a new reference; release it separately.

```go
var unk unsafe.Pointer
if err := stream.QueryInterface(&com.IID_IUnknown, &unk); err != nil {
	return err
}
(*com.IUnknown)(unk).Release() // release the QI reference
```

## Apartments

Creating objects via `CoCreateInstance` requires an initialized COM apartment
(`CoInitializeEx`) on the calling thread, and cross-apartment calls have
threading rules COM enforces — those are your responsibility. Objects created
by plain factory functions (like `CreateStreamOnHGlobal`) need no apartment.

## GUIDs / IIDs

Every interface's IID is generated as `IID_IFoo`, and coclass CLSIDs as
`CLSID_*`, in the same package — pass a pointer to them where an API wants an
`*GUID`.
