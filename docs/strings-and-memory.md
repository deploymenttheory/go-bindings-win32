# Strings, structs, and memory

Win32 is a C ABI: UTF-16 strings, structs with exact layouts, caller-owned
buffers, and opaque handles. The bindings smooth the common cases and leave
the rest faithful. This guide covers the patterns you'll hit repeatedly.

## Strings: UTF-16 at the boundary

Win32 wide strings are `PWSTR`/`PCWSTR` — pointers to NUL-terminated UTF-16.

**Input strings** are already handled: the bindings turn an input
`PWSTR` parameter into a Go `string` and convert it for you.

```go
opened, _ := threading.OpenEvent(access, false, "my-event") // Go string in
```

**Optional input strings** — parameters the metadata marks `[Optional]`,
where the API distinguishes NULL from an empty string (`CreateEvent`'s name,
`NetUser*`'s server name, `CreateProcess`'s application name and current
directory, …) — are `*string`: `nil` passes NULL, `win32.Str("x")` (or
`&name`) passes the string. A Go `""` can never silently become `L""` where
NULL was meant.

```go
event, _ := threading.CreateEvent(nil, true, false, nil)                  // unnamed: NULL
named, _ := threading.CreateEvent(nil, true, false, win32.Str("my-event")) // L"my-event"
```

`activedirectory.DsRoleGetPrimaryDomainInformation` also takes `lpServer
*string`. Its native contract allows NULL for the local computer even though
the current metadata omits `[Optional]`. Pass `nil` for the local computer or
`win32.Str(server)` for an explicit name; `win32.Str("")` still passes an empty
string, not NULL. Existing calls that supplied `server` as a Go string must
change that argument to `win32.Str(server)`. The returned `uint32` remains the
native status (zero is success), and the returned buffer must be released with
`DsRoleFreeMemory`. See the [native API contract](https://learn.microsoft.com/en-us/windows/win32/api/dsrole/nf-dsrole-dsrolegetprimarydomaininformation).

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
a struct field; it panics on an embedded NUL), `win32.UTF16PtrOrNil(*string)`
(nil → NULL), `win32.Str(s) *string`, and `win32.UTF16ToString(*uint16) string`
(UTF-16 → Go).

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
If you need one, define it yourself and call the function directly.

## Memory ownership

Some APIs return a buffer they allocated; you must free it with the matching
function. The bindings expose those frees, so `defer` them:

```go
var buf *byte
netmanagement.NetUserGetInfo(nil, user, 1, &buf) // nil server name: the local machine
defer netmanagement.NetApiBufferFree(unsafe.Pointer(buf)) // free what the API gave you
info := (*netmanagement.USER_INFO_1)(unsafe.Pointer(buf))
```

Common frees: `NetApiBufferFree`, `LocalFree`, `CoTaskMemFree`,
`SysFreeString`.

LDAP's `Ldap_memfreeW` takes the original `foundation.PWSTR` returned by
the LDAP library. Although its metadata calls this an input string, the
pointer identifies the allocation to release. Do not convert it to a Go
string and back before freeing it, or pass a Go-owned UTF-16 buffer.

```go
name := ldap.Ldap_dn2ufnW("CN=Example,DC=example,DC=test")
if name != nil {
    text := win32.UTF16ToString(name)
    ldap.Ldap_memfreeW(name)
    // text is a Go copy; name is no longer valid.
    _ = text
}
```

The generator preserves this pointer explicitly. Versions through v0.6.0
incorrectly accepted a Go string for `Ldap_memfreeW`; callers must migrate
to passing the original LDAP pointer. See Microsoft's
[allocation contract](https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_dn2ufnw)
and [deallocation contract](https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_memfreew).

## LDAP credentials depend on the bind method

`Ldap_bind_sW` takes `cred foundation.PWSTR`, preserving the original nullable
pointer. Its C signature names a wide-character pointer, but the bind method
determines what it points to. For current-identity Negotiate, pass `nil` for both
DN and credentials. For explicit Negotiate, pass a pointer to a correctly
initialized `rpc.SEC_WINNT_AUTH_IDENTITY_W` (or a supported extended identity)
with Unicode buffers, character counts, and the Unicode flag. Never reinterpret
a Go string as that structure.

```go
// identity and its buffers are caller-owned and initialized for Unicode.
credential := foundation.PWSTR(unsafe.Pointer(&identity))
status := ldap.Ldap_bind_sW(connection, nil, credential, negotiateMethod)
runtime.KeepAlive(&identity)
```

Keep all referenced storage alive through the synchronous native call and
follow the authentication provider's lifetime contract. The binding neither
constructs nor frees the identity or its buffers. Callers must protect and clear
sensitive storage when it is no longer required. This pointer also supports
SIMPLE's password-buffer contract; choose appropriate transport protection
before using that method. The binding does not choose authentication policy.

Versions through v0.6.0 incorrectly accepted `cred *string` and created a new
UTF-16 string. Current-identity calls with `nil` remain source compatible.
Explicit callers must supply the native pointer appropriate for their selected
method. Ordinary DN conversion and asynchronous SIMPLE bind are unchanged.
See Microsoft's [bind contract](https://learn.microsoft.com/windows/win32/api/winldap/nf-winldap-ldap_bind_sw)
and [Unicode identity layout](https://learn.microsoft.com/windows/win32/api/sspi/ns-sspi-sec_winnt_auth_identity_w).

## LDAP client-certificate callbacks

`Ldap_set_client_certificate(connection, callback)` is a typed helper over
`ldap_set_optionW` with `LDAP_OPT_CLIENT_CERTIFICATE`. The native option consumes
the `QUERYCLIENTCERT` code pointer by value. Passing `&callback` to the generic
pointer-valued option setter would install the address of Go data as executable
code. The helper dispatches the callback word directly and reuses the existing
`Procs.Ldap_set_optionW` availability probe; it adds no native export.

Use `syscall.NewCallbackCDecl` with the documented callback shape. Keep callbacks
and any referenced state valid through connection cleanup; Go callback thunks
are not released, so prefer a process-wide callback for a fixed policy. A zero
callback removes the selection routine. This helper does not implement client
certificate selection or choose a transport/authentication policy.

A callback that returns false declines a client certificate. WinLDAP derives
`SCH_CRED_NO_DEFAULT_CREDS` versus `SCH_CRED_USE_DEFAULT_CREDS` from whether a
client-certificate routine is installed, independently of automatic server
certificate validation. Native, network-free tests verify that the actual code
address round-trips through the option and that a callback preserves requested
automatic validation and no-default-credential flags. They do not prove a TLS
handshake or certificate validation. See Microsoft's
[callback contract](https://learn.microsoft.com/windows/win32/api/winldap/nc-winldap-queryclientcert),
[Schannel option behavior](https://learn.microsoft.com/previous-versions/windows/desktop/ldap/session-options),
and [.NET's callback-by-value adapter](https://github.com/dotnet/runtime/blob/main/src/libraries/Common/src/Interop/Windows/Wldap32/Interop.Ldap.cs).

## Handles and closers

Handles are opaque `uintptr`-backed types (`HANDLE`, `HKEY`, …). Where the
metadata records how a handle is closed (`[RAIIFree]`), the bindings
generate a `Close<Handle>(h) error` helper — `defer` it. Go has no
destructors, so this is a plain function, not RAII: nothing closes a handle
you forget to close.

```go
h, err := threading.CreateEvent(nil, true, false, nil)
if err != nil { return err }
defer foundation.CloseHANDLE(h) // → CloseHandle, normalized to error
```

Closing the zero value or the handle's `[InvalidHandleValue]` sentinel
(`INVALID_HANDLE_VALUE` for `HANDLE`) is a no-op that returns `nil`, so a
`defer` on an error path is safe. Closing a live handle twice is still an
error. `Close<Handle>` returns an `error` where the free function reports
one (`CloseHandle`, `RegCloseKey`, …); helpers over `void` frees such as
`CloseThreadpool` always return `nil`.

## `unsafe` is expected here

Passing struct pointers and reinterpreting API buffers uses `unsafe.Pointer`
and `unsafe.Sizeof` — that's inherent to a C ABI, not a smell. Keep the
`unsafe` at the call boundary and expose ordinary Go types to the rest of your
program (the [examples](../examples) do exactly this).
