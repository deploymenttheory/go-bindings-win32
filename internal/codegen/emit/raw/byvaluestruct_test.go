package rawwin

import (
	"strings"
	"testing"
)

// The Windows x64 convention passes a struct or union of 1, 2, 4 or 8 bytes as
// if it were an integer of the same size — in a register. Anything else travels
// by reference, which is a different call shape, so it must stay skipped rather
// than be passed truncated for the callee to read as garbage.
func TestRegisterStructWordAcceptsOnlyRegisterSizedStructs(t *testing.T) {
	for _, size := range []uint32{1, 2, 4, 8} {
		word, ok := registerStructWord(size, "size")
		if !ok {
			t.Errorf("a %d-byte struct was refused; the ABI passes it in a register", size)
			continue
		}
		if !strings.Contains(word, "win32.StructArg(size)") {
			t.Errorf("size %d: word = %q, want a StructArg packing", size, word)
		}
	}

	for _, size := range []uint32{0, 3, 5, 6, 7, 12, 16, 24} {
		if _, ok := registerStructWord(size, "value"); ok {
			t.Errorf("a %d-byte struct was accepted; the ABI passes it by reference", size)
		}
	}
}

// COORD is the case that motivated this: 2×int16 = 4 bytes, and without it
// CreatePseudoConsole, ResizePseudoConsole, SetConsoleCursorPosition and the
// console-output family cannot be expressed as syscall arguments at all.
func TestCOORDSizedStructIsPassedInARegister(t *testing.T) {
	word, ok := registerStructWord(4, "size")
	if !ok {
		t.Fatal("a COORD-sized struct was refused")
	}
	if word != "uintptr(win32.StructArg(size))" {
		t.Fatalf("word = %q", word)
	}
}
