//go:build windows

package win32

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Register-aware calls.
//
// syscall.SyscallN passes every argument as an integer word and returns
// RAX/X0 (plus, on amd64 only, XMM0 as r2). That cannot express a floating-
// point parameter on arm64 (V registers are never loaded), a floating-point
// return on arm64, a by-value composite larger than a register (a pointer to
// a caller copy on x64, two X registers or V registers on arm64), or a
// struct returned by value from a flat function on arm64 (X0:X1, V
// registers, or the X8 indirect result pointer).
//
// Call closes that gap without cgo: a Go-side planner assigns each argument
// to the host ABI's registers or stack — the x64 positional convention with
// XMM0–3 mirroring, or AAPCS64's independent integer/float counters with the
// homogeneous-float-aggregate (HFA) rule — into a callFrame, and a per-arch
// assembly trampoline (call_amd64.s, call_arm64.s) loads the registers,
// copies the stack words, calls the target and captures RAX/X0, RDX/X1 and
// XMM0/D0–D3. The trampoline itself is invoked through syscall.SyscallN as if
// it were the foreign function, so the runtime's g0 stack switch,
// SetLastError(0)/GetLastError capture and callback re-entry machinery are
// reused unchanged.

// ArgKind classifies one Call argument word.
type ArgKind uint8

const (
	// ArgWord is an integer or pointer passed as is.
	ArgWord ArgKind = iota
	// ArgFloat32 is a float32 whose IEEE bits sit in the low 32 bits of the word
	// (math.Float32bits).
	ArgFloat32
	// ArgFloat64 is a float64 passed as its IEEE bits (math.Float64bits).
	ArgFloat64
	// ArgStruct is a by-value composite: the word is a pointer to the value
	// (which Call copies — the caller's value is never modified) and the Arg
	// carries its layout.
	ArgStruct
)

// Arg describes one argument (or, in Spec.Ret, the return value).
type Arg struct {
	Kind ArgKind
	// Size and Align are the C layout of an ArgStruct.
	Size  uint32
	Align uint32
	// HFA is the element count (1–4) when the composite is a homogeneous
	// floating-point aggregate — one to four float32s (or float64s, HFA64)
	// and nothing else — which arm64 passes and returns in V registers.
	// 0 for every other composite.
	HFA   uint8
	HFA64 bool
}

// Argument descriptors for generated code.
var (
	// Word is an integer or pointer argument.
	Word = Arg{Kind: ArgWord}
	// Float32 is a float32 argument (pass math.Float32bits(v)).
	Float32 = Arg{Kind: ArgFloat32}
	// Float64 is a float64 argument (pass math.Float64bits(v)).
	Float64 = Arg{Kind: ArgFloat64}
)

// Struct describes a by-value composite argument or return of the given C
// layout; hfa is its HFA element count (0 when not an HFA) and hfa64 whether
// those elements are float64.
func Struct(size, align uint32, hfa uint8, hfa64 bool) Arg {
	return Arg{Kind: ArgStruct, Size: size, Align: align, HFA: hfa, HFA64: hfa64}
}

// Spec describes a call site: one Arg per argument word passed to Call
// (a COM method's `this` included), and the return value. Ret.Kind is
// ArgWord for integer, pointer and void returns, ArgFloat32/ArgFloat64 for a
// floating-point return (read Result.F0), or ArgStruct for a flat function
// returning a composite by value (Call writes it to the ret buffer).
type Spec struct {
	Args []Arg
	Ret  Arg
}

// Result carries the registers captured after the call.
type Result struct {
	// R1 is RAX / X0; R2 is RDX / X1.
	R1, R2 uintptr
	// F0 is XMM0 / D0: a float64 return's bits, or a float32 return's bits in
	// the low 32 bits.
	F0 uint64
	// Err is GetLastError as captured immediately after the call.
	Err Errno
}

// Tuple returns the integer registers and error in syscall.SyscallN's shape,
// so generated code can dispatch through Call where it would otherwise use
// SyscallN without changing the code that consumes r1/e1.
func (r Result) Tuple() (r1, r2 uintptr, err Errno) { return r.R1, r.R2, r.Err }

// maxStackWords bounds the outgoing stack argument area the trampolines
// reserve (Windows' own syscall path allows 42; 32 keeps the frame small).
const maxStackWords = 32

// callFrame is the trampoline's contract; call_amd64.s and call_arm64.s
// address its fields through the go_asm.h offsets.
type callFrame struct {
	fn uintptr
	// ints are the integer argument registers: RCX,RDX,R8,R9 (ints[0:4]) on
	// amd64; X0–X7 on arm64.
	ints [8]uintptr
	// floats are the floating-point argument registers: XMM0–3 (floats[0:4])
	// on amd64; D0–D7 on arm64.
	floats [8]uint64
	// nstack words at *stack are copied to the outgoing stack argument area
	// (after the 32-byte shadow space on amd64).
	nstack uintptr
	stack  *uintptr
	// x8 is arm64's indirect result location register (a composite return
	// larger than 16 bytes); unused on amd64.
	x8 uintptr
	// Captured after the call.
	r1, r2 uintptr
	f      [4]uint64 // XMM0 (f[0]) on amd64; D0–D3 on arm64
}

// callTrampolineABI0 is the ABI0 entry address of the trampoline, exported
// by the assembly via a DATA symbol (a Go func value would resolve to the
// ABIInternal wrapper, which must not run as a C function). Zero on
// architectures without a trampoline (call_other.go).
var callTrampolineABI0 uintptr

// Call invokes the foreign function fn with the arguments described by spec.
// Each word of args is passed as spec.Args[i] says: integers and pointers
// verbatim, floats as their IEEE bits, by-value composites as a pointer to
// the value (copied by Call). A composite return (spec.Ret.Kind == ArgStruct)
// is written to ret, which must be heap-allocated (see OutParam); pass nil
// otherwise. The registers captured after the call come back in Result.
//
// Pointer arguments converted with uintptr(unsafe.Pointer(p)) directly in
// the argument list are escaped to the heap and kept alive for the call
// (//go:uintptrescapes).
//
//go:uintptrescapes
func Call(fn uintptr, spec *Spec, ret unsafe.Pointer, args ...uintptr) Result {
	if len(args) != len(spec.Args) {
		panic("win32: Call argument count does not match its Spec")
	}
	frame := new(callFrame)
	frame.fn = fn
	var keep [][]byte
	var stack []uintptr
	switch runtime.GOARCH {
	case "amd64":
		keep, stack = planAMD64(frame, spec, ret, args)
	case "arm64":
		keep, stack = planARM64(frame, spec, ret, args)
	default:
		panic("win32: Call is not supported on " + runtime.GOARCH)
	}
	if len(stack) > maxStackWords {
		panic("win32: Call has too many stack arguments")
	}
	if len(stack) > 0 {
		frame.nstack = uintptr(len(stack))
		frame.stack = &stack[0]
	}
	// The frame is written by the trampoline after the callee returns, and a
	// callee that reenters Go can move this goroutine's stack meanwhile —
	// OutParam keeps it on the heap (see outparam.go).
	_, _, err := syscall.SyscallN(callTrampolineABI0, uintptr(OutParam(unsafe.Pointer(frame))))
	runtime.KeepAlive(keep)
	runtime.KeepAlive(stack)
	switch runtime.GOARCH {
	case "amd64":
		finishAMD64(frame, spec, ret)
	case "arm64":
		finishARM64(frame, spec, ret)
	}
	return Result{R1: frame.r1, R2: frame.r2, F0: frame.f[0], Err: err}
}

// ── x64 planner ──────────────────────────────────────────────────────────────
//
// Arguments are positional: the first four words go in RCX, RDX, R8, R9 and
// are mirrored into XMM0–3 (a float in position n lives in XMMn; the integer
// register is simply unused), the rest are stack words. An aggregate of 1, 2,
// 4 or 8 bytes is passed as if it were an integer; any other aggregate is
// passed as a pointer to a 16-byte-aligned caller copy. A flat function
// returns a 1/2/4/8-byte aggregate in RAX and anything else through a hidden
// pointer passed as the first argument.

func planAMD64(frame *callFrame, spec *Spec, ret unsafe.Pointer, args []uintptr) (keep [][]byte, stack []uintptr) {
	words := make([]uintptr, 0, len(args)+1)
	if spec.Ret.Kind == ArgStruct && !registerSized(spec.Ret.Size) {
		words = append(words, uintptr(ret))
	}
	for i, arg := range spec.Args {
		word := args[i]
		if arg.Kind == ArgStruct {
			value := argPointer(args, i)
			if registerSized(arg.Size) {
				word = loadWord(value, arg.Size)
			} else {
				aligned, buffer := alignedCopy(value, arg.Size, 16)
				keep = append(keep, buffer)
				word = uintptr(aligned)
			}
		}
		words = append(words, word)
	}
	for i, word := range words {
		if i < 4 {
			frame.ints[i] = word
			frame.floats[i] = uint64(word)
		} else {
			stack = append(stack, word)
		}
	}
	return keep, stack
}

func finishAMD64(frame *callFrame, spec *Spec, ret unsafe.Pointer) {
	if spec.Ret.Kind == ArgStruct && registerSized(spec.Ret.Size) {
		storeWord(ret, frame.r1, spec.Ret.Size)
	}
}

// ── arm64 planner (AAPCS64 as used by Windows) ───────────────────────────────
//
// Integer/pointer words take the next of X0–X7 (NGRN), floats the next of
// V0–V7 (NSRN), independently. An HFA takes one V register per element when
// they all fit, otherwise the whole aggregate goes to the stack and no later
// float uses a register. Another composite of at most 16 bytes takes
// consecutive X registers (an even-numbered first register when 16-byte
// aligned) when it fits, else the stack; larger composites are copied and
// passed by pointer. A flat function returns an HFA in D0–D3, an aggregate of
// at most 8 bytes in X0, at most 16 in X0:X1, and anything larger through the
// buffer whose address the caller places in X8.

func planARM64(frame *callFrame, spec *Spec, ret unsafe.Pointer, args []uintptr) (keep [][]byte, stack []uintptr) {
	ngrn, nsrn := 0, 0
	putInt := func(word uintptr) {
		if ngrn < 8 {
			frame.ints[ngrn] = word
			ngrn++
			return
		}
		stack = append(stack, word)
	}
	if spec.Ret.Kind == ArgStruct && spec.Ret.HFA == 0 && spec.Ret.Size > 16 {
		frame.x8 = uintptr(ret)
	}
	for i, arg := range spec.Args {
		switch arg.Kind {
		case ArgWord:
			putInt(args[i])
		case ArgFloat32, ArgFloat64:
			if nsrn < 8 {
				frame.floats[nsrn] = uint64(args[i])
				nsrn++
			} else {
				nsrn = 8
				stack = append(stack, args[i])
			}
		case ArgStruct:
			value := argPointer(args, i)
			switch {
			case arg.HFA > 0:
				elements := int(arg.HFA)
				if nsrn+elements <= 8 {
					for e := range elements {
						frame.floats[nsrn] = hfaElement(value, e, arg.HFA64)
						nsrn++
					}
				} else {
					nsrn = 8
					stack = appendComposite(stack, value, arg.Size, arg.Align)
				}
			case arg.Size > 16:
				aligned, buffer := alignedCopy(value, arg.Size, 8)
				keep = append(keep, buffer)
				putInt(uintptr(aligned))
			default:
				words := int((arg.Size + 7) / 8)
				if arg.Align >= 16 && ngrn%2 == 1 {
					ngrn++
				}
				if ngrn+words <= 8 {
					var packed [2]uintptr
					copyBytes(unsafe.Pointer(&packed), value, arg.Size)
					for w := range words {
						frame.ints[ngrn] = packed[w]
						ngrn++
					}
				} else {
					ngrn = 8
					stack = appendComposite(stack, value, arg.Size, arg.Align)
				}
			}
		}
	}
	return keep, stack
}

func finishARM64(frame *callFrame, spec *Spec, ret unsafe.Pointer) {
	if spec.Ret.Kind != ArgStruct {
		return
	}
	switch {
	case spec.Ret.HFA > 0:
		for e := range int(spec.Ret.HFA) {
			if spec.Ret.HFA64 {
				*(*uint64)(unsafe.Add(ret, e*8)) = frame.f[e]
			} else {
				*(*uint32)(unsafe.Add(ret, e*4)) = uint32(frame.f[e])
			}
		}
	case spec.Ret.Size <= 8:
		storeWord(ret, frame.r1, spec.Ret.Size)
	case spec.Ret.Size <= 16:
		storeWord(ret, frame.r1, 8)
		storeWord(unsafe.Add(ret, 8), frame.r2, spec.Ret.Size-8)
	}
	// Larger composites were written by the callee through X8.
}

// ── helpers ──────────────────────────────────────────────────────────────────

// argPointer recovers the pointer an ArgStruct word carries. The caller
// converted it with uintptr(unsafe.Pointer(p)) in Call's argument list, so
// the //go:uintptrescapes contract has escaped the pointee to the heap and
// keeps it alive for the whole call; reading the word's bits back as a
// pointer is therefore safe (heap objects do not move).
func argPointer(args []uintptr, i int) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&args[i]))
}

func registerSized(size uint32) bool {
	switch size {
	case 1, 2, 4, 8:
		return true
	}
	return false
}

// loadWord reads size bytes at p into the low end of a word.
func loadWord(p unsafe.Pointer, size uint32) uintptr {
	var word uintptr
	copyBytes(unsafe.Pointer(&word), p, size)
	return word
}

// storeWord writes the low size bytes of word to p.
func storeWord(p unsafe.Pointer, word uintptr, size uint32) {
	copyBytes(p, unsafe.Pointer(&word), size)
}

func copyBytes(dst, src unsafe.Pointer, size uint32) {
	copy(unsafe.Slice((*byte)(dst), size), unsafe.Slice((*byte)(src), size))
}

// alignedCopy copies size bytes at p into a fresh heap buffer aligned to
// align, returning the aligned address and the buffer that keeps it alive.
func alignedCopy(p unsafe.Pointer, size, align uint32) (unsafe.Pointer, []byte) {
	buffer := make([]byte, size+align)
	base := uintptr(unsafe.Pointer(&buffer[0]))
	offset := (uintptr(align) - base%uintptr(align)) % uintptr(align)
	aligned := unsafe.Pointer(&buffer[offset])
	copyBytes(aligned, p, size)
	return aligned, buffer
}

// hfaElement returns element e of a homogeneous float aggregate as the bits
// a D register carries (a float32 in the low 32 bits).
func hfaElement(p unsafe.Pointer, e int, float64Elements bool) uint64 {
	if float64Elements {
		return *(*uint64)(unsafe.Add(p, e*8))
	}
	return uint64(*(*uint32)(unsafe.Add(p, e*4)))
}

// appendComposite appends a composite's bytes to the stack words: first
// padded to an even word when 16-byte aligned, then the bytes, rounded up to
// whole words.
func appendComposite(stack []uintptr, p unsafe.Pointer, size, align uint32) []uintptr {
	if align >= 16 && len(stack)%2 == 1 {
		stack = append(stack, 0)
	}
	words := int((size + 7) / 8)
	start := len(stack)
	for range words {
		stack = append(stack, 0)
	}
	copyBytes(unsafe.Pointer(&stack[start]), p, size)
	return stack
}
