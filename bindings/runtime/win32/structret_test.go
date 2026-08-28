//go:build windows

package win32

import "testing"

// A register-returned aggregate round-trips through the word exactly as
// StructArg packs it: first field in the low bytes.
func TestStructRetMirrorsStructArg(t *testing.T) {
	want := coord{X: 0x1234, Y: 0x5678}
	if got := StructRet[coord](StructArg(want)); got != want {
		t.Errorf("StructRet(StructArg(%+v)) = %+v", want, got)
	}
	type pair struct{ A, B uint32 }
	if got := StructRet[pair](StructArg(pair{1, 2})); got != (pair{1, 2}) {
		t.Errorf("8-byte round trip = %+v", got)
	}
	// Only the struct's own bytes are read; garbage above them is ignored.
	if got := StructRet[coord](0xFFFF_FFFF_5678_1234); got != want {
		t.Errorf("high garbage leaked: %+v", got)
	}
}

func TestStructRetRejectsOversizedStructs(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("StructRet of a 16-byte struct did not panic")
		}
	}()
	StructRet[[2]uint64](0)
}
