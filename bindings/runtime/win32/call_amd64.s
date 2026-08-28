//go:build windows

#include "go_asm.h"
#include "textflag.h"

// Outgoing stack area: the 32-byte shadow space Windows x64 callees expect,
// then up to maxStackWords argument words. Must stay a multiple of 16 so the
// stack pointer is 16-byte aligned at the CALL.
#define OUTGOING_BYTES (32 + const_maxStackWords*8)

// callTrampoline is entered as a C function by the runtime's asmstdcall
// (syscall.SyscallN) with CX = *callFrame, on the g0 system stack. It loads
// the argument registers and stack words from the frame, calls frame.fn,
// and stores RAX, RDX and XMM0 back into the frame. Only volatile registers
// are used, so nothing needs saving for the caller.
TEXT ·callTrampoline(SB),NOSPLIT|NOFRAME,$0
	MOVQ	SP, AX
	SUBQ	$16, SP
	ANDQ	$~15, SP		// 16-byte alignment as per Windows requirement
	MOVQ	AX, 8(SP)		// saved entry SP
	MOVQ	CX, 0(SP)		// *callFrame
	SUBQ	$OUTGOING_BYTES, SP

	// Copy the stack argument words after the shadow space.
	MOVQ	callFrame_nstack(CX), R10
	TESTQ	R10, R10
	JZ	loaded
	MOVQ	callFrame_stack(CX), R11
	LEAQ	32(SP), R8
copy:
	MOVQ	(R11), R9
	MOVQ	R9, (R8)
	ADDQ	$8, R11
	ADDQ	$8, R8
	DECQ	R10
	JNZ	copy
loaded:
	// Floating-point arguments in the first four positions travel in XMM0–3;
	// the planner mirrored every word so the register the callee reads is
	// always loaded.
	MOVQ	(callFrame_floats+0)(CX), X0
	MOVQ	(callFrame_floats+8)(CX), X1
	MOVQ	(callFrame_floats+16)(CX), X2
	MOVQ	(callFrame_floats+24)(CX), X3
	MOVQ	callFrame_fn(CX), AX
	MOVQ	(callFrame_ints+8)(CX), DX
	MOVQ	(callFrame_ints+16)(CX), R8
	MOVQ	(callFrame_ints+24)(CX), R9
	MOVQ	(callFrame_ints+0)(CX), CX	// last: CX held the frame
	CALL	AX

	ADDQ	$OUTGOING_BYTES, SP
	MOVQ	0(SP), CX
	MOVQ	AX, callFrame_r1(CX)
	MOVQ	DX, callFrame_r2(CX)
	MOVQ	X0, (callFrame_f+0)(CX)
	MOVQ	8(SP), SP
	RET

// callTrampolineABI0 exposes the ABI0 entry address to Go.
DATA	·callTrampolineABI0+0(SB)/8, $·callTrampoline(SB)
GLOBL	·callTrampolineABI0(SB), NOPTR|RODATA, $8
