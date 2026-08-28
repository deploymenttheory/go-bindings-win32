//go:build windows

package win32

import (
	"math"
	"syscall"
	"testing"
	"unsafe"
)

// ── live calls through the trampoline (this machine's architecture) ────────

var (
	ucrtbase = NewDLL("ucrtbase.dll")
	procPow  = ucrtbase.NewProc("pow")
	procFmaf = ucrtbase.NewProc("fmaf")
)

// pow(double, double) double: two float arguments in XMM0/XMM1 (D0/D1) and
// a float return in XMM0 (D0).
func TestCallFloat64ArgsAndReturn(t *testing.T) {
	spec := &Spec{Args: []Arg{Float64, Float64}, Ret: Float64}
	result := Call(procPow.Addr(), spec, nil, uintptr(math.Float64bits(2)), uintptr(math.Float64bits(10)))
	if got := math.Float64frombits(result.F0); got != 1024 {
		t.Fatalf("pow(2, 10) = %v, want 1024", got)
	}
}

// fmaf(float, float, float) float: single-precision arguments and return.
func TestCallFloat32ArgsAndReturn(t *testing.T) {
	spec := &Spec{Args: []Arg{Float32, Float32, Float32}, Ret: Float32}
	result := Call(procFmaf.Addr(), spec,
		nil, uintptr(math.Float32bits(2)), uintptr(math.Float32bits(3)), uintptr(math.Float32bits(4)))
	if got := math.Float32frombits(uint32(result.F0)); got != 10 {
		t.Fatalf("fmaf(2, 3, 4) = %v, want 10", got)
	}
}

// GDI+: GdipAddPathLine(path, x1, y1, x2, y2) has its fifth argument — a
// float — on the stack on x64, and reading the points back proves every
// coordinate landed where the callee expected it.
func TestCallStackFloatArgument(t *testing.T) {
	gdiplus := NewDLL("gdiplus.dll")
	startup := gdiplus.NewProc("GdiplusStartup")
	shutdown := gdiplus.NewProc("GdiplusShutdown")
	createPath := gdiplus.NewProc("GdipCreatePath")
	deletePath := gdiplus.NewProc("GdipDeletePath")
	addLine := gdiplus.NewProc("GdipAddPathLine")
	pointCount := gdiplus.NewProc("GdipGetPointCount")
	pathPoints := gdiplus.NewProc("GdipGetPathPoints")
	for _, proc := range []*Proc{startup, shutdown, createPath, deletePath, addLine, pointCount, pathPoints} {
		if err := proc.Find(); err != nil {
			t.Skipf("gdiplus unavailable: %v", err)
		}
	}

	type startupInput struct {
		Version                  uint32
		DebugEventProc           uintptr
		SuppressBackgroundThread int32
		SuppressExternalCodecs   int32
	}
	input := &startupInput{Version: 1}
	token := new(uintptr)
	if status, _, _ := syscall.SyscallN(startup.Addr(), uintptr(OutParam(unsafe.Pointer(token))), uintptr(unsafe.Pointer(input)), 0); status != 0 {
		t.Skipf("GdiplusStartup status %d", status)
	}
	defer syscall.SyscallN(shutdown.Addr(), *token)

	path := new(uintptr)
	if status, _, _ := syscall.SyscallN(createPath.Addr(), 0, uintptr(OutParam(unsafe.Pointer(path)))); status != 0 {
		t.Fatalf("GdipCreatePath status %d", status)
	}
	defer syscall.SyscallN(deletePath.Addr(), *path)

	spec := &Spec{Args: []Arg{Word, Float32, Float32, Float32, Float32}}
	result := Call(addLine.Addr(), spec, nil, *path,
		uintptr(math.Float32bits(1.5)), uintptr(math.Float32bits(2.5)),
		uintptr(math.Float32bits(3.5)), uintptr(math.Float32bits(4.5)))
	if result.R1 != 0 {
		t.Fatalf("GdipAddPathLine status %d", result.R1)
	}

	count := new(int32)
	if status, _, _ := syscall.SyscallN(pointCount.Addr(), *path, uintptr(OutParam(unsafe.Pointer(count)))); status != 0 || *count != 2 {
		t.Fatalf("GdipGetPointCount = (%d, status %d), want 2 points", *count, status)
	}
	points := new([2][2]float32)
	if status, _, _ := syscall.SyscallN(pathPoints.Addr(), *path, uintptr(OutParam(unsafe.Pointer(points))), 2); status != 0 {
		t.Fatalf("GdipGetPathPoints status %d", status)
	}
	if want := [2][2]float32{{1.5, 2.5}, {3.5, 4.5}}; *points != want {
		t.Fatalf("path points = %v, want %v (the stack float y2 must arrive intact)", *points, want)
	}
}

// shlwapi!AssocCreate takes a CLSID by value: a 16-byte composite (a pointer
// to a copy on x64, two X registers on arm64).
func TestCallStructByValue16(t *testing.T) {
	assocCreate := NewDLL("shlwapi.dll").NewProc("AssocCreate")
	if err := assocCreate.Find(); err != nil {
		t.Skipf("AssocCreate unavailable: %v", err)
	}
	clsidQueryAssociations := GUID{Data1: 0xa07034fd, Data2: 0x6caa, Data3: 0x4954, Data4: [8]byte{0xac, 0x3f, 0x97, 0xa2, 0x72, 0x16, 0xf9, 0x8a}}
	iidQueryAssociations := GUID{Data1: 0xc46ca590, Data2: 0x3c3f, Data3: 0x11d2, Data4: [8]byte{0xbe, 0xe6, 0x00, 0x00, 0xf8, 0x05, 0xca, 0x57}}
	out := new(*IUnknown)
	spec := &Spec{Args: []Arg{Struct(16, 4, 0, false), Word, Word}}
	result := Call(assocCreate.Addr(), spec, nil,
		uintptr(unsafe.Pointer(&clsidQueryAssociations)),
		uintptr(unsafe.Pointer(&iidQueryAssociations)),
		uintptr(OutParam(unsafe.Pointer(out))))
	if err := ErrIfFailed(int32(result.R1)); err != nil {
		t.Fatalf("AssocCreate(CLSID_QueryAssociations by value): %v", err)
	}
	if *out == nil {
		t.Fatal("AssocCreate returned no object")
	}
	(*out).Release()
}

// msdelta!GetDeltaInfoB takes a 24-byte DELTA_INPUT by value (a pointer to
// a copy on both architectures). An empty input is rejected with a
// last-error code, which proves the call reached the callee intact and the
// error capture works across the trampoline.
func TestCallStructByValue24(t *testing.T) {
	getDeltaInfo := NewDLL("msdelta.dll").NewProc("GetDeltaInfoB")
	if err := getDeltaInfo.Find(); err != nil {
		t.Skipf("msdelta unavailable: %v", err)
	}
	type deltaInput struct {
		Start    unsafe.Pointer
		Size     uintptr
		Editable int32
		_        int32
	}
	var input deltaInput
	header := new([16]uint64)
	spec := &Spec{Args: []Arg{Struct(24, 8, 0, false), Word}}
	result := Call(getDeltaInfo.Addr(), spec, nil, uintptr(unsafe.Pointer(&input)), uintptr(OutParam(unsafe.Pointer(header))))
	if result.R1 != 0 {
		t.Fatalf("GetDeltaInfoB(empty) succeeded, want failure")
	}
	if result.Err == 0 {
		t.Fatal("GetDeltaInfoB failed without a GetLastError code")
	}
}

// d2d1!D2D1ConvertColorSpace returns a 16-byte D2D1_COLOR_F: through a hidden
// first-argument pointer on x64, in S0–S3 as a four-float HFA on arm64.
func TestCallStructReturn16(t *testing.T) {
	convert := NewDLL("d2d1.dll").NewProc("D2D1ConvertColorSpace")
	if err := convert.Find(); err != nil {
		t.Skipf("D2D1ConvertColorSpace unavailable: %v", err)
	}
	const colorSpaceSRGB = 1
	color := [4]float32{0.25, 0.5, 0.75, 1}
	ret := new([4]float32)
	spec := &Spec{Args: []Arg{Word, Word, Word}, Ret: Struct(16, 4, 4, false)}
	Call(convert.Addr(), spec, OutParam(unsafe.Pointer(ret)), colorSpaceSRGB, colorSpaceSRGB, uintptr(unsafe.Pointer(&color)))
	if *ret != color {
		t.Fatalf("D2D1ConvertColorSpace(sRGB → sRGB) = %v, want %v", *ret, color)
	}
}

func TestCallRejectsArgumentCountMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Call with a mismatched Spec did not panic")
		}
	}()
	Call(procPow.Addr(), &Spec{Args: []Arg{Float64}}, nil, 1, 2)
}

// ── planners (pure Go; both architectures checked on any host) ─────────────

func TestPlanAMD64(t *testing.T) {
	type big struct{ A, B, C uint64 }
	type small struct{ X, Y int32 }
	value := big{1, 2, 3}
	pair := small{7, 8}
	spec := &Spec{Args: []Arg{Float64, Struct(24, 8, 0, false), Word, Struct(8, 4, 0, false), Float32, Word}}
	frame := new(callFrame)
	keep, stack := planAMD64(frame, spec, nil, []uintptr{
		uintptr(math.Float64bits(1.5)), uintptr(unsafe.Pointer(&value)), 42,
		uintptr(unsafe.Pointer(&pair)), uintptr(math.Float32bits(2.5)), 99,
	}, nil)
	if frame.floats[0] != math.Float64bits(1.5) || frame.ints[0] != uintptr(math.Float64bits(1.5)) {
		t.Errorf("position 0 not mirrored into XMM0/RCX: %#x / %#x", frame.floats[0], frame.ints[0])
	}
	if len(keep) != 1 || frame.ints[1]%16 != 0 || *(*big)(wordPointer(&frame.ints[1])) != value {
		t.Errorf("24-byte composite: want a 16-byte-aligned copy passed by pointer in RDX; got %#x (keep=%d)", frame.ints[1], len(keep))
	}
	if frame.ints[2] != 42 {
		t.Errorf("R8 = %d, want 42", frame.ints[2])
	}
	if got := *(*small)(unsafe.Pointer(&frame.ints[3])); got != pair {
		t.Errorf("8-byte composite in R9 = %+v, want %+v", got, pair)
	}
	if len(stack) != 2 || uint32(stack[0]) != math.Float32bits(2.5) || stack[1] != 99 {
		t.Errorf("stack words = %#x, want [float32 2.5 bits, 99]", stack)
	}

	// A composite return larger than a register: hidden pointer first.
	ret := new(big)
	frame = new(callFrame)
	planAMD64(frame, &Spec{Args: []Arg{Word}, Ret: Struct(24, 8, 0, false)}, unsafe.Pointer(ret), []uintptr{5}, nil)
	if frame.ints[0] != uintptr(unsafe.Pointer(ret)) || frame.ints[1] != 5 {
		t.Errorf("hidden result pointer must be RCX and shift the arguments: %#x %d", frame.ints[0], frame.ints[1])
	}
	// A register-sized composite return: reconstructed from RAX.
	out := new(small)
	frame = &callFrame{r1: loadWord(unsafe.Pointer(&pair), 8)}
	finishAMD64(frame, &Spec{Ret: Struct(8, 4, 0, false)}, unsafe.Pointer(out))
	if *out != pair {
		t.Errorf("finishAMD64 = %+v, want %+v", *out, pair)
	}
}

func TestPlanARM64(t *testing.T) {
	t.Run("independent integer and float counters", func(t *testing.T) {
		spec := &Spec{Args: []Arg{Float64, Word, Float32, Word}}
		frame := new(callFrame)
		_, stack := planARM64(frame, spec, nil, []uintptr{uintptr(math.Float64bits(1.5)), 1, uintptr(math.Float32bits(2.5)), 2}, nil)
		if frame.floats[0] != math.Float64bits(1.5) || uint32(frame.floats[1]) != math.Float32bits(2.5) {
			t.Errorf("D0/D1 = %#x/%#x", frame.floats[0], frame.floats[1])
		}
		if frame.ints[0] != 1 || frame.ints[1] != 2 || len(stack) != 0 {
			t.Errorf("X0/X1 = %d/%d, stack %v", frame.ints[0], frame.ints[1], stack)
		}
	})
	t.Run("ninth word goes to the stack", func(t *testing.T) {
		args := make([]uintptr, 9)
		spec := &Spec{Args: make([]Arg, 9)}
		for i := range args {
			args[i] = uintptr(i + 1)
			spec.Args[i] = Word
		}
		frame := new(callFrame)
		_, stack := planARM64(frame, spec, nil, args, nil)
		if frame.ints[7] != 8 || len(stack) != 1 || stack[0] != 9 {
			t.Errorf("X7 = %d, stack = %v", frame.ints[7], stack)
		}
	})
	t.Run("HFA in V registers, float32 elements in the low word", func(t *testing.T) {
		point := [2]float32{1.5, 2.5}
		spec := &Spec{Args: []Arg{Float64, Struct(8, 4, 2, false), Word}}
		frame := new(callFrame)
		planARM64(frame, spec, nil, []uintptr{uintptr(math.Float64bits(9)), uintptr(unsafe.Pointer(&point)), 3}, nil)
		if uint32(frame.floats[1]) != math.Float32bits(1.5) || uint32(frame.floats[2]) != math.Float32bits(2.5) {
			t.Errorf("HFA elements in D1/D2 = %#x/%#x", frame.floats[1], frame.floats[2])
		}
		if frame.ints[0] != 3 {
			t.Errorf("X0 = %d, want 3 (HFA consumes no X register)", frame.ints[0])
		}
	})
	t.Run("HFA that does not fit goes whole to the stack and closes the V file", func(t *testing.T) {
		quad := [4]float64{1, 2, 3, 4}
		spec := &Spec{Args: make([]Arg, 8)}
		args := make([]uintptr, 8)
		for i := 0; i < 6; i++ {
			spec.Args[i] = Float64
			args[i] = uintptr(math.Float64bits(float64(i)))
		}
		spec.Args[6] = Struct(32, 8, 4, true)
		args[6] = uintptr(unsafe.Pointer(&quad))
		spec.Args[7] = Float32 // after the overflow every float is stacked
		args[7] = uintptr(math.Float32bits(7))
		frame := new(callFrame)
		_, stack := planARM64(frame, spec, nil, args, nil)
		if frame.floats[6] != 0 || frame.floats[7] != 0 {
			t.Errorf("D6/D7 must stay unused: %#x/%#x", frame.floats[6], frame.floats[7])
		}
		if len(stack) != 5 || *(*[4]float64)(unsafe.Pointer(&stack[0])) != quad || uint32(stack[4]) != math.Float32bits(7) {
			t.Errorf("stack = %v, want the 4 doubles then the trailing float32", stack)
		}
	})
	t.Run("16-byte composite in two X registers, or stacked when it does not fit", func(t *testing.T) {
		guid := GUID{Data1: 0x11223344, Data2: 0x5566, Data3: 0x7788, Data4: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}}
		frame := new(callFrame)
		planARM64(frame, &Spec{Args: []Arg{Word, Struct(16, 4, 0, false)}}, nil, []uintptr{1, uintptr(unsafe.Pointer(&guid))}, nil)
		if got := *(*GUID)(unsafe.Pointer(&frame.ints[1])); got != guid {
			t.Errorf("GUID in X1:X2 = %v, want %v", got, guid)
		}
		spec := &Spec{Args: []Arg{Word, Word, Word, Word, Word, Word, Word, Struct(16, 4, 0, false), Word}}
		frame = new(callFrame)
		_, stack := planARM64(frame, spec, nil, []uintptr{1, 2, 3, 4, 5, 6, 7, uintptr(unsafe.Pointer(&guid)), 9}, nil)
		if len(stack) != 3 || *(*GUID)(unsafe.Pointer(&stack[0])) != guid || stack[2] != 9 {
			t.Errorf("stack = %v, want the GUID then 9 (NGRN closes at 8)", stack)
		}
		if frame.ints[7] != 0 {
			t.Errorf("X7 must stay unused once a composite overflowed: %d", frame.ints[7])
		}
	})
	t.Run("larger composite by pointer to a copy", func(t *testing.T) {
		value := [3]uint64{1, 2, 3}
		frame := new(callFrame)
		keep, _ := planARM64(frame, &Spec{Args: []Arg{Struct(24, 8, 0, false)}}, nil, []uintptr{uintptr(unsafe.Pointer(&value))}, nil)
		if len(keep) != 1 || frame.ints[0] == uintptr(unsafe.Pointer(&value)) || *(*[3]uint64)(wordPointer(&frame.ints[0])) != value {
			t.Errorf("X0 = %#x, want a pointer to a copy of %v", frame.ints[0], value)
		}
	})
	t.Run("composite returns", func(t *testing.T) {
		// HFA: elements from D0–D3.
		color := new([4]float32)
		frame := &callFrame{f: [4]uint64{uint64(math.Float32bits(1)), uint64(math.Float32bits(2)), uint64(math.Float32bits(3)), uint64(math.Float32bits(4))}}
		finishARM64(frame, &Spec{Ret: Struct(16, 4, 4, false)}, unsafe.Pointer(color))
		if *color != [4]float32{1, 2, 3, 4} {
			t.Errorf("HFA return = %v", *color)
		}
		// 16 bytes: X0:X1.
		guid := new(GUID)
		want := GUID{Data1: 0xaabbccdd, Data2: 0xeeff, Data3: 0x1122, Data4: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}}
		words := *(*[2]uintptr)(unsafe.Pointer(&want))
		finishARM64(&callFrame{r1: words[0], r2: words[1]}, &Spec{Ret: Struct(16, 4, 0, false)}, unsafe.Pointer(guid))
		if *guid != want {
			t.Errorf("X0:X1 return = %v, want %v", *guid, want)
		}
		// Larger: X8 carries the buffer address.
		big := new([3]uint64)
		frame = new(callFrame)
		planARM64(frame, &Spec{Ret: Struct(24, 8, 0, false)}, unsafe.Pointer(big), nil, nil)
		if frame.x8 != uintptr(unsafe.Pointer(big)) {
			t.Errorf("X8 = %#x, want the result buffer", frame.x8)
		}
	})
}

// wordPointer reads a frame word that the planner filled with a pointer.
func wordPointer(word *uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(word)) }
