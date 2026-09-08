# volume — the default playback device, through COM

A **no-privileges**, read-only program that prints the name of the default
audio output device with its master volume and mute state, using the generated
Core Audio bindings. It is the COM tour: an apartment, a coclass created by
CLSID, interfaces cast from `IUnknown`, a property store read through a
`PROPVARIANT`, and a device-scoped interface activated off the endpoint.

```sh
go run ./examples/volume
```
```
default playback device
-----------------------
name:       5 - Dell S2417DG (AMD High Definition Audio Device)
volume:     100%
muted:      false
```

It needs at least one active playback device. It changes nothing.

## What it shows

| Pattern | Where | Notes |
|---|---|---|
| **Apartment lifetime** | `CoInitializeEx` / `CoUninitialize` | An apartment belongs to a *thread*, so the goroutine is pinned with `runtime.LockOSThread`. `CoInitializeEx` is one of the curated informational-success APIs: it returns `(win32.HRESULT, error)` so `S_FALSE` — already in a compatible apartment — survives as the success it is. |
| **Coclass by CLSID** | `CoCreateInstance(&audio.CLSID_MMDeviceEnumerator, …)` | A coclass has no layout of its own, so it exists in the bindings only as its `CLSID_*` GUID, next to the `IID_*` of every interface in the same package. |
| **riid-selected out-params** | `CoCreateInstance`, `IMMDevice::Activate` | A `void**` out-param whose type the `riid` argument picks at runtime is typed `**win32.IUnknown`. `win32.Cast[T]` reinterprets the result as the interface the riid selected — no `AddRef`, the factory's reference is now held through the typed pointer. |
| **Typed interface out-params** | `IMMDeviceEnumerator::GetDefaultAudioEndpoint`, `IMMDevice::OpenPropertyStore` | When the metadata names the interface, the out-param is already `**IMMDevice` / `**IPropertyStore` — no cast needed. |
| **Tagged union** | `IPropertyStore::GetValue` → `PROPVARIANT` | See below. |
| **`BOOL` out-param** | `IAudioEndpointVolume::GetMute` | A Win32 `BOOL` out-param is exposed as `*bool`; the 4-byte value the callee writes is converted at the boundary. |
| **Unmanaged ownership** | `PropVariantClear` | The variant owns whatever it points at. `defer` the clear, exactly as you would `LocalFree`/`CoTaskMemFree`. |

## Reading a PROPVARIANT

`PROPVARIANT` is a tagged union, and the *whole struct* is one anonymous union
— which is why it used to be opaque. Every union member now has a typed
accessor (all members overlay the same storage from offset 0, so each accessor
is a single reinterpretation), and the structs nested inside the union are
emitted too. So the discriminant is a real `VARENUM` and the payload arrives as
its own type:

```go
var value structuredstorage.PROPVARIANT
store.GetValue(&functiondiscovery.PKEY_Device_FriendlyName, &value)
defer structuredstorage.PropVariantClear(&value)

tagged := value.Anonymous.Anonymous() // the vt-carrying overlay
if tagged.Vt == variant.VT_LPWSTR {
	name := win32.UTF16ToString((*uint16)(*tagged.Anonymous.PwszVal()))
}
```

The example imports no `unsafe`.

`PKEY_Device_FriendlyName` lives in `devices/functiondiscovery` — the property
keys are grouped by the namespace that defines them, not by the one that reads
them. Note that `foundation.PROPERTYKEY` and `foundation.DEVPROPKEY` share a
layout but are distinct Win32 types with distinct key sets; `IPropertyStore`
wants the `PROPERTYKEY` one, so `properties.DEVPKEY_Device_FriendlyName` is
**not** its substitute.

## Credit

The original version of this example was contributed by
[@mathertel](https://github.com/mathertel) in
[#45](https://github.com/deploymenttheory/go-bindings-win32/pull/45). The
workarounds it needed drove four generator fixes; this is the same program
written against them.
