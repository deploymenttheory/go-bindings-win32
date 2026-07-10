# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Build everything
go build ./...

# Unit tests (winmd reader + ingest; needs the committed winmd)
go test ./internal/...

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

# 2. Emit raw Go bindings (all 324 namespaces → bindings/win32/)
go run ./cmd/generate/ bindings

# Emit one namespace plus the transitive closure of namespaces it references
go run ./cmd/generate/ bindings --namespace System.Threading

# Verbose diagnostics (degradations, skips, cycle breaks)
go run ./cmd/generate/ bindings -v

# Emit the idiomatic wrapper tier (opinionated/idiomatic/win32/) over the raw bindings
go run ./cmd/generate/ idiomatic

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
  attributes (`attrs.go`). No .NET, no cgo. The whole winmd (37k types, 318k
  signatures, 152k attributes) decodes with zero failures; tests brute-force
  all of it.
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
- **`internal/codegen/emit/raw/`** (pkg `rawwin`) — the gather → view → render
  compiler:
  - gather (`types.go`, `functions.go`, `sizes.go`, `generator.go`): all
    decisions; unions become size/alignment-correct opaque blobs, C layout is
    computed in `sizes.go` (amd64 model).
  - `view/` — pure-data IR; imports nothing from meta/typemap.
  - `render/` — `//go:embed templates/*.tmpl`; imports only `view` (the
    render firewall). Templates branch on `ReturnKind`, never decide.
- **`internal/codegen/shared/fileasm/`** — DO-NOT-EDIT header + `//go:build
  windows` + grouped imports + `go/format`. All generated files funnel
  through it.
- **`bindings/runtime/win32/`** — the hand-written runtime: lazy system-DLL
  loading, `LastError` normalization, `GUID`, UTF-16 helpers. Single external
  dependency: `golang.org/x/sys/windows`.

### Generated output (`bindings/win32/`)

One package per namespace, directory = namespace path
(`System.Threading` → `bindings/win32/system/threading`, import alias for
cross-refs = all segments joined: `systemthreading`). Files per package:
`doc.go`, `<pkg>_types.go`, `<pkg>_constants.go`, `<pkg>_functions.go`.

Function shapes (view.ReturnKind):
- void → no return
- `BOOL` + SetLastError → `error` only
- handle/pointer + SetLastError → `(T, error)`, failure sentinels from
  `[InvalidHandleValue]` metadata
- other + SetLastError → `(T, error)` where err is the advisory GetLastError
- no SetLastError → bare `T`

### Idiomatic tier (`opinionated/idiomatic/win32/`, pkg per namespace)

`internal/codegen/emit/idiomatic/` (pkg `idiowin`) is a second view→render
leaf that wraps the raw tier — hermetic (only calls the raw packages).
`generate idiomatic` first runs the raw emitter to learn the emitted-function
set and share the mapper's exact degradation decisions (so a wrapper's
resolved types always match the raw function it calls), then emits one
ergonomic wrapper per improvable function:

- input `PWSTR`/`PCWSTR` → Go `string` (UTF-16 at the boundary)
- input `BOOL` → Go `bool`; plain `BOOL` return → `bool`
- `HRESULT` return (no SetLastError) → `error`
- `[Reserved]` params elided (passed as the raw type's zero)
- `-W` functions de-suffixed when the bare name is free

Functions with nothing to improve are skipped (no pointless alias). Types
resolve with `Context.QualifyOwn = true` so even same-namespace raw types are
package-qualified. Selection mirrors the raw tier exactly (metadata order,
first amd64 entry per name) so signatures always align. COM ergonomics are a
later increment; v1 covers free functions (~8,200 wrappers, 208 packages).

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
  tests, then the regeneration gate: ingest → validate → abitest → idiomatic
  → ratchet →
  `git diff --exit-code` over `bindings/` + `acceptance/`.
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
  generated, and raw `HRESULT` returns convert via `win32.Succeeded`/
  `win32.HRESULTError`. Severed base embeddings (import cycles) demote to a
  rootless vtable with a doc note; slots stay correct.
- **Skipped constructs are tracked**: functions with by-value struct/float
  params, packed structs, struct-initializer constants → diagnostics, never
  broken output. A pre-pass (`computeSkippedTypes`) guarantees no reference
  to a skipped type is ever emitted.
- **Single external dependency**: `golang.org/x/sys/windows`. Do not add more.
- `metadata/win32/` (the IR cache) is derived and gitignored; the committed
  source of truth is `metadata/winmd/Windows.Win32.winmd` + `PROVENANCE.json`.
  Bump `win32meta.CurrentSchemaVersion` on incompatible IR changes.
- After changing the generator, re-run `ingest` (if the IR changed) and
  `bindings`, and include the regenerated `bindings/win32/` in the same PR.
