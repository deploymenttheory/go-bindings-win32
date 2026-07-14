# go-bindings-win32

[![Go Reference](https://pkg.go.dev/badge/github.com/deploymenttheory/go-bindings-win32.svg)](https://pkg.go.dev/github.com/deploymenttheory/go-bindings-win32)
[![CI](https://github.com/deploymenttheory/go-bindings-win32/actions/workflows/ci.yml/badge.svg)](https://github.com/deploymenttheory/go-bindings-win32/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Idiomatic Go bindings for the **Win32 API**, generated from Microsoft's
[win32metadata](https://github.com/microsoft/win32metadata) — the same
metadata Microsoft's own C#/Rust projections build on. The full surface,
minus only a small tracked set of constructs that can't be represented
faithfully (see [Status](#status--contributing)), as Go you can actually
enjoy calling: Go strings, Go errors, Go slices, and typed COM interfaces.

Today that's **324 packages**: roughly **17,700 functions**, **43,700 COM
methods**, and **16,500 structs**.

```go
//go:build windows

package main

import (
	"log"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

func main() {
	event, err := threading.CreateEvent(nil, true, false, "my-event") // (foundation.HANDLE, error)
	if err != nil {
		log.Fatal(err)
	}
	defer foundation.CloseHANDLE(event) // generated RAII closer

	if err := threading.SetEvent(event); err != nil {
		log.Fatal(err)
	}
}
```

COM interfaces are typed vtable structs — the struct **is** the COM object,
obtained by casting a factory out-param. Here, streaming XML with
`IXmlReader` (a curated API whose `S_FALSE` end-of-input code survives — see
[Errors](#errors)):

```go
var out *win32.IUnknown
if err := xmllite.CreateXmlReader(&xmllite.IID_IXmlReader, &out, nil); err != nil {
	return err
}
reader := (*xmllite.IXmlReader)(unsafe.Pointer(out))
defer reader.Release()
if err := reader.SetInput((*com.IUnknown)(unsafe.Pointer(stream))); err != nil {
	return err
}

var nodeType xmllite.XmlNodeType
for {
	hr, err := reader.Read(&nodeType)
	if err != nil {
		return err // a real failure
	}
	if hr == win32.S_FALSE {
		break // end of input
	}
	// process the node
}
```

## Why

`golang.org/x/sys/windows` is hand-curated and covers a small slice of Win32.
This project generates the **whole** surface from the metadata — kept honest
by a regenerate-and-diff gate, a diagnostics ratchet, and live round-trip
tests, with generated ABI assertions checking the layout of ~14,000 structs —
so the coverage is broad and the mapping is faithful.

## One tree

| Package | Import | What you get |
|---|---|---|
| **Bindings** | `bindings/win32/<namespace>` | The full typed surface — structs, enums, constants, COM interfaces — with idiomatic-shaped calls: Go `string` for `PWSTR`, `bool` for `BOOL`, `error` for `HRESULT`/`SetLastError`, `[]T` for array+count pairs, `[out,retval]` lifted to returns, `Close<Handle>` RAII helpers, and COM interfaces as method-bearing vtable structs. Each function dispatches through `syscall` inline — no wrapper layer. |
| **Runtime** | `bindings/runtime/win32` | Shared helpers: `UTF16Ptr`, `UTF16ToString`, `GUID`, `Bool32`, and the typed `HRESULT` error (`ErrIfFailed`, `S_OK`/`S_FALSE`/`E_*` sentinels, `errors.Is` interop). |

Everything lives in one tree: import `bindings/win32/<namespace>` and the runtime.

## Errors

Win32 reports failure four different ways; each function's Go signature tells
you which domain it uses ([full guide](docs/errors.md)). Failed `HRESULT`s
come back as the typed `win32.HRESULT`, so `errors.Is` works — including
across the `FACILITY_WIN32` bridge:

```go
if errors.Is(err, win32.E_NOINTERFACE) { /* ... */ }
if errors.Is(err, windows.ERROR_ACCESS_DENIED) { /* matches E_ACCESSDENIED too */ }
```

A curated set of APIs whose *success* codes matter (`IEnum*::Next`/`::Skip`,
`IXmlReader::Read`, `CoInitializeEx`) returns `(win32.HRESULT, error)` so
`S_FALSE`-style informational successes are never lost.

## Install

```sh
go get github.com/deploymenttheory/go-bindings-win32@latest
```

Pre-v1: until a tagged release exists, `@latest` resolves a pseudo-version of
`main` — pin the commit you build against.

**Requirements:** Go 1.25+; runs on **Windows amd64 or arm64** (they share
Win32's LLP64 layout). The one external dependency is
`golang.org/x/sys/windows`. The generated files carry
`//go:build windows && (amd64 || arm64)` tags, so you can develop and
**cross-compile from macOS/Linux** (`GOOS=windows go build ./...`) — only
running requires Windows.

## Examples

Runnable programs, each with its own README, under [`examples/`](examples):

- **[`sysinfo`](examples/sysinfo)** — read-only host info (no admin): computer
  name, user, CPU/memory, OS version. Size-probe strings, self-sized structs,
  a C union.
- **[`localaccount`](examples/localaccount)** — the full local user account
  lifecycle (`NetUserAdd`/`GetInfo`/`Enum`/`Del`); mutation gated behind
  `-apply` (needs Administrator), safe dry run by default.

## Documentation

- [Getting started](docs/getting-started.md)
- [Error handling](docs/errors.md) — the four Win32 error domains,
  `errors.Is`, informational successes
- [Strings, structs, and memory](docs/strings-and-memory.md) — UTF-16,
  self-sized structs, buffer ownership, handles
- [Using COM interfaces](docs/com.md) — vtable structs, casting factory
  out-params, lifetime
- [Implementation plan / architecture](docs/IMPLEMENTATION_PLAN.md) — how the
  generator works
- [`CLAUDE.md`](CLAUDE.md) — the as-built generator internals

## How it's built

A native Go reader parses the committed `Windows.Win32.winmd` (ECMA-335, no
Clang, no .NET) into an intermediate model, then a template-based emitter
produces the bindings tree. One command clears and re-emits it:

```sh
go run ./cmd/generate ingest    # winmd → per-namespace IR
go run ./cmd/generate bindings  # IR → bindings/win32 (self-cleaning)
```

Regeneration is byte-deterministic and gated in CI; a diagnostics **ratchet**
fails the build if a change introduces any new degradation beyond the
committed baseline; and a scheduled workflow opens a PR when Microsoft ships
a new winmd. See [`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md)
for the full pipeline.

## Status & contributing

The generator covers the flat Win32 surface and COM interfaces across all
namespaces on amd64/arm64. Constructs that can't be faithfully represented
(e.g. some packed structs, by-value struct/float parameters) are deliberately
skipped rather than emitted wrong; these are tracked in
`metadata/diagnostics-baseline.json`.

Generated code (`bindings/`) is never hand-edited — fix the generator under
`internal/` and regenerate. See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

[MIT](LICENSE).
