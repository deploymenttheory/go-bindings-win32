package win32

import (
	"testing"
	"unsafe"
)

// coord mirrors the Win32 COORD: two 16-bit fields, 4 bytes total. It is the
// struct that motivated StructArg — CreatePseudoConsole and the console-output
// family all take one by value.
type coord struct {
	X int16
	Y int16
}

// The packing must match what the callee reads out of the register: the
// struct's own bytes, in order, in the low end of the word. Getting this
// backwards would put the cursor at (Y, X) — a bug that looks like an API
// misunderstanding rather than a marshalling error.
func TestStructArgPacksFieldsInLayoutOrder(t *testing.T) {
	word := StructArg(coord{X: 0x1234, Y: 0x5678})

	if got := uint16(word & 0xFFFF); got != 0x1234 {
		t.Errorf("low half = %#04x, want the first field 0x1234", got)
	}
	if got := uint16((word >> 16) & 0xFFFF); got != 0x5678 {
		t.Errorf("second half = %#04x, want the second field 0x5678", got)
	}

	// Round-tripping through the word must reproduce the struct exactly,
	// which is the property the callee depends on.
	back := *(*coord)(unsafe.Pointer(&word))
	if back != (coord{X: 0x1234, Y: 0x5678}) {
		t.Errorf("round trip = %+v", back)
	}
}

// Bytes above the struct must be zero rather than whatever was on the stack:
// a callee reading a wider register than the struct occupies would otherwise
// see garbage.
func TestStructArgZeroesTheUnusedBytes(t *testing.T) {
	word := StructArg(coord{X: -1, Y: -1})
	if high := word >> 32; high != 0 {
		t.Errorf("bytes above the struct = %#x, want 0", high)
	}
}

func TestStructArgHandlesEveryRegisterSize(t *testing.T) {
	type oneByte struct{ A uint8 }
	type twoBytes struct{ A uint16 }
	type eightBytes struct{ A, B uint32 }

	if got := StructArg(oneByte{A: 0x7F}); got != 0x7F {
		t.Errorf("1-byte struct = %#x", got)
	}
	if got := StructArg(twoBytes{A: 0xBEEF}); got != 0xBEEF {
		t.Errorf("2-byte struct = %#x", got)
	}
	if got := StructArg(eightBytes{A: 0xAAAAAAAA, B: 0xBBBBBBBB}); got != 0xBBBBBBBBAAAAAAAA {
		t.Errorf("8-byte struct = %#x", got)
	}
}

// A struct larger than a register travels by reference under the ABI, so it
// must panic rather than silently pass a truncated value. The generator never
// emits such a call, but the guard is what keeps that true if it ever changes.
func TestStructArgPanicsOnAnOversizedStruct(t *testing.T) {
	type tooBig struct{ A, B, C uint64 }

	defer func() {
		if recover() == nil {
			t.Fatal("an oversized struct was packed into one word instead of panicking")
		}
	}()
	_ = StructArg(tooBig{})
}
