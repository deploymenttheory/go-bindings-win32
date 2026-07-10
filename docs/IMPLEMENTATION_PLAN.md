# go-bindings-win32 — Implementation Plan

> **Status (2026-07-10):** M0, M1, and **M2 are implemented and passing**. The
> raw tier emits **all 324 namespaces** (17,713 functions, zero compile
> errors), live acceptance tests exercise Threading end-to-end, and the M2
> gates are in place: generated ABI layout test (13k structs recorded,
> Sizeof/Offsetof assertions), packed-struct representability skip,
> struct-initializer constants (DEVPROPKEY/PROPERTYKEY/GUID), `validate`,
> `diff`, the diagnostics ratchet (`metadata/diagnostics-baseline.json`),
> self-cleaning re-emittance (stale generated files pruned, matching the
> macOS generator), and `fetch-metadata` + GitHub Actions (`ci.yml`
> regen/ratchet gate; `winmd-update.yml` scheduled NuGet watcher that opens a
> diff-annotated PR on new winmd releases).
> Remaining: M3 COM vtables, M4 idiomatic tier, M5 arch build tags
> (amd64-only today). See CLAUDE.md for the as-built architecture.

Generate idiomatic Go bindings for the entire Win32 API from Microsoft's
`win32metadata`, mirroring the architecture of the sibling
`go-bindings-macosplatform` generator.

---

## 0. The core idea

The macOS generator is a three-phase compiler:

```
Clang AST dump  →  .gometa.json (metadata IR)  →  Go source
   (scan)              (load → Registry)            (emit: gather→view→render)
```

For Windows we **replace only the front of the pipeline**. Microsoft already ran
Clang for us: `win32metadata` scrapes the Windows SDK headers and produces an
ECMA-335 `Windows.Win32.winmd` (shipped in the `Microsoft.Windows.SDK.Win32Metadata`
NuGet package). We **read that `.winmd` directly in Go** — a native ECMA-335
metadata reader — and project it into our IR. No Clang, no headers, no .NET
runtime (a `.nupkg` is just a `.zip`; the winmd is a PE file with CLI metadata
tables we parse ourselves).

```
Windows.Win32.winmd  →  .w32meta.json (metadata IR)  →  Go source
   (read: native ECMA-335 reader)  (load → Registry)     (emit: gather→view→render)
```

> **DECISION (locked):** the input is the **official binary `.winmd` parsed by a
> native Go ECMA-335 reader**, not the `marlersoft/win32json` JSON projection.
> This is more up-front work than JSON ingestion but stays on the always-current
> official artifact with no community-mirror lag. Model the reader on
> `ynkdir/py-win32more` (reads the winmd with no .NET) and the `windows-rs`
> `windows-metadata` crate (a from-scratch reader). `win32json`'s documented JSON
> schema remains a useful *cross-check oracle* for the reader's output.

**Everything downstream of the IR is reusable in shape:** the Registry, the
`gather → pure-data view → template-only render` firewall, the `ImportSet`
side-effect discipline, the diagnostics ratchet, and the regenerate-and-`git diff`
determinism gate. What changes is the **front-end reader** (native ECMA-335 winmd
parser instead of the Clang scanner), the **typemap** (Win32/COM/UTF-16/HRESULT
instead of ObjC), and the **runtime** (`syscall` DLL + COM vtable instead of purego).

### Locked decisions

| Decision | Choice |
|----------|--------|
| **Input source** | Native Go ECMA-335 `.winmd` reader against the official `Microsoft.Windows.SDK.Win32Metadata` NuGet winmd. `win32json` = cross-check oracle only. |
| **Raw-tier errors** | Go `error` — `(T, error)`; HRESULT/`SetLastError`/NTSTATUS wrapped in structured, per-domain error types. |
| **COM** | **In v1** — the COM vtable pipeline (interfaces, `IUnknown`/`IDispatch` embedding, sealed providers) ships in the first release, not deferred. |
| **Runtime dependency** | `golang.org/x/sys/windows` — the single external dep (mirrors the macOS repo's single `purego` dep). |
| **Module path** | `github.com/deploymenttheory/go-bindings-win32`. |

**Prior art we lean on:**
- `zzl/go-win32api` — closest existing Go generator from this metadata (raw tier:
  type aliases `HANDLE=uintptr`, `syscall.SyscallN`, union accessor methods, typed
  `WIN32_ERROR`). x64-only, pure `syscall`, no cgo.
- `winlabs/gowin32` — hand-written idiomatic model (raw `wrappers/` split + a
  hand-quality ergonomic tier: UTF-16 conversion, `defer` handle cleanup,
  per-domain structured errors).
- `ynkdir/py-win32more` — reads the `.winmd` with **no .NET**; the reference model
  for our native Go ECMA-335 reader. `windows-rs`'s `windows-metadata` crate is a
  second from-scratch-reader reference.

---

## 1. Current state & bootstrap

- **Target repo** (`go-bindings-win32`) is currently a bare template: no `go.mod`,
  no Go code. Green field.
- **Host**: Go 1.26.2, Windows SDK 10.0.19041 present, **no .NET SDK** — and none
  needed: the native Go winmd reader parses the PE + CLI metadata tables with only
  the stdlib (`debug/pe`, `encoding/binary`).

**Bootstrap tasks**
1. `go mod init github.com/deploymenttheory/go-bindings-win32` (match the sibling
   module-naming convention).
2. Add `golang.org/x/sys/windows` as the **only** external runtime dependency
   (the Windows analogue of the macOS repo's single `ebitengine/purego` dep). It
   gives us `windows.NewLazyDLL`, `UTF16PtrFromString`, `Errno`, `Handle`,
   `CoInitializeEx`, etc. (Decision point — see §16.)
3. Port `CLAUDE.md`, `.golangci.yml`, and the docs skeleton.

---

## 2. Input acquisition — the native `.winmd` reader (`internal/winmd`)

This is the largest net-new subsystem (the piece the macOS repo gets for free from
Clang). Two parts:

**(a) Acquire the winmd.** `generate fetch-metadata` downloads the pinned
`Microsoft.Windows.SDK.Win32Metadata` `.nupkg` from NuGet, unzips it (a nupkg is a
zip; `archive/zip`), and extracts `Windows.Win32.winmd` into
`metadata/winmd/Windows.Win32.winmd` (committed, with a
`metadata/winmd/PROVENANCE.json` recording package version + hash). Committing the
winmd means regeneration needs no network — the analogue of the committed
`.gometa.json` cache.

**(b) Parse the winmd.** A native Go ECMA-335 reader — no .NET, no cgo. Stages:
1. **PE load** — `debug/pe` to locate the CLI header and the metadata root.
2. **Stream headers** — parse the `#~` (compressed tables), `#Strings`, `#US`,
   `#GUID`, `#Blob` streams.
3. **Table decode** — walk the metadata tables we need: `TypeDef`, `TypeRef`,
   `TypeSpec`, `MethodDef`, `Field`, `Param`, `Constant`, `CustomAttribute`,
   `InterfaceImpl`, `NestedClass`, `MemberRef`, `ClassLayout`, `FieldLayout`.
   Decode coded indices and table row sizes per ECMA-335 §II.22–24.
4. **Signature blobs** — decode `MethodDefSig`/`FieldSig`/`TypeSpec` blobs into the
   recursive `TypeRef` IR (§3): element types, `PTR`, `SZARRAY`, `ARRAY`,
   `CLASS`/`VALUETYPE` tokens, `GENERICINST`.
5. **Custom attributes** — decode the win32-specific attribute blobs that carry the
   semantics `projections.md` documents: `DllImport` (DLL/entrypoint/calling
   convention/`SetLastError`), `Guid`, `SupportedArchitecture`, `SupportedOSPlatform`,
   `Ansi`/`Unicode`, `Const`, `NativeTypedef`/`MetadataTypedef`, `NativeArrayInfo`,
   `MemorySize`, `RAIIFree`/`FreeWith`/`InvalidHandleValue`/`AlsoUsableFor`,
   `NativeBitfield`, `FlexibleArray`, `StructSizeField`, `AssociatedEnum`,
   `Documentation`, `In`/`Out`/`Optional`/`Reserved`/`RetVal`/`ComOutPtr`.

**Design for testability:** the reader is a self-contained package with the winmd
as its only input. Use the documented `win32json` schema as a **golden oracle** —
a test that diffs the reader's projection of a namespace against the corresponding
`win32json` file catches decode bugs cheaply. Keep the reader behind an
`Ingester` interface so a win32json-based ingester could be swapped in for
bootstrapping/comparison without touching load/emit.

---

## 3. The intermediate representation (`internal/win32meta`)

Adapt `macosplatformmetadata.FrameworkMeta`. Keep the field names/naming standard;
swap ObjC-isms for Win32-isms. One JSON file per **namespace** = one `NamespaceMeta`.

```go
type NamespaceMeta struct {
    Namespace     string            // "Windows.Win32.System.Threading"
    Package       string            // "threading"  (leaf, lowercased)
    SchemaVersion int
    Provenance    Provenance        // win32json SHA, winmd version

    Structs   map[string]Struct     // includes unions (IsUnion)
    Enums     map[string]Enum
    Functions []Function            // the flat Win32 surface (most of the API)
    Constants []Constant            // was Extern (GUIDs, #defines, PROPERTYKEYs)
    Interfaces map[string]ComInterface   // was Protocol (COM)
    Delegates map[string]FuncPointer     // was BlockType (callbacks)
    Typedefs  map[string]Typedef         // NativeTypedef handle/alias wrappers
}
```

Field-level mapping from the macOS IR:

| macOS IR                    | Win32 IR                      | Source in win32json |
|-----------------------------|-------------------------------|---------------------|
| `Param.ObjCType` (raw str)  | `Param.Type` (**structured** `TypeRef`) | recursive `Type` grammar |
| `Method.IsNSError`          | `Function.ReturnsHRESULT` / `SetLastError` | `SetLastError`, return type |
| `Protocol` / `Method.Selector` | `ComInterface` / `Method.Name` | `Kind:"Com"`, `Methods[]` |
| `BlockType`                 | `FuncPointer`                 | `Kind:"FunctionPointer"` |
| `Availability`              | `Availability{Platform, Architectures}` | `Platform`, `Architectures` |
| `Enum.IsBitmask`            | `Enum.IsFlags`                | `Flags` |
| ownership by heuristic      | **ownership by `Api` field**  | `ApiRef.Api` |

**Key structural difference:** the winmd reader decodes ECMA-335 signature blobs
directly into this **structured** recursive `TypeRef` (element type + `PTR`/
`SZARRAY`/`ARRAY`/`CLASS`/`VALUETYPE` token), so the typemap consumes a tree
rather than parsing a `qualType` string — *simpler and less error-prone* than the
macOS `Normalise`/regex approach. (This `TypeRef` is intentionally shaped to match
the `win32json` `Kind`/`Child` grammar so that projection can serve as the reader's
test oracle.)

```go
type TypeRef struct {
    Kind   string   // Native | ApiRef | PointerTo | Array | LPArray | MissingClrType
    Name   string   // for Native ("UInt32") and ApiRef ("HANDLE")
    Api    string   // ApiRef owning namespace ("Foundation") → import resolution
    TargetKind string // Default | Com | Enum | FunctionPointer
    Child  *TypeRef // PointerTo / Array element
    Shape  *ArrayShape // Array size
}
```

---

## 4. Phase 1 — Ingest (`internal/win32meta/ingest`)

`Ingest(winmd) → []*NamespaceMeta`, written to `metadata/win32/<ns>.w32meta.json`.
The winmd exposes one `TypeDef` per type; free functions and constants live as
`static` members of a synthetic `Apis` class per namespace — so ingestion groups
`TypeDef`s by namespace, reads each namespace's `Apis` members into
`Functions`/`Constants`, and reads sibling `TypeDef`s into
`Structs`/`Enums`/`Interfaces`/`Delegates`/`Typedefs`.

- Project the reader's tables into `NamespaceMeta`. Committing the resulting
  `.w32meta.json` keeps the emit phase runnable without re-parsing the winmd.
- Normalize: strip the `Windows.Win32.` prefix; compute leaf `Package` names;
  resolve `UnicodeAliases` (`CreateFile→CreateFileW`) into a decision to **emit
  the `W` variant unsuffixed** and drop the `A` variant (matching CsWin32 /
  windows-rs convention).
- Apply **overrides sidecars** (`metadata/win32/<ns>/overrides.json`) at ingest
  time — same declarative-fixup pattern as the macOS loader (exclude, remap type,
  force-flags, rename). Keeps committed metadata pure.
- Optional docs merge: `Microsoft.Windows.SDK.Win32Docs` MessagePack dictionary
  (keyed by API name → `ApiDetails`) → `Doc` fields, the analogue of the
  `appledocs.json` sidecar merge.

Provenance + `SchemaVersion` gate exactly as macOS (`ErrSchemaTooNew/Old`).

---

## 5. Phase 2 — Load (`internal/codegen/pipeline/loader.go`)

`LoadAll(metadataDir) → *Registry`. **Massively simpler than the macOS loader**
because win32json namespaces are authoritative — we **delete the entire "fewest
non-zero methods wins" ownership heuristic**.

`Registry` indices (all `*Index` per the naming standard):
- `OwnerIndex map[string]string` — typeName → owning namespace. Built trivially by
  walking each namespace's declared types (no scoring). Cross-namespace refs use
  the `TypeRef.Api` field directly.
- `StructIndex`, `EnumIndex`, `EnumBaseIndex`, `InterfaceIndex`, `TypedefIndex`,
  `DelegateIndex`, `ConstantIndex`.
- `TypedefTargetIndex` — `HANDLE → uintptr`, `PWSTR → *uint16`, etc. (follow
  `NativeTypedef.Def`).
- `BlockedImports map[string]map[string]bool` — cross-namespace cycle-break set
  (kept: the Win32 namespace graph has cycles, e.g. Foundation ↔ others).

**Keep from macOS:**
- `filterInheritedInterfaceMethods` — COM interfaces list only their own vtable
  slots; parent methods come via embedding (`IUnknown` → `IDispatch` → …).
- `detectAndBreakCycles` + `BlockedImports` (degrade to `uintptr`/`unsafe.Pointer`
  on a blocked cross-namespace edge, recording a diagnostic).
- `SortByDependency` — topo-sort namespaces so referenced packages emit first.

---

## 6. Phase 3 — Emit — reuse the view→render compiler verbatim in shape

Directory layout mirrors `internal/codegen/emit/`:

```
internal/codegen/emit/
  raw/                (pkg rawwin)   syscall/COM bindings  → bindings/win32/
  idiomatic/          (pkg idiowin)                        → opinionated/idiomatic/win32/
```

Each leaf is the identical three-package compiler:

1. **`<leaf>/*.go` (gather)** — the *only* place type/naming/import decisions live.
   `structs.go`, `enums.go`, `functions.go`, `interfaces.go`, `delegates.go`,
   `constants.go`, `typedefs.go`. Consumes `Registry` + `typemap.Mapper`, produces
   pure-data view IR. May use `fmt.Fprintf` only to build **fragments**
   (an expression, a comment), never file bodies.
2. **`<leaf>/view/`** (pkg `view`) — pure-data IR structs, imports nothing from
   meta/typemap. Carries pre-rendered fragments + enum-like discriminants
   (`ReturnKind`, `DispatchKind ∈ {DllProc, ComVtable}`) so templates only branch.
3. **`<leaf>/render/`** (pkg `render`) — `//go:embed templates/*.tmpl`, typed
   `Execute*` funcs, imports **only** `view` (the render firewall). No Go syntax
   is string-built here.
4. **`<leaf>/render/templates/*.tmpl`**.

**Reuse wholesale:**
- `internal/codegen/shared/fileasm` — file scaffold (DO-NOT-EDIT header +
  `//go:build windows` + package + import block + body via `file.tmpl`), plus the
  import groupers. Every file finalized through `go/format.Source`.
- **Imports computed from resolved types**, never scanned from output text — the
  `Mapper.GoType(ref, ctx, imports)` call populates an `ImportSet` as a side effect.
- The **two gofmt invariants** (indented doc-comment smart-quoting; one-line vs
  multiline body preservation) — carry the discipline over.
- The **regenerate + `git diff` empty gate** as the master correctness check.

---

## 7. Type mapping (`internal/codegen/typemap/mapper.go`)

Consumes the structured `TypeRef` tree (no string parsing). Resolution ladder:

1. `Native` → primitive: `Void→`(elided)`, `Byte→byte`, `Int32→int32`,
   `UInt32→uint32`, `UInt64→uint64`, `Single→float32`, `Double→float64`,
   `Char→uint16`, `Boolean→bool`, `IntPtr→uintptr`.
2. `ApiRef` with `TargetKind`:
   - handle/alias typedefs (`HANDLE→uintptr`, `PWSTR→*uint16`, `BOOL→int32`,
     `HRESULT→int32`) via `TypedefTargetIndex`.
   - `Enum` → the named Go enum type (qualified + import if cross-namespace).
   - `Com` → `*IFoo` wrapper pointer.
   - `Default` struct → by value (param & field) / `unsafe.Pointer` or `*T` per
     context on return.
3. `PointerTo` → `*<child>`; `PointerTo Void → unsafe.Pointer`;
   `PointerTo Char/WChar` at param position with `[Const]` → `string` in the
   idiomatic tier (raw tier keeps `*byte`/`*uint16`).
4. `Array{Shape.Size:N}` → `[N]<child>`; `LPArray` → slice/pointer per attrs
   (`NativeArrayInfo` count linkage feeds the idiomatic slice params).

**Keep the macOS discipline:** `ImportSet` side-effect, the `Diagnostics`
degradation recorder (every fallthrough-to-`uintptr`/`unsafe.Pointer` logged), and
cycle-aware degradation via `BlockedImports`. Cross-namespace `ApiRef.Api` →
qualified package alias + recorded import.

**Architecture skew:** win32json marks arch-specific types/params with
`Architectures`. Emit per-arch files with `//go:build amd64` / `arm64` build tags
(the zzl generator is x64-only today — we do better by honoring `Architectures`).

---

## 8. Naming (`internal/codegen/naming/naming.go`)

Simpler than ObjC (names are already PascalCase — no selector splitting):
- **Reserved-word/import-collision escaping** for params — port the macOS
  `goReservedWords` defensive set (keywords + `unsafe`, `runtime`, `context`, …).
- **Initialism correction** in the idiomatic tier (`Id→ID`, `Url→URL`, `Rpc→RPC`)
  — port `initialisms.go`.
- `PackageName(namespace)` → lowercased leaf segment (with a collision map for
  duplicate leaves, e.g. two `Common` leaves → namespaced).
- Optional Hungarian-notation stripping for idiomatic param names (`dwFlags→flags`).
- Unicode-variant de-suffixing (`MessageBoxW→MessageBox`) in the idiomatic tier.

---

## 9. Runtime (`bindings/runtime/win32`)

The clean 1:1 replacement for `bindings/runtime/purego`. Pure `syscall` +
`golang.org/x/sys/windows`. Provides:

- **DLL dispatch** — generated `<pkg>_runtime.go` with a `sync.Once`-guarded
  `_loadLibrary` doing `windows.NewLazyDLL("kernel32.dll")` and lazy `NewProc`
  registration (mirrors the purego `_loadLibrary`+`RegisterLibFunc` template
  shape). Calls via `syscall.SyscallN(proc.Addr(), args…)`.
- **Error surfacing** — `SetLastError` functions return `windows.Errno`; a helper
  `Win32Error(name, errno)` wraps context. `HRESULT` returns → `HRESULTError`
  (the failable-call pattern maps exactly onto the macOS `NSError` handling).
  Distinct error domains: Win32 / HRESULT / NTSTATUS (as gowin32 does).
- **Strings** — `UTF16PtrFromString` / `UTF16ToString` helpers (the `GoString`/
  `NSString` analogue).
- **COM** — `bindings/runtime/win32/com`: `IUnknown` base, vtable dispatch
  (`syscall.SyscallN(vtbl[slot], this, args…)`), `AddRef`/`Release` with
  `runtime.SetFinalizer` (the `Track`/`-release` analogue), `QueryInterface`,
  `CoInitializeEx`/`CoCreateInstance`, `BSTR` helpers, `GUID` type.
- **Callbacks** — `syscall.NewCallback` trampolines for `FunctionPointer` params
  (the block-trampoline analogue). Registry of Go closures keyed by handle.
- **STA/thread affinity** — a `RunOnUIThread` helper for APIs needing message-pump
  affinity (the `purego.Main` analogue), if/when UI namespaces are tackled.

---

## 10. Idiomatic layer (`opinionated/idiomatic/win32/`, pkg `idiowin`)

Hermetic (never imports `bindings/win32`; re-does dispatch over the runtime), same
view→render leaf shape. Port the macOS idiomatic patterns:

- **COM interface wrappers** with **base embedding** (`IDispatch` embeds `IUnknown`
  → promotes `QueryInterface`/`AddRef`/`Release`) and **sealed provider interfaces**
  for "accept any derived interface" params.
- **Fluent constructors / failable calls** — `HRESULT`/`SetLastError` calls
  surface as `(T, error)`; the macOS `errkit.FromObjC` becomes `hresult.From`.
- **String ergonomics** — `PWSTR`/`PSTR` params become Go `string`, converted at
  the boundary; array+count param pairs (`NativeArrayInfo`) collapse to `[]T`.
- **Handle RAII** — `[RAIIFree]`/`CloseApi` metadata drives generated
  `Close()`/`defer`-friendly handle wrappers (`HANDLE→CloseHandle`,
  `BSTR→SysFreeString`).
- **Bitfield & union accessors** — `[NativeBitfield]` → get/set methods over the
  backing `_bitfieldN`; unions → accessor methods (`WholeValue()`/`WholeValueVal()`
  à la zzl) since Go has no unions.
- **Enum flags** — `[Flags]` enums get `|`-friendly typed constants.

Hand-crafted escape hatch: `opinionated/library/win32/<domain>/` for hand-written
QoL helpers the generator never overwrites (only `*_generated.go` is regenerated),
matching the macOS `opinionated/library/` rule.

---

## 11. QA & determinism (port directly)

- **`generate validate`** — structural gate: dangling type refs, duplicate types
  across arch without `SupportedArchitecture`, enum base conflicts, missing DLL
  imports. Errors fail CI.
- **`generate diff --old --new`** — semantic API diff between two metadata trees;
  makes win32json/winmd bumps reviewable instead of eyeballing JSON.
- **Diagnostics ratchet** — `--diagnostics-baseline metadata/diagnostics-baseline.json`
  fails when a new `unsafe.Pointer`/`uintptr` degradation appears beyond baseline.
- **Regenerate + `git diff` empty** — the master gate. CI runs `generate bindings`
  + `generate idiomatic` and fails if `bindings/`/`opinionated/` diff is non-empty.

---

## 12. CLI (`cmd/generate/main.go`) — `//go:build windows`

| Sub-command      | Replaces macOS | Does |
|------------------|----------------|------|
| `fetch-metadata` | `scan`         | Download+extract NuGet winmd → `metadata/winmd/` |
| `ingest`         | (scan tail)    | native winmd reader → `metadata/win32/*.w32meta.json` |
| `bindings`       | `bindings`     | Load Registry → emit `bindings/win32/` |
| `idiomatic`      | `idiomatic`    | Emit `opinionated/idiomatic/win32/` |
| `all`            | `all`          | ingest + bindings + idiomatic |
| `validate`       | `validate`     | Structural QA |
| `diff`           | `diff`         | Semantic metadata diff |
| `list`           | `list`         | List namespaces |

Plus `cmd/inspect` (dump a `.w32meta.json`).

---

## 13. Generated output layout

```
bindings/win32/<namespace-leaf>/        (raw tier)
├── doc.go
├── <leaf>_runtime.go        # DLL load + lazy proc registration
├── <leaf>_structs.go
├── <leaf>_enums.go
├── <leaf>_functions.go
├── <leaf>_constants.go
├── <leaf>_interfaces.go     # COM
├── <leaf>_delegates.go      # callback types
├── <leaf>_functions_amd64.go / _arm64.go   # arch-specific
opinionated/idiomatic/win32/<leaf>/     (idiomatic tier)
bindings/runtime/win32/                 (hand-written runtime + com/ callbacks/)
```

All generated files: `// Code generated by go-bindings-win32-codegen. DO NOT EDIT.`
+ `//go:build windows`.

---

## 14. Phasing — vertical slice first, then breadth

**M0 — The winmd reader (native ECMA-335).** Build `internal/winmd`: PE →
streams → tables → signature blobs → custom attributes, projecting one namespace
into the IR. Gate it with the `win32json` golden-oracle diff test. **This is the
riskiest net-new subsystem — nothing downstream works until it does.** Deliver it
first, standalone, with `cmd/inspect` able to dump a namespace.

**M1 — Walking skeleton (one namespace, functions + structs + enums).**
Target `Windows.Win32.System.Threading` + its `Foundation` dependency (HANDLE,
BOOL). Prove: reader → IR → Registry → typemap → gather → view → render → a
compilable `bindings/win32/threading` that can `CreateEventW`/`SetEvent`/`WaitFor…`.
Establish the runtime `_loadLibrary` template, `(T, error)` surfacing, and the
regen+diff gate end-to-end.

**M2 — Constants, typedefs/handles, callbacks, arch skew.** Complete the flat
(non-COM) surface for core namespaces (Foundation, System.Threading, System.Memory,
Storage.FileSystem, Security). Wire `validate` + the diagnostics ratchet.

**M3 — COM (in v1).** `IUnknown`/`IDispatch` base embedding, vtable dispatch
runtime (`bindings/runtime/win32/com`), sealed providers, `HRESULT`→`error`.
Prove on `System.Com` + a real interface (e.g. `IFileDialog`, `IShellItem`).

**M4 — Idiomatic tier.** String/handle/slice ergonomics, failable `(T,error)`,
bitfield/union accessors, flags enums, COM wrappers with embedding. Parity with
gowin32-quality ergonomics on the M1–M3 namespaces.

**M5 — Breadth.** Turn the crank across all ~130+ namespaces (flat + COM). Triage
degradations via the diagnostics baseline. Add `diff` for winmd version bumps.

**M6 — Docs, CI, release.** Port the CI regen gate, golangci-lint, release-please.

---

## 15. Key risks & mitigations

- **Union / bitfield / packing correctness** — Go struct layout must match C ABI.
  Mitigation: emit explicit padding from win32json `Size`/`PackingSize`/
  `FieldOffset`; add ABI round-trip tests (`unsafe.Sizeof`/`Offsetof` == metadata).
- **`syscall.SyscallN` float/struct-by-value ABI** — passing floats and large
  structs by value through `SyscallN` is fiddly. Mitigation: follow zzl's proven
  patterns; for float-heavy namespaces (Direct2D/GDI) consider deferring or a
  targeted shim.
- **winmd reader correctness** (the biggest net-new risk) — ECMA-335 table
  decoding, coded-index sizing, and signature-blob parsing are exacting.
  Mitigation: build it first (M0), standalone; gate every namespace against the
  `win32json` golden oracle; fuzz the blob decoder. The `windows-rs` and
  `py-win32more` readers are proven references for the tricky parts.
- **Callback lifetime** — `syscall.NewCallback` has a process-lifetime callback
  cap. Mitigation: pool/reuse trampolines, document the constraint.
- **Scale** — tens of thousands of functions/structs. Mitigation: the determinism
  gate + diagnostics ratchet keep quality measurable; namespace-parallel emit.

---

## 16. Decisions — resolved & still open

**Resolved (see the "Locked decisions" table in §0):** native `.winmd` reader as
input · `(T, error)` raw-tier errors · COM in v1 · `x/sys/windows` as the single
dep · module path `github.com/deploymenttheory/go-bindings-win32`.

**Still open (can be decided during M0/M1, don't block the start):**
1. **winmd version to pin** — which `Microsoft.Windows.SDK.Win32Metadata` release
   (track the latest stable at bootstrap; record in `PROVENANCE.json`).
2. **Docs source** — pull the `Microsoft.Windows.SDK.Win32Docs` MessagePack docs
   for rich doc-comments in v1, or ship with `[Documentation]` URL links only and
   add prose later.
3. **Architectures shipped** — amd64 + arm64 from the start (honoring
   `SupportedArchitecture`), or amd64-only for M1–M3 and add arm64 in M5.
4. **Float/large-struct-by-value ABI** — whether to include float-heavy namespaces
   (Direct2D/GDI) in v1 or gate them behind a targeted `SyscallN` ABI shim.
