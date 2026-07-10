# sysinfo — read-only host information

A tiny, **no-privileges** program that prints host details gathered through the
generated Win32 bindings: computer name, current user, processor topology,
physical memory, and the reported OS version. It is the gentlest tour of the
bindings — nothing it does modifies the system.

```sh
go run ./examples/sysinfo
```
```
host information (via go-bindings-win32)
----------------------------------------
hostname:   MYBOX
user:       alice
cpus:       16 logical
page size:  4096 bytes (allocation granularity 65536)
arch:       x64 (AMD64)
memory:     65456 MiB total, 48569 MiB free (25% in use)
version:    6.2 build 9200 (GetVersionEx; manifest-dependent)
```

## What it shows

Everything comes from the **idiomatic** packages (plus the runtime for UTF-16);
it never imports `bindings/win32`. Along the way it demonstrates three Win32
patterns the bindings surface faithfully:

| Pattern | Where | Notes |
|---|---|---|
| **Size probe, then fill** | `GetComputerNameEx`, `GetUserName` | Call once with a `nil` buffer to learn the required length, allocate a `[]uint16`, call again. |
| **Struct with a self-size field** | `MEMORYSTATUSEX.DwLength`, `OSVERSIONINFOW.DwOSVersionInfoSize` | Set the size field to `unsafe.Sizeof(v)` before the call or the API rejects it. |
| **Value struct filled by a void call** | `GetNativeSystemInfo(&SYSTEM_INFO)` | Includes a leading **C union** (`dwOemId` overlaying `wProcessorArchitecture`), exposed as a correctly sized backing blob you read via `.Anonymous.Data[0]`. |

## A note on the OS version

`GetVersionEx` is deprecated and, without an application *compatibility
manifest*, Windows shims it to report **6.2 (Windows 8)** even on Windows
10/11 — hence the `6.2 build 9200` above. That's a genuine Win32 gotcha, shown
here on purpose. For a true build number, read
`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion` from the registry.
