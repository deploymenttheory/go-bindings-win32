# Using COM interfaces

COM interfaces are generated as **method-bearing wrapper types**: a Go struct
that holds the raw interface pointer and dispatches through its vtable, with
`HRESULT` methods surfaced as `error`.

## The shape

For an interface `IFoo`, the idiomatic package emits:

- `type IFoo struct { … Raw *rawpkg.IFoo }` — embeds its idiomatic base
  wrapper (so inherited methods like `QueryInterface` are promoted), and holds
  the raw pointer.
- `func WrapIFoo(raw *rawpkg.IFoo) IFoo` — wrap a raw pointer you obtained.
- methods that forward to the vtable, returning `error` for `HRESULT` and
  lifting `[out, retval]` parameters into Go return values.

## Getting an interface

Most objects come from a factory function that hands back a raw pointer via an
out parameter; wrap it:

```go
import (
	rawcom "…/bindings/win32/system/com"
	"…/bindings/win32/system/com/structuredstorage"
	com "…/opinionated/idiomatic/win32/system/com"
)

var raw *rawcom.IStream
if hr := structuredstorage.CreateStreamOnHGlobal(0, 1, &raw); hr != 0 {
	return win32.HRESULTError(int32(hr))
}
defer raw.Release()

stream := com.WrapIStream(raw)
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

Interface **parameters** also take wrapper values — pass the wrapper, the
binding forwards its `.Raw`:

```go
appVisibility.Advise(myCallback /* an IAppVisibilityEvents wrapper */, &cookie)
```

## Lifetimes

COM uses reference counting. The rules the bindings expect:

- **`Release` what you own.** A factory or `QueryInterface` hands you a
  reference; release it (`raw.Release()` or the promoted `stream.Release()`)
  when done — `defer` is idiomatic.
- **`AddRef` if you keep an extra copy.** Storing a second long-lived
  reference means an extra `AddRef` and a matching `Release`.
- **`QueryInterface`** returns a new reference through a `ComOutPtr`; release
  it separately.

```go
var unk unsafe.Pointer
if err := stream.QueryInterface(&rawcom.IID_IUnknown, &unk); err != nil {
	return err
}
(*rawcom.IUnknown)(unk).Release() // release the QI reference
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
