# Strings, structs, and memory

Win32 is a C ABI: UTF-16 strings, structs with exact layouts, caller-owned
buffers, and opaque handles. The bindings smooth the common cases and leave
the rest faithful. This guide covers the patterns you'll hit repeatedly.

## Strings: UTF-16 at the boundary

Win32 wide strings are `PWSTR`/`PCWSTR` — pointers to NUL-terminated UTF-16.

**Input strings** are already handled: the idiomatic layer turns an input
`PWSTR` parameter into a Go `string` and converts it for you.

```go
event, _ := threading.CreateEvent(nil, true, false, "my-event") // Go string in
```

**Output/buffer strings** stay `PWSTR` (they're memory you provide), and you
convert with the runtime. The idiom is *size-probe then fill*:

```go
import "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"

var size uint32
_ = sysinfo.GetComputerNameEx(kind, nil, &size) // probe: fails, sets size
buf := make([]uint16, size)
sysinfo.GetComputerNameEx(kind, foundation.PWSTR(&buf[0]), &size)
name := win32.UTF16ToString(&buf[0])            // back to a Go string
```

Runtime helpers: `win32.UTF16Ptr(s string) *uint16` (Go → UTF-16, for filling
a struct field), and `win32.UTF16ToString(*uint16) string` (UTF-16 → Go).

## Structs

Generated structs match the C layout exactly (amd64/arm64 LLP64), so you can
pass a pointer straight to the API.

**Self-size fields.** Many structs carry their own size in the first field;
set it before the call:

```go
var mem sysinfo.MEMORYSTATUSEX
mem.DwLength = uint32(unsafe.Sizeof(mem))
sysinfo.GlobalMemoryStatusEx(&mem)
```

**Unions** are exposed as a correctly sized, correctly aligned backing blob
(a struct with a `Data [N]uintN` field). Read the overlay you need:

```go
var si sysinfo.SYSTEM_INFO
sysinfo.GetNativeSystemInfo(&si)
arch := uint16(si.Anonymous.Data[0] & 0xFFFF) // wProcessorArchitecture
```

**Bitfields** become `_bitfieldN` backing fields; mask/shift to read members
(typed accessors are a future addition).

**Packed structs** that Go cannot reproduce byte-for-byte are deliberately
**not emitted** (with a diagnostic) rather than emitted with a wrong layout.
If you need one, define it yourself and call the raw function.

## Memory ownership

Some APIs return a buffer they allocated; you must free it with the matching
function. The idiomatic package re-exports those frees, so `defer` them:

```go
var buf *byte
netmanagement.NetUserGetInfo("", user, 1, &buf)
defer netmanagement.NetApiBufferFree(unsafe.Pointer(buf)) // free what the API gave you
info := (*netmanagement.USER_INFO_1)(unsafe.Pointer(buf))
```

Common frees: `NetApiBufferFree`, `LocalFree`, `CoTaskMemFree`,
`SysFreeString`.

## Handles and RAII

Handles are opaque `uintptr`-backed types (`HANDLE`, `HKEY`, …). Where the
metadata records how a handle is closed (`[RAIIFree]`), the idiomatic package
generates a `Close<Handle>(h) error` helper — `defer` it:

```go
h, err := threading.CreateEvent(nil, true, false, "")
if err != nil { return err }
defer foundation.CloseHANDLE(h) // → CloseHandle, normalized to error
```

`Close<Handle>` returns an `error` so a failed close is observable; ignore it
with `_ =` if you don't care.

## `unsafe` is expected here

Passing struct pointers and reinterpreting API buffers uses `unsafe.Pointer`
and `unsafe.Sizeof` — that's inherent to a C ABI, not a smell. Keep the
`unsafe` at the call boundary and expose ordinary Go types to the rest of your
program (the [examples](../examples) do exactly this).
