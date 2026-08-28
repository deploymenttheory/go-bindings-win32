//go:build windows

#include "go_asm.h"
#include "textflag.h"

// callTrampoline is entered as a C function by the runtime's asmstdcall
// (syscall.SyscallN) with R0 = *callFrame, on the g0 system stack. It mirrors
// the runtime's own asmstdcall frame discipline (R19/R20 saved in the 16-byte
// local area), additionally loading D0–D7 and X8, and capturing X0, X1 and
// D0–D3 after the call.
TEXT ·callTrampoline(SB),NOSPLIT,$16
	STP	(R19, R20), 16(RSP)	// save callee-saved R19, R20
	MOVD	R0, R19			// *callFrame
	MOVD	RSP, R20		// saved stack pointer

	// Reserve and fill the outgoing stack argument area (even word count
	// keeps RSP 16-byte aligned).
	MOVD	callFrame_nstack(R19), R9
	CBZ	R9, regs
	ADD	$1, R9, R10
	AND	$~1, R10
	LSL	$3, R10
	SUB	R10, RSP
	MOVD	callFrame_stack(R19), R11
	MOVD	RSP, R12
	LSL	$3, R9, R9		// byte count
	MOVD	$0, R13
copy:
	MOVD	(R13)(R11), R14
	MOVD	R14, (R13)(R12)
	ADD	$8, R13
	CMP	R13, R9
	BNE	copy

regs:
	FMOVD	(callFrame_floats+0)(R19), F0
	FMOVD	(callFrame_floats+8)(R19), F1
	FMOVD	(callFrame_floats+16)(R19), F2
	FMOVD	(callFrame_floats+24)(R19), F3
	FMOVD	(callFrame_floats+32)(R19), F4
	FMOVD	(callFrame_floats+40)(R19), F5
	FMOVD	(callFrame_floats+48)(R19), F6
	FMOVD	(callFrame_floats+56)(R19), F7
	MOVD	callFrame_x8(R19), R8
	MOVD	(callFrame_ints+0)(R19), R0
	MOVD	(callFrame_ints+8)(R19), R1
	MOVD	(callFrame_ints+16)(R19), R2
	MOVD	(callFrame_ints+24)(R19), R3
	MOVD	(callFrame_ints+32)(R19), R4
	MOVD	(callFrame_ints+40)(R19), R5
	MOVD	(callFrame_ints+48)(R19), R6
	MOVD	(callFrame_ints+56)(R19), R7
	MOVD	callFrame_fn(R19), R12
	BL	(R12)

	MOVD	R20, RSP		// release the stack argument area
	MOVD	R0, callFrame_r1(R19)
	MOVD	R1, callFrame_r2(R19)
	FMOVD	F0, (callFrame_f+0)(R19)
	FMOVD	F1, (callFrame_f+8)(R19)
	FMOVD	F2, (callFrame_f+16)(R19)
	FMOVD	F3, (callFrame_f+24)(R19)

	LDP	16(RSP), (R19, R20)	// restore callee-saved registers
	RET

// callTrampolineABI0 exposes the ABI0 entry address to Go.
DATA	·callTrampolineABI0+0(SB)/8, $·callTrampoline(SB)
GLOBL	·callTrampolineABI0(SB), NOPTR|RODATA, $8
