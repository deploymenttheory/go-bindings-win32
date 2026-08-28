# go-bindings-win32

[![GoDoc](https://pkg.go.dev/badge/github.com/deploymenttheory/go-bindings-win32)](https://pkg.go.dev/github.com/deploymenttheory/go-bindings-win32)
[![License](https://img.shields.io/github/license/deploymenttheory/go-bindings-win32)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/deploymenttheory/go-bindings-win32)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/deploymenttheory/go-bindings-win32)](https://github.com/deploymenttheory/go-bindings-win32/releases)
[![codecov](https://codecov.io/gh/deploymenttheory/go-bindings-win32/graph/badge.svg)](https://codecov.io/gh/deploymenttheory/go-bindings-win32)
![Status: pre-v1](https://img.shields.io/badge/status-pre--v1-orange)

Idiomatic Go bindings for the **Win32 API**, generated from Microsoft's
[win32metadata](https://github.com/microsoft/win32metadata) — the same
metadata Microsoft's own C#/Rust projections build on. The full surface —
every function and 99.9% of COM methods; the exact numbers, and what the
remaining diagnostics mean, are in [`docs/COVERAGE.md`](docs/COVERAGE.md) —
as Go you can actually enjoy calling: Go strings, Go errors, Go slices, and
typed COM interfaces.

Today that's **324 packages**: **18,278 functions**, **46,326 COM methods**,
and **15,315 structs** (plus their nested types).

```go
//go:build windows

package main

import (
	"log"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

func main() {
	event, err := threading.CreateEvent(nil, true, false, nil) // unnamed (NULL); win32.Str("name") names it
	if err != nil {
		log.Fatal(err)
	}
	defer foundation.CloseHANDLE(event) // generated Close<Handle> helper

	if err := threading.SetEvent(event); err != nil {
		log.Fatal(err)
	}
}
```

COM interfaces are typed vtable structs — the struct **is** the COM object.
Typed factories hand you a `*com.IStream` directly; riid-selected factories
hand you the root `*win32.IUnknown`, which `win32.Cast[T]` reinterprets as
the interface the riid picked. Here, streaming XML with `IXmlReader` (a
curated API whose `S_FALSE` end-of-input code survives — see
[Errors](#errors)):

```go
import (
	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/data/xml/xmllite"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
)

func countNodes(document []byte) (int, error) {
	var stream *com.IStream // a typed factory out-param: no cast
	if err := structuredstorage.CreateStreamOnHGlobal(0, true, &stream); err != nil {
		return 0, err
	}
	defer stream.Release()
	var written uint32
	if err := stream.Write(document, &written); err != nil { // void*+size → []byte
		return 0, err
	}
	var position uint64
	if err := stream.Seek(0, com.STREAM_SEEK_SET, &position); err != nil {
		return 0, err
	}

	var out *win32.IUnknown // a riid-selected factory out-param
	if err := xmllite.CreateXmlReader(&xmllite.IID_IXmlReader, &out, nil); err != nil {
		return 0, err
	}
	reader := win32.Cast[xmllite.IXmlReader](out) // the interface the riid selected
	defer reader.Release()
	if err := reader.SetInput(&stream.IUnknown); err != nil { // upcast: the embedded root
		return 0, err
	}

	nodes := 0
	var nodeType xmllite.XmlNodeType
	for {
		hr, err := reader.Read(&nodeType)
		if err != nil {
			return nodes, err // a real failure
		}
		if hr == win32.S_FALSE {
			return nodes, nil // end of input
		}
		nodes++
	}
}
```

## Why

`golang.org/x/sys/windows` is hand-curated and covers a small slice of Win32.
This project generates the **whole** surface from the metadata — kept honest
by a regenerate-and-diff gate, a diagnostics ratchet, and live round-trip
tests, with generated ABI assertions checking the size and every field offset of
all ~13,900 emitted structs —
so the coverage is broad and the mapping is faithful.

## One tree

| Package | Import | What you get |
|---|---|---|
| **Bindings** | `bindings/win32/<namespace>` | The full typed surface — structs, enums, constants, COM interfaces — with idiomatic-shaped calls: Go `string` for `PWSTR`, `bool` for `BOOL`, `error` for `HRESULT`/`SetLastError`, `[]T` for array+count pairs, `[out,retval]` lifted to returns, `Close<Handle>` helpers (from `[RAIIFree]`), and COM interfaces as method-bearing vtable structs. The shaping is inlined into each function: nothing sits between you and `syscall.SyscallN` (or `win32.Call`, the runtime's register-aware dispatch for floats and by-value structs). |
| **Runtime** | `bindings/runtime/win32` | Shared helpers: `UTF16Ptr`, `UTF16ToString`, `GUID`, `Bool32`, the typed `HRESULT` error (`ErrIfFailed`, `S_OK`/`S_FALSE`/`E_*` sentinels, `errors.Is` interop), and the System32-only loader (`ProcError`, `ErrProcNotFound`/`ErrDLLNotFound`). |

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

An API that is absent on the running Windows build (a newer export, or an
optional component's DLL) cannot be dispatched to and **panics** with a
`*win32.ProcError`; probe first with `pkg.Procs.<Function>.Find()` — see
[Unavailable APIs](docs/errors.md#unavailable-apis-missing-exports).

## Install

```sh
go get github.com/deploymenttheory/go-bindings-win32@latest
```

Pre-v1: tagged releases are published, so `@latest` resolves the newest tag.
The pre-v1 API is still evolving — pin the version you build against and
review the changelog before upgrading.

**Requirements:** Go 1.25+; runs on **Windows amd64 or arm64** (they share
Win32's LLP64 layout). The only dependency is our own
[go-winmd](https://github.com/deploymenttheory/go-winmd) metadata reader
(used by the generator); the runtime and generated bindings link **nothing
beyond the Go standard library**. The 23 MB `Windows.Win32.winmd` lives in a
nested, Go-code-free module (`metadata/`), so it is not part of what
`go get` downloads. The generated files and the runtime carry `windows`
build tags, so you can develop and **cross-compile from macOS/Linux**
(`GOOS=windows go build ./...`) — only running requires Windows.

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
- [How calls reach Win32](docs/calling-convention.md) — the `SyscallN` path,
  the register-aware `win32.Call` path and its assembly, the ABI rules
- [Coverage](docs/COVERAGE.md) — what is emitted, what is not, and why
  (regenerated with the tree)
- [`CLAUDE.md`](CLAUDE.md) — the as-built generator internals and pipeline

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
a new winmd. See [`CLAUDE.md`](CLAUDE.md) for the full pipeline.

## Known limitations

- **Callbacks with floats.** Delegates are `uintptr` function pointers you
  create with `syscall.NewCallback`, which cannot marshal floating-point
  parameters or results ([golang/go#45300](https://github.com/golang/go/issues/45300))
  and needs exactly one `uintptr`-sized result; the four such callbacks are
  flagged in their doc comments. Every *call* shape — floats, by-value
  structs, struct returns — is covered by the runtime's assembly-backed
  `win32.Call` on both amd64 and arm64, without cgo.
- **Variadic functions.** The metadata declares the `wsprintf` family with
  their fixed parameters only (all integers/pointers), so the bindings call
  them correctly on both architectures but cannot pass a variadic tail.
- **386 is excluded.** Structs whose amd64 and arm64 layouts differ
  (`CONTEXT`, `SLIST_HEADER`, the unwind/minidump thread records and the
  structs embedding them) are emitted per architecture under `amd64` /
  `arm64` build tags, with per-architecture ABI assertions; structs whose
  only other layout is x86 emit the shared 64-bit one.
- **Absent APIs panic.** An export missing from the running Windows build
  (or a DLL that is not installed) has no address to call; the binding panics
  with a `*win32.ProcError`. Probe with `pkg.Procs.<Function>.Find()` —
  see [docs/errors.md](docs/errors.md#unavailable-apis-missing-exports).
- **Header inlines.** The three `FORCEINLINE` pseudo-exports
  (`GetCurrentProcessToken` and friends) are emitted from their `[Constant]`
  metadata values, not dispatched.
- **arm64 runs in CI** as a cross-compile plus a Windows-on-ARM job that
  executes the runtime, acceptance and per-architecture ABI suites.

## Status & contributing

The generator covers the flat Win32 surface and COM interfaces across all
namespaces on amd64/arm64. Every construct is emitted unless a diagnostic
says why not — 567 of them today, most informational (`-W` names kept,
architecture-specific layouts); the skips are 44 handle closers whose free
function is ambiguous, 29 name collisions, 15 structs without an amd64
layout and 3 struct-initializer constants. The full breakdown is in
[`docs/COVERAGE.md`](docs/COVERAGE.md) and the set is ratcheted in
`metadata/diagnostics-baseline.json`.

Generated code (`bindings/`) is never hand-edited — fix the generator under
`internal/` and regenerate. See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

[MIT](LICENSE).
