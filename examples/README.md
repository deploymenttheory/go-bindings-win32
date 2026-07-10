# Examples — and how to adopt the bindings in your own project

Runnable programs that exercise the generated Win32 bindings the way a real
program would. Each has its own README for *what it does*; this page is the
cross-cutting guide for *how and why* to use the bindings.

Everything here is Windows-only, amd64/arm64 (`//go:build windows`).

## The examples

| Example | What it shows | Bindings it uses |
|---|---|---|
| [`localaccount`](localaccount) | Full lifecycle of a local user account — create, query, enumerate, delete (`NetUserAdd`/`NetUserGetInfo`/`NetUserEnum`/`NetUserDel`) | the **idiomatic** `NetworkManagement.NetManagement` package (only) |

Run one with `go run ./examples/<name>`. Some do system-modifying work behind
an explicit flag (e.g. `localaccount` needs `-apply` and Administrator to
create an account); by default they do a safe, read-only dry run.

## Which layer should I use, and why?

The repo offers the same Win32 API at two levels. Prefer the idiomatic layer;
drop to raw only for something it doesn't cover.

| Layer | Import path | Use it for |
|---|---|---|
| **Idiomatic** | `opinionated/idiomatic/win32/<namespace>` | **The default.** Go strings for `PWSTR`, Go `bool` for `BOOL`, `error` for `HRESULT`/`SetLastError`, `[]T` for array+count pairs, `[out,retval]` lifted to return values, `Close<Handle>` helpers, and COM interfaces as method-bearing wrapper types. It is **self-contained** — it re-exports every struct, constant, and pass-through function it doesn't otherwise improve, so you never import the raw package. |
| **Raw** | `bindings/win32/<namespace>` | The 1:1 `syscall` surface. Reach for it only if you need a construct the idiomatic layer degraded (rare) or want the un-adapted signature. |
| **Runtime** | `bindings/runtime/win32` | Shared, low-level helpers both tiers rely on: `UTF16Ptr`/`UTF16ToString`, `GUID`, `HRESULTError`, `Bool32`. Not a binding — a small toolbox for the boundary. |

The rule of thumb: **import only the idiomatic package and the runtime.** If you
find yourself importing `bindings/win32`, check whether the idiomatic package
already re-exports what you need (it almost always does).

## Prerequisites every adopter hits

- **Architecture.** Generated code is tagged `windows && (amd64 || arm64)`.
  32-bit (386) is not supported.
- **Privileges.** Many APIs need elevation. Account creation, service control,
  and most `NetworkManagement`/`Security` mutations require an Administrator
  token; the examples detect access-denied and explain rather than crash.
- **Ownership of unmanaged memory.** Some APIs hand back a buffer you must free
  (`NetApiBufferFree`, `LocalFree`, `CoTaskMemFree`). The idiomatic layer
  surfaces these; use `defer` to release them (see `localaccount`).
