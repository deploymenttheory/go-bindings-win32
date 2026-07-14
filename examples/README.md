# Examples — and how to adopt the bindings in your own project

Runnable programs that exercise the generated Win32 bindings the way a real
program would. Each has its own README for *what it does*; this page is the
cross-cutting guide for *how and why* to use the bindings.

Everything here is Windows-only, amd64/arm64 (`//go:build windows`).

## The examples

| Example | What it shows | Needs admin? |
|---|---|---|
| [`sysinfo`](sysinfo) | Read-only host info — computer name, user, CPU topology, memory, OS version. Size-probe strings, self-sized structs, and a C union. | No |
| [`localaccount`](localaccount) | Full lifecycle of a local user account — create, query, enumerate, delete (`NetUserAdd`/`GetInfo`/`Enum`/`Del`). Structs, constants, `NetApiBuffer` ownership, handle-free cleanup. | Only with `-apply` |

Both import the generated packages under `bindings/win32` (plus the runtime).
Run one with `go run ./examples/<name>`. `sysinfo` is entirely read-only;
`localaccount` does a safe read-only dry run unless you pass `-apply` (which
needs Administrator to create an account) and self-cleans what it creates.

## What you import, and why

There is one bindings tree, and it is already idiomatic-shaped. Import the
generated package for each namespace you use, plus the runtime.

| Package | Import path | What it gives you |
|---|---|---|
| **Bindings** | `bindings/win32/<namespace>` | The generated Win32 surface, shaped for Go: Go strings for `PWSTR`, Go `bool` for `BOOL`, `error` for `HRESULT`/`SetLastError`, `[]T` for array+count pairs, `[out,retval]` lifted to return values, `Close<Handle>` helpers, and COM interfaces as method-bearing wrapper types. Every struct, typed constant, and pass-through function lives here too. Each call dispatches through `syscall.SyscallN` inline. |
| **Runtime** | `bindings/runtime/win32` | Shared, low-level helpers the bindings rely on: `UTF16Ptr`/`UTF16ToString`, `GUID`, `ErrIfFailed`, `Bool32`. Not a binding — a small toolbox for the boundary. |

The rule of thumb: **import the `bindings/win32/<namespace>` packages you need
and the runtime.** Everything — the improved signatures and the structs and
constants they use — lives in that one tree.

## Prerequisites every adopter hits

- **Architecture.** Generated code is tagged `windows && (amd64 || arm64)`.
  32-bit (386) is not supported.
- **Privileges.** Many APIs need elevation. Account creation, service control,
  and most `NetworkManagement`/`Security` mutations require an Administrator
  token; the examples detect access-denied and explain rather than crash.
- **Ownership of unmanaged memory.** Some APIs hand back a buffer you must free
  (`NetApiBufferFree`, `LocalFree`, `CoTaskMemFree`). The bindings surface
  these; use `defer` to release them (see `localaccount`).
