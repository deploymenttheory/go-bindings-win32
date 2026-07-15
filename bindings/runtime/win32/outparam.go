package win32

import "unsafe"

// Native out-parameters must never point into a Go stack.
//
// A native call can reenter Go on the SAME goroutine before it returns —
// most commonly through a syscall.NewCallback trampoline the consumer
// registered (the generated *_delegates.go types document exactly that
// pattern), or through a Go-implemented COM object handed to the callee.
// While the reentered Go code runs (or parks), the goroutine has left
// syscall state, so its stack can MOVE: growth copies it, and a concurrent
// GC may shrink it. Any raw pointer the native side still holds into that
// stack — the out-param word the binding passed to SyscallN — then goes
// stale, and the callee writes its result into freed stack memory. Go's
// heap does not move, so the invariant (generated and hand-written code
// alike) is: every pointer handed to native code for WRITING is
// heap-allocated. (Proven live in the sister go-bindings-winrt: lost
// out-params on reentrant Calendar factory calls.)
//
// Plain `new(T)` is not enough: escape analysis keeps a non-escaping new on
// the stack, and syscall.SyscallN is //go:uintptrkeepalive (liveness only),
// not //go:uintptrescapes. The helpers below defeat escape analysis the same
// way runtime.Escape does — a never-taken store to a package-level sink —
// which the inliner preserves and which propagates through call summaries,
// so even a CALLER's `&local` passed as an out-pointer parameter is moved to
// the heap at the caller.

// alwaysFalse is never set; reading it keeps the compiler from proving the
// sink stores below dead, which is what forces the escapes.
var alwaysFalse bool

// outParamSink is written only on never-taken branches.
var outParamSink unsafe.Pointer

// OutParam returns p unchanged while forcing the allocation p points to onto
// the heap. Generated bindings route every elevated [out,retval] pointer
// through it as `uintptr(win32.OutParam(unsafe.Pointer(result)))`, keeping
// the pointer-to-uintptr conversion in the SyscallN argument list (the
// sanctioned keepalive pattern) while guaranteeing the pointee is
// heap-allocated and so survives any stack move. Inlined: one global load
// and a never-taken branch.
func OutParam(p unsafe.Pointer) unsafe.Pointer {
	if alwaysFalse {
		outParamSink = p
	}
	return p
}
