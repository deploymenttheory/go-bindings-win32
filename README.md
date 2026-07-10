# go-bindings-win32

Idiomatic Go bindings for the **Win32 API**, generated from Microsoft's
[win32metadata](https://github.com/microsoft/win32metadata). Every function,
struct, enum, constant, and COM interface in the metadata — hundreds of
namespaces — surfaced as Go you can actually enjoy calling: Go strings, Go
errors, Go slices, and typed COM wrappers.

```go
//go:build windows

import (
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/threading"
)

event, err := threading.CreateEvent(nil, true, false, "my-event") // (HANDLE, error)
if err != nil {
	return err
}
defer foundation.CloseHANDLE(event) // generated RAII closer
threading.SetEvent(event)
```

## Why

`golang.org/x/sys/windows` is hand-curated and covers a small slice of Win32.
This project generates the **whole** surface from the same metadata Microsoft's
own C#/Rust projections use — kept honest by a regenerate-and-diff gate and
live ABI/round-trip tests — so the coverage is broad and the mapping is
faithful.

## Two layers

| Layer | Import | What you get |
|---|---|---|
| **Idiomatic** *(use this)* | `opinionated/idiomatic/win32/<namespace>` | Go `string` for `PWSTR`, `bool` for `BOOL`, `error` for `HRESULT`/`SetLastError`, `[]T` for array+count pairs, `[out,retval]` lifted to returns, `Close<Handle>` RAII helpers, COM interfaces as method-bearing wrappers. **Self-contained** — it re-exports every type/constant/pass-through it doesn't improve, so you never import the raw package. |
| **Raw** | `bindings/win32/<namespace>` | The 1:1 `syscall` surface, for the rare un-adapted signature. |
| **Runtime** | `bindings/runtime/win32` | Shared helpers: `UTF16Ptr`, `UTF16ToString`, `GUID`, `HRESULTError`, `Bool32`. |

Rule of thumb: **import only the idiomatic package and the runtime.**

## Install

```sh
go get github.com/deploymenttheory/go-bindings-win32@latest
```

Targets **Windows on amd64 or arm64** (they share Win32's LLP64 layout); the
one external dependency is `golang.org/x/sys/windows`.

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
- [Error handling](docs/errors.md) — the four Win32 error domains
- [Strings, structs, and memory](docs/strings-and-memory.md) — UTF-16,
  self-sized structs, buffer ownership, handles
- [Using COM interfaces](docs/com.md)
- [Implementation plan / architecture](docs/IMPLEMENTATION_PLAN.md) — how the
  generator works
- [`CLAUDE.md`](CLAUDE.md) — the as-built generator internals

## How it's built

A native Go reader parses the committed `Windows.Win32.winmd` (ECMA-335, no
Clang, no .NET) into an intermediate model, then a template-based emitter
produces the raw and idiomatic trees. One command clears and re-emits both:

```sh
go run ./cmd/generate ingest    # winmd → per-namespace IR
go run ./cmd/generate bindings  # IR → raw + idiomatic (both self-cleaning)
```

Regeneration is byte-deterministic and gated in CI, and a scheduled workflow
opens a PR when Microsoft ships a new winmd. See
[`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md) for the full
pipeline.

## Status & contributing

The generator covers the flat Win32 surface, COM interfaces, and the idiomatic
layer across all namespaces on amd64/arm64. Constructs that can't be faithfully
represented (e.g. some packed structs) are deliberately skipped rather than
emitted wrong; these are tracked in `metadata/diagnostics-baseline.json`.

Generated code (`bindings/`, `opinionated/`) is never hand-edited — fix the
generator under `internal/` and regenerate. See [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

[MIT](LICENSE).
