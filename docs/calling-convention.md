# How calls reach Win32: the two dispatch paths

Every generated function and COM method is a thin, inlined body that
converts its parameters and calls the target. Which of two dispatchers it
uses is decided by the generator from the metadata; the templates never
know. This page explains both paths, the ABI facts they encode, and how to
maintain the hand-written half.

## Path 1 — `syscall.SyscallN` (about 95% of call sites)

`syscall.SyscallN(addr, words...)` passes every argument as one integer
word and returns RAX/X0 (and, on amd64 only, XMM0 as `r2`). That is exactly
enough for:

- integers, pointers, handles, enums, `BOOL`, `bool` (`win32.Bool8`),
  strings (`win32.UTF16Ptr` / `UTF16PtrOrNil`), slices (`&s[0]` + `len`);
- by-value aggregates of 1, 2, 4 or 8 bytes with no floats — the x64
  convention passes those *as if they were integers*, and arm64 puts a
  non-float aggregate that small in one X register (`win32.StructArg`);
- integer returns, and 1/2/4/8-byte non-float aggregate returns from flat
  functions (RAX / X0, `win32.StructRet[T]`);
- COM methods returning any struct by value: non-static member functions
  return aggregates through a hidden pointer placed right after `this` on
  both architectures, so the generator passes `&_ret` as the first word.

## Path 2 — `win32.Call` (about 5%)

Everything else needs registers `SyscallN` cannot load or read:

| Shape | x64 | arm64 |
|---|---|---|
| `float32`/`float64` parameter | XMM0–3 by position, then stack | V0–V7 by float count, then stack |
| float return | XMM0 | D0 |
| by-value aggregate, not 1/2/4/8 bytes | pointer to a 16-byte-aligned copy | ≤16 B: 1–2 X registers; HFA: V registers; >16 B: pointer to a copy |
| flat struct return, not 1/2/4/8 bytes | hidden pointer as the first argument | ≤8 B: X0; ≤16 B: X0:X1; HFA: D0–D3; larger: buffer address in X8 |

`win32.Call(fn, spec, ret, args...)` takes the same integer words plus a
`*win32.Spec` describing each word (`win32.Word`, `win32.Float32`,
`win32.Float64`, or `win32.Struct(size, align, hfa, hfa64)` when the word is
a pointer to a by-value composite). The generator computes the descriptor
from the metadata's C layout — `sizes.go` keeps a scalar-leaf census per
layout so the arm64 *homogeneous floating-point aggregate* rule (1–4 members
of one float type, e.g. `D2D_POINT_2F`) is known — and emits it once per call
site as a package-level `spec<Name>` variable; there is no per-call
allocation for the descriptor.

Inside `Call` (`bindings/runtime/win32/call.go`):

1. A **planner** for the host architecture (`planAMD64` / `planARM64`, plain
   Go, unit-tested on any host) fills a `callFrame`: integer registers,
   floating-point registers, stack words, arm64's X8, and the result
   buffer. Composites the ABI passes by reference are copied (x64 requires a
   16-byte-aligned temporary), so the caller's value is never modified.
2. A **trampoline** in assembly (`call_amd64.s`, `call_arm64.s`) is entered
   *through* `syscall.SyscallN` as if it were the foreign function, with the
   frame's address as its single argument. That reuses the Go runtime's g0
   stack switch, `SetLastError(0)` / `GetLastError` capture and callback
   re-entry machinery unchanged. The trampoline copies the stack words,
   loads every argument register from the frame, calls the target, and
   stores RAX/X0, RDX/X1 and XMM0 / D0–D3 back into the frame.
3. `finishAMD64` / `finishARM64` reconstruct a struct return from the
   captured registers where the ABI returns one that way.

Frames are pooled (`sync.Pool`); a callee that re-enters Go and calls again
simply takes another frame. `Call` is `//go:uintptrescapes`, so a pointer
converted with `uintptr(unsafe.Pointer(p))` in its argument list is escaped
to the heap and kept alive for the whole call — the same guarantee
`SyscallN` gives, obtained with the one pragma non-std code may use.

No cgo is involved anywhere, so `GOOS=windows go build` keeps working from
macOS and Linux.

## Where the ABI facts come from

- x64: *x64 calling convention* (learn.microsoft.com/cpp/build/x64-calling-convention).
  Aggregates of 8/16/32/64 bits travel as integers; other sizes as a pointer
  to caller memory (16-byte aligned); floats in XMM0–3 by *position*.
- arm64: *Overview of ARM64 ABI conventions* (learn.microsoft.com/cpp/build/arm64-windows-abi-conventions),
  which is AAPCS64 stage B/C verbatim, plus the Windows rule that a
  non-static member function returns an aggregate through a pointer in X1
  when `this` is in X0.
- Go: `internal/runtime/syscall/windows/asm_windows_amd64.s` mirrors args
  1–4 into XMM0–3 and stores XMM0 into `r2`; the arm64 version does neither
  (golang/go#62583), which is why the shim exists.

## What the ABI does not let us do

- **Variadic functions.** The metadata declares the `wsprintf` family with
  their fixed parameters only (all integers/pointers), so the bindings can
  call them correctly on both architectures but cannot pass a variadic tail.
- **Callbacks with floats.** `syscall.NewCallback` cannot marshal float
  parameters or results (golang/go#45300); the affected delegates say so in
  their doc comments.
- **More than 42 stack words.** The trampolines reserve Go's own syscall
  limit; the generator skips a call site that could exceed it (none exist).

## Maintaining the shim

- The frame layout is shared with the assembly through `go_asm.h`
  (`callFrame_ints`, `callFrame_floats`, …): add fields freely, never reorder
  what the `.s` files read without rebuilding both.
- Keep the trampolines' frame discipline identical to Go's own
  `asmstdcall` for the architecture (they were copied from it) — that is
  what makes entering them through `SyscallN` safe.
- Every planner rule has a unit test in `call_test.go` (`TestPlanAMD64`,
  `TestPlanARM64`); every live shape has an acceptance test in
  `acceptance/registercall_test.go`. CI runs both on `windows-latest` and on
  a `windows-11-arm` runner, so an arm64 regression fails a real job.
- If Go ever loads V registers in its own `asmstdcall`, nothing here needs
  to change; the shim would merely become optional.
