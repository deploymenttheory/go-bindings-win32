# Getting started

Call the Win32 API from Go with generated bindings — Go strings, Go errors,
Go slices, and typed COM interfaces, across every namespace in Microsoft's
[win32metadata](https://github.com/microsoft/win32metadata).

## Install

```sh
go get github.com/deploymenttheory/go-bindings-win32@latest
```

The bindings target **Windows on amd64 or arm64** (they share the same 64-bit
LLP64 layout). Every generated file carries `//go:build windows && (amd64 ||
arm64)`, so a non-Windows or 32-bit build simply skips them. The only external
dependency is `golang.org/x/sys/windows`.

## Your first call

Pick the namespace you need under `bindings/win32/…` and call it.
Namespaces map from win32metadata: `Windows.Win32.System.Threading` →
`bindings/win32/system/threading`.

```go
//go:build windows

package main

import (
	"fmt"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

func main() {
	// Go string name, Go bool flags, (HANDLE, error) return.
	event, err := threading.CreateEvent(nil, true /*manualReset*/, false /*initial*/, "my-event")
	if err != nil {
		panic(err)
	}
	defer foundation.CloseHANDLE(event) // generated RAII closer for HANDLE

	if err := threading.SetEvent(event); err != nil {
		panic(err)
	}
	status, _ := threading.WaitForSingleObject(event, 0)
	fmt.Printf("wait status: %#x\n", status) // 0 = WAIT_OBJECT_0
}
```

Run it: `go run .` — see [`examples/`](../examples) for complete, runnable
programs.

## What to import

| Package | Import path | What it gives you |
|---|---|---|
| **Namespace bindings** | `bindings/win32/<namespace>` | The generated API for a namespace: Go strings for `PWSTR`, `bool` for `BOOL`, `error` for `HRESULT`/`SetLastError`, `[]T` for array+count pairs, elevated `[out,retval]` returns, `Close<Handle>` helpers, COM interfaces as method-bearing vtable structs — plus every struct, constant, and function. One Go package per namespace. |
| **Runtime** | `bindings/runtime/win32` | Shared helpers the bindings use: `UTF16Ptr`, `UTF16ToString`, `GUID`, `HRESULTError`, `Bool32`. |

Rule of thumb: **import `bindings/win32/<namespace>` and the runtime
`bindings/runtime/win32`.** There is a single bindings tree — the Go-friendly
shaping is built in, so there is no separate package to reach for.

## Next

- [Error handling](errors.md) — the four Win32 error domains and how the
  bindings surface each.
- [Strings, structs, and memory](strings-and-memory.md) — UTF-16, self-sized
  structs, buffer ownership, and handles.
- [Using COM interfaces](com.md) — method-bearing structs, `QueryInterface`,
  and lifetimes.
- [Examples](../examples) — runnable programs with their own READMEs.
