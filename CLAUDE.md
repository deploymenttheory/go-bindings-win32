# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Build everything
go build ./...

# Unit tests (winmd reader + ingest; needs the committed winmd)
go test ./internal/...

# Runtime unit tests (loader security, HRESULT error semantics)
go test ./bindings/runtime/...

# Live acceptance tests (calls real Win32 APIs; Windows only)
go test ./acceptance/

# ── Pipeline ─────────────────────────────────────────────────────────────────
# 0. Update the committed winmd from NuGet (writes PROVENANCE.json; no-op when current)
go run ./cmd/generate/ fetch-metadata
go run ./cmd/generate/ fetch-metadata --version 71.0.14-preview

# 1. Project the committed winmd into per-namespace IR (metadata/win32/*.w32meta.json)
go run ./cmd/generate/ ingest

# Ingest a subset
go run ./cmd/generate/ ingest --namespace System.Threading,Foundation

# 2. Emit the single idiomatic-shaped bindings tree (bindings/win32/),
#    self-cleaning (all 324 namespaces).
go run ./cmd/generate/ bindings

# One namespace plus the transitive closure it references
go run ./cmd/generate/ bindings --namespace System.Threading

# Verbose diagnostics (degradations, skips, cycle breaks)
go run ./cmd/generate/ bindings -v

# Regenerate bindings AND the ABI layout acceptance test (acceptance/abi_generated_test.go)
go run ./cmd/generate/ abitest

# Structural integrity checks (errors fail; warnings report)
go run ./cmd/generate/ validate

# Semantic API diff between two metadata trees (markdown; --json for machine output)
go run ./cmd/generate/ diff --old /tmp/old-metadata --new metadata/win32

# Diagnostics ratchet: fail on NEW degradations beyond the committed baseline (CI)
go run ./cmd/generate/ bindings --diagnostics-baseline metadata/diagnostics-baseline.json

# Rewrite the baseline after a reviewed change
go run ./cmd/generate/ bindings --diagnostics metadata/diagnostics-baseline.json

# List namespaces with construct counts
go run ./cmd/generate/ list

# Inspect ingested IR
go run ./cmd/inspect/ metadata/win32/System.Threading.w32meta.json
go run ./cmd/inspect/ --name CreateEventW metadata/win32/System.Threading.w32meta.json
```

## Architecture

This is a **code generator** that reads Microsoft's `Windows.Win32.winmd`
(ECMA-335 metadata, committed at `metadata/winmd/`, from the
`Microsoft.Windows.SDK.Win32Metadata` NuGet package) and emits Go bindings for
the Win32 API. It mirrors the architecture of the sibling
`go-bindings-macosplatform` generator; the design rationale and milestones live
in [`docs/IMPLEMENTATION_PLAN.md`](docs/IMPLEMENTATION_PLAN.md).

```
Windows.Win32.winmd → .w32meta.json (IR) → Go source
  (internal/winmd)     (ingest → Registry)  (emit: gather → view → render)
```

### Pipeline packages

- **`internal/winmd/`** — native Go ECMA-335 reader: PE container → metadata
  streams → tables (`tables.go`) → signature blobs (`sig.go`) → custom
  attributes (`attrs.go`). No .NET, no cgo. Aligned with the ECMA-335 6th
  edition: exported symbols carry §II.x section references, table IDs are a
  typed `Table` enum (all 45 tables, spec names), and bitmask columns are
  typed (`TypeAttributes`, `ParamAttributes`, `PInvokeAttributes`, … in
  `flags.go`). Untrusted lengths/rows are bounds-checked and
  allocation-clamped (`corrupt_test.go` covers hostile inputs). The package
  doc records explicit non-goals (no lazy tables, no CodedIndex tag
  generics, no microsoft/go-winmd dependency). The whole winmd (37k types,
  318k signatures, 152k attributes) decodes with zero failures; tests
  brute-force all of it.
- **`internal/win32meta/`** — the IR (`model.go`): one `NamespaceMeta` per
  namespace with structs/enums/functions/constants/interfaces/delegates/
  typedefs. `TypeRef` is the recursive type grammar (Native / ApiRef /
  PointerTo / Array), shaped to match win32json's vocabulary so that project
  can serve as a test oracle. `SchemaVersion` gates stale caches.
- **`internal/win32meta/ingest/`** — winmd → IR projector. Namespace ownership
  is authoritative (no scoring heuristic needed, unlike macOS). Reads the
  attribute contract from win32metadata's `docs/projections.md`: DllImport via
  the ImplMap table, `[SupportedArchitecture]`, `[Unicode]`/`[Ansi]`,
  `[NativeTypedef]`/`[RAIIFree]`/`[InvalidHandleValue]`, `[NativeArrayInfo]`,
  `[MemorySize]`, `[NativeBitfield]`, `[FlexibleArray]`, `[Documentation]`, …
- **`internal/codegen/pipeline/`** — `LoadAll` → `Registry` (`*Index` maps) +
  `ComputeBlockedImports` (namespace import-cycle detection; severed edges
  degrade to raw types instead of importing — the Win32 namespace graph IS
  cyclic).
- **`internal/codegen/typemap/`** — `Mapper.GoType(TypeRef, Context,
  ImportSet)`: the only place type decisions live. Cross-namespace refs
  qualify + record imports as a side effect; every degradation lands in
  `Diagnostics`. `Kind` classifies results for marshaling (`ArgClassOf`).
- **`internal/codegen/emit/raw/`** (pkg `rawwin`) — the single emitter, a
  gather → view → render compiler. (Still named `raw` for historical reasons;
  it now emits the one idiomatic-shaped tree. Renaming it is a cosmetic
  follow-up.)
  - gather (`types.go`, `functions.go`, `interfaces.go`, `handles.go`,
    `sizes.go`, `generator.go`): all decisions; the function/COM gathers apply
    the idiomatic shaping (see below), unions become size/alignment-correct
    opaque blobs, C layout is computed in `sizes.go` (amd64 model).
  - `view/` — pure-data IR; imports nothing from meta/typemap.
  - `render/` — `//go:embed templates/*.tmpl`; imports only `view` (the
    render firewall). Templates branch on `ReturnKind`, never decide.
- **`internal/codegen/shared/fileasm/`** — DO-NOT-EDIT header + `//go:build
  windows` + grouped imports + `go/format`. All generated files funnel
  through it.
- **`bindings/runtime/win32/`** — the hand-written runtime: lazy system-DLL
  loading (System32-only via `LoadLibraryExW` +
  `LOAD_LIBRARY_SEARCH_SYSTEM32`, `loader.go`), `LastError` normalization,
  the typed `HRESULT` error, `GUID`, UTF-16 helpers. Standard library only —
  the module has zero external dependencies.

### Generated output (`bindings/win32/`)

One package per namespace, directory = namespace path
(`System.Threading` → `bindings/win32/system/threading`, import alias for
cross-refs = all segments joined: `systemthreading`). Files per package, split
by construct: `doc.go`, `<pkg>_typedefs.go`, `<pkg>_enums.go`,
`<pkg>_structs.go`, `<pkg>_delegates.go`, `<pkg>_constants.go`,
`<pkg>_functions.go`, `<pkg>_interfaces.go` (COM), `<pkg>_handles.go` (RAII
closers). Empty files are not written.

There is **one** tree. The typed constructs (typedefs/enums/structs/delegates/
constants/interfaces) are the full faithful definitions; the functions and COM
methods carry an idiomatic look-and-feel while dispatching through `syscall`
**inline** (no wrapper calling a second function). The former two-tier split
(a separate `opinionated/idiomatic/win32` layer wrapping a 1:1 raw tier) has
been dissolved into this single emitter.

### Idiomatic shaping (functions and COM methods)

The function and COM-method gathers (`functions.go`, `interfaces.go`) shape
each call, then the template dispatches via `syscall.SyscallN`:

- input `PWSTR`/`PCWSTR` → Go `string` (UTF-16 at the boundary:
  `_name := win32.UTF16Ptr(name)`)
- input `BOOL` → Go `bool` (`win32.Bool32`); plain `BOOL` return → `bool`
- `HRESULT` return → `error` (failures surface as the typed `win32.HRESULT`,
  which `errors.Is`-matches `syscall.Errno` for `FACILITY_WIN32` codes);
  `BOOL` + SetLastError → `error`
- a curated set of informational-success APIs (`IEnum*::Next`/`::Skip`,
  `IXmlReader::Read`, `CoInitializeEx` — see `emit/raw/informational.go`)
  returns `(win32.HRESULT, error)` instead: err reflects failure only, the
  HRESULT preserves `S_FALSE`-style success codes. The winmd has no attribute
  for this, so the set is curated; stale entries surface as diagnostics.
- handle/pointer + SetLastError → `(T, error)`, failure sentinels from
  `[InvalidHandleValue]` metadata; other + SetLastError → `(T, error)` where
  err is the advisory GetLastError; no SetLastError → bare `T`
- `[Reserved]` params elided from the signature (passed as `0`)
- `-W` functions de-suffixed when the bare name is free (`CreateEventW` →
  `CreateEvent`); `-A` variants and unsuffixed names keep their name
- an array-pointer param + its input count param (`[NativeArrayInfo]`
  `CountParamIndex`) collapse into a single `[]T` (count derived from `len`
  at the call site) — flat functions and COM methods alike. Applies only to
  typed-pointer arrays with a unique, input-only integer count; shared/out
  counts stay raw.
- a `void*` or `byte*` param + its input byte-size param (`[MemorySize]`
  `BytesParamIndex`) collapse into a single `[]byte` (size derived from `len`
  at the call site) — flat functions and COM methods alike. Requires a unique,
  input-only integer size not referenced by any `[NativeArrayInfo]` count (a
  shared size stays raw rather than un-collapsing a typed array). Typed
  non-byte pointers with `[MemorySize]` keep their type.
- an `[out,retval]` param is elevated out of the signature into a Go return
  value (`Get_X() (T, error)`). For flat functions only when the return is a
  clean status (HRESULT / BOOL+SetLastError / void); for COM methods whenever
  the return is HRESULT.
- a `void**` `[out]` param carrying `[ComOutPtr]` (or `[IidParameterIndex]`,
  ingested but absent from the current winmd) is typed `**win32.IUnknown` —
  the runtime's root COM shape (`bindings/runtime/win32/iunknown.go`), layout-
  compatible with every generated `*IFoo` and carrying QueryInterface/AddRef/
  Release. Cast to the concrete interface selected by the riid argument. A
  `[retval]` one elevates to a `(*win32.IUnknown, error)` return like any
  typed COM out. An un-attributed `void**` `[out]` that pairs with an input
  `*GUID` param whose name contains `iid` — immediately preceding it, or the
  signature's unique riid/`void**` pair — is retyped the same way (the
  windows-rs shape heuristic); other un-attributed `void**` outs stay
  `*unsafe.Pointer`.
- a parameter whose name would shadow a type used in the signature (e.g. a
  param `Node` in a method returning `(*Node, error)`) is suffixed with `_`,
  because Go puts parameter names in scope for the result types.

The set of emittable functions is exactly what `syscall.SyscallN` can marshal:
by-value struct/union/array/GUID params and floats are skipped with a
diagnostic. The merged `view.ReturnKind` enumerates all the shapes above.

**Handle RAII** (`<pkg>_handles.go`): each `[RAIIFree]` handle typedef gets a
`Close<Handle>(h) error` helper that calls the (idiomatic-shaped) free function
— looked up via the registry's function-owner index, possibly cross-namespace —
and normalizes its return to `error`. Emitted only when the closer is
unambiguous, takes exactly the handle, and has a normalizable return; otherwise
skipped with a diagnostic (~107 closers).

**COM interfaces** (`<pkg>_interfaces.go`): the generated struct **is** the COM
object — obtain one by casting a factory out-param / `QueryInterface` pointer
(`var s *com.IStream; structuredstorage.CreateStreamOnHGlobal(0, true, &s)`).
Roots carry `LpVtbl *[1024]uintptr`; derived interfaces embed their base,
promoting its methods. Methods dispatch through the absolute vtable slot
(`syscall.SyscallN(self.LpVtbl[slot], uintptr(unsafe.Pointer(self)), args…)`),
converting `HRESULT` → `error` and lifting `[out,retval]` COM outs to typed
`*IFoo` return values. `IID_*` GUID vars are generated. There is **no**
`WrapIFoo` constructor and **no** `.Raw` field. Severed base embeddings (import
cycles) demote to a rootless vtable with a doc note; slots stay correct.

**One command.** `generate bindings` clears and re-emits `bindings/win32/` in a
single self-cleaning run. Selection is metadata order, first amd64 entry per
name; names are claimed types-first, then functions (desuffix-when-free), then
closers, so nothing shadows.

### Architecture support

Generated files carry `//go:build windows && (amd64 || arm64)`
(`fileasm.GeneratedBuildTag`). Win32 amd64 and arm64 share the LLP64 layout,
so one binding set is correct on both; 386 (32-bit pointers) is deliberately
excluded rather than silently miscompiled. CI cross-compiles for arm64.

### Name rules (`internal/codegen/naming/`)

- Everything package-level is exported via `naming.Export` (`select` →
  `Select`, `_11` → `F11`).
- Types pre-claim their names per package before values (enum members,
  constants, functions); losers are skipped with a diagnostic.
- Param names escape Go keywords/predeclared/generated locals (`err`, `ret`).

### Determinism gate

Regeneration must be byte-identical: run `generate bindings` twice and
`git diff` must be empty. All maps are sorted before iteration. Import-usage
pruning scans code with comments stripped. The generator is self-cleaning:
generated files not rewritten by the current run (renamed constructs, removed
namespaces) are pruned, matching the macOS generator; files without the
DO-NOT-EDIT header are never touched.

### QA gates (M2)

- **ABI layout test** — `generate abitest` records every emitted struct's
  computed C layout and writes `acceptance/abi_generated_test.go` with
  `unsafe.Sizeof`/`Offsetof` assertions (all Foundation + ~400 sampled).
  Structs whose packed C layout Go cannot reproduce are skipped up front.
- **`generate validate`** — dangling refs, invalid enum bases, missing DLLs,
  dangling COM bases. Errors fail CI.
- **`generate diff`** — reviewable markdown/JSON API diff for winmd bumps.
- **Diagnostics ratchet** — `metadata/diagnostics-baseline.json` is the
  committed degradation set; CI fails on any new entry.

### CI (GitHub Actions)

- `ci.yml` — build, vet (non-generated packages only: generated syscall
  wrappers trip vet's unsafe.Pointer heuristic by design), unit + acceptance
  tests, then the regeneration gate: ingest → validate → bindings (with
  ratchet) → abitest → `git diff --exit-code` over `bindings/` + `acceptance/`.
- `winmd-update.yml` — weekly + manual: `fetch-metadata` checks NuGet; on a
  new version it re-ingests, regenerates, rewrites the baseline, and opens a
  PR whose body is the `generate diff` output old→new.

## Important constraints

- **amd64-only for now**: arch-specific structs/functions emit the amd64
  variant (`chooseArchVariant`); arm64/x86 build-tag emission is a later
  milestone (M5).
- **COM interfaces are emitted** (`<pkg>_interfaces.go`): one Go struct per
  interface (roots carry `LpVtbl *[1024]uintptr`; derived interfaces embed
  their base, promoting its methods), methods dispatch through absolute
  vtable slots computed from the metadata base chain, `IID_*` GUID vars are
  generated, and `HRESULT` returns convert to `error` via `win32.ErrIfFailed`.
  The struct IS the COM object (no `Wrap`/`.Raw`). Severed base embeddings
  (import cycles) demote to a rootless vtable with a doc note; slots stay
  correct.
- **Skipped constructs are tracked**: functions with by-value struct/float
  params, packed structs, struct-initializer constants → diagnostics, never
  broken output. A pre-pass (`computeSkippedTypes`) guarantees no reference
  to a skipped type is ever emitted.
- **Zero external dependencies**: the module is standard-library-only
  (`go.mod` has no require lines). Do not add any. Consumers may still use
  `golang.org/x/sys/windows` constants in `errors.Is` checks — its `ERROR_*`
  values are typed `syscall.Errno`, which the runtime matches.
- `metadata/win32/` (the IR cache) is derived and gitignored; the committed
  source of truth is `metadata/winmd/Windows.Win32.winmd` + `PROVENANCE.json`.
  Bump `win32meta.CurrentSchemaVersion` on incompatible IR changes.
- After changing the generator, re-run `ingest` (if the IR changed) and
  `bindings`, and include the regenerated `bindings/win32/` in the same PR.
