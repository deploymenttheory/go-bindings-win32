# volume — inspect the default playback device

A small Windows-only program that opens the default audio endpoint, reads its
friendly name, and reports the current master volume and mute state using the
generated Win32 bindings. It is a focused COM and Core Audio demo: it shows how
interfaces are returned as `IUnknown` and then cast to the correct typed
interface, how to initialize COM on a single-threaded apartment, and how to
handle a `PROPVARIANT` that carries a UTF-16 string value.

## What it shows

| Pattern | Win32 API / surface | Symbol source |
|---|---|---|
| COM startup | `CoInitializeEx` / `CoUninitialize` | `bindings/win32/system/com` |
| Create the device enumerator | `CoCreateInstance` + `IMMDeviceEnumerator` | `bindings/win32/media/audio` |
| Pick the default render endpoint | `IMMDeviceEnumerator::GetDefaultAudioEndpoint` | `bindings/win32/media/audio` |
| Open the property store | `IMMDevice::OpenPropertyStore` | `bindings/win32/media/audio` |
| Read a `PROPVARIANT` value | `IPropertyStore::GetValue` | `bindings/win32/media/audio` / `bindings/win32/system/com/structuredstorage` |
| Activate the endpoint volume interface | `IMMDevice::Activate` | `bindings/win32/media/audio/endpoints` |
| Read current volume | `IAudioEndpointVolume::GetMasterVolumeLevelScalar` | `bindings/win32/media/audio/endpoints` |
| Read mute state | `IAudioEndpointVolume::GetMute` | `bindings/win32/media/audio/endpoints` |

The program relies on the generated packages for the Core Audio interfaces and
constants, and on the shared runtime for the UTF-16 string conversion and the
generic COM casting helpers (`Cast`, `QueryInterface`, `IUnknown`).

## Running it

```sh
go run ./examples/volume
```

This expects a Windows machine with at least one active playback device. It does
not modify system state; it only reads the current default output device and its
volume status.

## Notes

This example intentionally exercises a few Win32 quirks that are easy to miss:

- Core Audio interfaces are COM objects and are surfaced as typed interfaces
  after casting from the runtime `IUnknown` root.
- `PROPVARIANT` values can carry a UTF-16 string in a union-backed payload, so
  the sample reads the pointer from the variant's union data and converts it
  using the runtime's UTF-16 helpers.
- COM initialization is apartment-aware, and the sample locks the thread to the
  apartment it uses before calling the Core Audio APIs.
