package rawwin

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files under testdata/golden")

// TestGoldenSyntheticNamespaces drives the real pipeline (IR files → Registry
// → Generator → assembled Go source) over a small synthetic API surface that
// exercises one instance of every shaping rule, and compares every emitted
// file plus the diagnostics list against testdata/golden. Any change to the
// emitter's decisions therefore shows up as a reviewable golden diff.
//
// Regenerate after an intended change:
//
//	go test ./internal/codegen/emit/raw/ -run TestGoldenSyntheticNamespaces -update
func TestGoldenSyntheticNamespaces(t *testing.T) {
	got := emitSynthetic(t)

	goldenDir := filepath.Join("testdata", "golden")
	if *updateGolden {
		if err := os.RemoveAll(goldenDir); err != nil {
			t.Fatal(err)
		}
		for name, content := range got {
			path := filepath.Join(goldenDir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("rewrote %d golden files", len(got))
		return
	}

	want := readTree(t, goldenDir)
	names := map[string]bool{}
	for name := range got {
		names[name] = true
	}
	for name := range want {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		t.Run(name, func(t *testing.T) {
			expected, inGolden := want[name]
			actual, emitted := got[name]
			switch {
			case !inGolden:
				t.Errorf("emitted file %s has no golden counterpart (review, then run -update)", name)
			case !emitted:
				t.Errorf("golden file %s was not emitted this run", name)
			case expected != actual:
				t.Errorf("%s differs from golden:\n%s", name, firstDiff(expected, actual))
			}
		})
	}
}

// emitSynthetic runs the pipeline over the synthetic fixture and returns the
// emitted files (slash-separated path → content) plus "diagnostics.txt".
func emitSynthetic(t *testing.T) map[string]string {
	t.Helper()
	metaDir := t.TempDir()
	outDir := t.TempDir()
	for _, meta := range syntheticNamespaces() {
		if err := win32meta.Write(metaDir, meta); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := pipeline.LoadAll(metaDir)
	if err != nil {
		t.Fatal(err)
	}
	generator := New(registry, testModulePath, outDir)
	if _, err := generator.EmitAll(nil); err != nil {
		t.Fatal(err)
	}
	got := readTree(t, outDir)
	diagnostics := slices.Clone(generator.Diagnostics)
	sort.Strings(diagnostics)
	got["diagnostics.txt"] = strings.Join(diagnostics, "\n") + "\n"
	return got
}

// readTree loads every file below root, keyed by slash-separated relative
// path, with line endings normalized.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = strings.ReplaceAll(string(content), "\r\n", "\n")
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s (run with -update to create the golden tree): %v", root, err)
	}
	return files
}

// firstDiff renders the first differing line.
func firstDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			return "line " + strconv.Itoa(i+1) + ":\n  want: " + w + "\n  got:  " + g
		}
	}
	return "(identical after normalization?)"
}

// ── synthetic IR ────────────────────────────────────────────────────────────

func native(name string) win32meta.TypeRef { return win32meta.TypeRef{Kind: "Native", Name: name} }

func pointerTo(child win32meta.TypeRef) win32meta.TypeRef {
	return win32meta.TypeRef{Kind: "PointerTo", Child: &child}
}

func apiRef(api, name, kind string) win32meta.TypeRef {
	return win32meta.TypeRef{Kind: "ApiRef", Api: api, Name: name, TargetKind: kind}
}

func optional(p *win32meta.Param) { p.IsOptional = true }
func retval(p *win32meta.Param)   { p.IsOut = true; p.IsRetVal = true }

func countedBy(index int) func(*win32meta.Param) {
	return func(p *win32meta.Param) { p.NativeArrayCountParamIndex = index }
}

func sizedBy(index int) func(*win32meta.Param) {
	return func(p *win32meta.Param) { p.MemorySizeBytesParamIndex = index }
}

func field(name string, typ win32meta.TypeRef) win32meta.StructField {
	return win32meta.StructField{Name: name, Type: typ}
}

func function(name, dll string, ret win32meta.TypeRef, params ...win32meta.Param) win32meta.Function {
	return win32meta.Function{Name: name, DLL: dll, Return: ret, Params: params}
}

func method(name string, ret win32meta.TypeRef, params ...win32meta.Param) win32meta.ComMethod {
	return win32meta.ComMethod{Name: name, Return: ret, Params: params}
}

func withLastError(f win32meta.Function) win32meta.Function { f.SetLastError = true; return f }

func withUnsuffixed(f win32meta.Function, bare string) win32meta.Function {
	f.UnsuffixedName = bare
	return f
}

func withConstant(f win32meta.Function, value string) win32meta.Function {
	f.Constant = value
	return f
}

func withArch(f win32meta.Function, arches ...string) win32meta.Function {
	f.Availability.Architectures = arches
	return f
}

const (
	foundation = "Foundation"
	systemCom  = "System.Com"
	shapes     = "Test.Shapes"
	other      = "Test.Other"
	testDLL    = "TEST.dll"
)

func handleType() win32meta.TypeRef  { return apiRef(foundation, "HANDLE", "Typedef") }
func boolType() win32meta.TypeRef    { return apiRef(foundation, "BOOL", "Typedef") }
func hresultType() win32meta.TypeRef { return apiRef(foundation, "HRESULT", "Typedef") }
func pwstrType() win32meta.TypeRef   { return apiRef(foundation, "PWSTR", "Typedef") }
func pcwstrType() win32meta.TypeRef  { return apiRef(foundation, "PCWSTR", "Typedef") }

// syntheticNamespaces is the golden fixture: a minimal Foundation and
// System.Com (the real names the emitter keys shaping rules off), plus two
// test namespaces carrying one instance of every rule.
func syntheticNamespaces() []*win32meta.NamespaceMeta {
	foundationMeta := &win32meta.NamespaceMeta{
		Namespace: foundation,
		Typedefs: map[string]win32meta.Typedef{
			"HANDLE":  {Underlying: pointerTo(native("Void")), InvalidValues: []string{"-1", "0"}, FreeFunc: "CloseHandle"},
			"BOOL":    {Underlying: native("Int32")},
			"HRESULT": {Underlying: native("Int32")},
			"PWSTR":   {Underlying: pointerTo(native("Char"))},
			"PCWSTR":  {Underlying: pointerTo(native("Char"))},
		},
		Functions: []win32meta.Function{
			withLastError(function("CloseHandle", "KERNEL32.dll", boolType(), param("hObject", handleType(), in))),
		},
	}

	unknown := win32meta.ComInterface{
		GUID: "00000000-0000-0000-c000-000000000046",
		Methods: []win32meta.ComMethod{
			method("QueryInterface", hresultType(),
				param("riid", pointerTo(native("Guid")), in),
				param("ppvObject", voidDoublePtrType(), comOutPtr)),
			method("AddRef", native("UInt32")),
			method("Release", native("UInt32")),
		},
	}
	comMeta := &win32meta.NamespaceMeta{
		Namespace:  systemCom,
		Interfaces: map[string]win32meta.ComInterface{"IUnknown": unknown},
		Functions: []win32meta.Function{
			// The curated informational-success entry (S_FALSE survives).
			function("CoInitializeEx", "OLE32.dll", hresultType(),
				param("pvReserved", voidPtrType(), in, reserved),
				param("dwCoInit", native("UInt32"), in)),
		},
	}

	small := win32meta.Struct{Fields: []win32meta.StructField{field("a", native("Int32")), field("b", native("Int32"))}}
	medium := win32meta.Struct{Fields: []win32meta.StructField{field("a", native("Int32")), field("b", native("Int32")), field("c", native("Int32"))}}
	big := win32meta.Struct{Fields: []win32meta.StructField{field("a", native("Int64")), field("b", native("Int64")), field("c", native("Int64"))}}
	fpair := win32meta.Struct{Fields: []win32meta.StructField{field("x", native("Single")), field("y", native("Single"))}}
	packed := win32meta.Struct{PackingSize: 1, Fields: []win32meta.StructField{field("tag", native("Byte")), field("value", native("Int32"))}}
	union := win32meta.Struct{IsUnion: true, Fields: []win32meta.StructField{field("i", native("Int32")), field("q", native("Int64"))}}
	withNested := win32meta.Struct{
		Fields: []win32meta.StructField{
			field("kind", native("Int32")),
			field("Anonymous", win32meta.TypeRef{Kind: "ApiRef", Name: "_Anonymous_e__Union"}),
		},
		NestedTypes: map[string]win32meta.Struct{
			"_Anonymous_e__Union": {IsUnion: true, Fields: []win32meta.StructField{field("i", native("Int32")), field("d", native("Double"))}},
		},
	}
	taken := win32meta.Struct{Fields: []win32meta.StructField{field("v", native("Int32"))}}

	smallRef := apiRef(shapes, "SMALL", "Struct")
	otherStructRef := apiRef(other, "OTHER_STRUCT", "Struct")

	shapesMeta := &win32meta.NamespaceMeta{
		Namespace: shapes,
		Typedefs: map[string]win32meta.Typedef{
			"HTEST":      {Underlying: pointerTo(native("Void")), InvalidValues: []string{"-1", "0"}, FreeFunc: "CloseTest"},
			"HLOCALTEST": {Underlying: pointerTo(native("Void")), FreeFunc: "FreeTest"},
		},
		Structs: map[string]win32meta.Struct{
			"SMALL": small, "MEDIUM": medium, "BIG": big, "FPAIR": fpair,
			"PACKED": packed, "UNI": union, "WITHNESTED": withNested, "Taken": taken,
		},
		Enums: map[string]win32meta.Enum{
			"MODE": {BaseType: "int32", Members: []win32meta.EnumMember{{Name: "MODE_A", Value: "0"}, {Name: "MODE_B", Value: "1"}}},
			"FLAGSX": {BaseType: "uint32", IsFlags: true, Members: []win32meta.EnumMember{
				{Name: "F_ONE", Value: "1"}, {Name: "F_TWO", Value: "2"}, {Name: "F_ALIAS", Value: "2"},
			}},
		},
		Constants: []win32meta.Constant{
			{Name: "MAX_THING", Type: native("UInt32"), Value: "42", ValueKind: "UInt"},
			{Name: "BAD_HANDLE", Type: handleType(), Value: "-1", ValueKind: "Int"},
			{Name: "NAME_STR", Type: native("String"), Value: "test", ValueKind: "String"},
			{Name: "CLSID_Test", Type: native("Guid"), Value: "12345678-1234-1234-1234-123456789abc", ValueKind: "Guid"},
		},
		Delegates: map[string]win32meta.FuncPointer{
			"PFN_CALLBACK": {Return: native("UInt32"), Params: []win32meta.Param{param("h", handleType(), in), param("ctx", voidPtrType(), in)}},
			"PFN_PROGRESS": {Return: native("Void"), Params: []win32meta.Param{param("progress", native("Double"), in)}},
		},
		Functions: []win32meta.Function{
			withUnsuffixed(withLastError(function("RequiredStringW", testDLL, boolType(), param("name", pcwstrType(), in))), "RequiredString"),
			function("OptionalStringW", testDLL, hresultType(), param("name", pcwstrType(), in, optional)),
			function("InOutString", testDLL, native("Void"), param("buffer", pwstrType(), in, out)),
			function("BoolIn", testDLL, boolType(), param("flag", boolType(), in)),
			withLastError(function("HandleOpen", testDLL, handleType())),
			function("ReservedParam", testDLL, native("Void"), param("x", native("UInt32"), in), param("reserved", voidPtrType(), in, reserved)),
			withUnsuffixed(function("TakenW", testDLL, native("Void")), "Taken"),
			function("SliceParam", testDLL, hresultType(),
				param("count", native("UInt32"), in),
				param("items", pointerTo(smallRef), in, countedBy(0))),
			withLastError(function("ByteBuffer", testDLL, boolType(),
				param("buffer", voidPtrType(), in, sizedBy(1)),
				param("size", native("UInt32"), in))),
			function("RetValHR", testDLL, hresultType(), param("value", pointerTo(native("UInt32")), retval)),
			withLastError(function("RetValBool", testDLL, boolType(), param("value", pointerTo(native("UInt32")), retval))),
			function("RetValVoid", testDLL, native("Void"), param("value", pointerTo(native("UInt32")), retval)),
			function("ComOut", testDLL, hresultType(), param("riid", guidPtrType(), in), param("ppv", voidDoublePtrType(), comOutPtr)),
			function("RiidPair", testDLL, hresultType(), param("riid", guidPtrType(), in), param("ppv", voidDoublePtrType(), out)),
			function("GetCurrentThing", "FORCEINLINE", handleType()),
			withConstant(function("GetCurrentThingToken", "FORCEINLINE", handleType()), "-4"),
			withConstant(function("InlineInt", "FORCEINLINE", native("Int32")), "-7"),
			function("SmallStruct", testDLL, native("Void"), param("s", smallRef, in)),
			function("MediumStruct", testDLL, native("Void"), param("m", apiRef(shapes, "MEDIUM", "Struct"), in)),
			function("BigStruct", testDLL, native("Void"), param("b", apiRef(shapes, "BIG", "Struct"), in)),
			function("FpairParam", testDLL, native("Void"), param("p", apiRef(shapes, "FPAIR", "Struct"), in)),
			function("FloatParam", testDLL, native("Void"), param("f", native("Single"), in)),
			function("FloatReturn", testDLL, native("Double")),
			function("StructReturn", testDLL, smallRef),
			withLastError(function("StructReturnLastErr", testDLL, smallRef)),
			function("FpairReturn", testDLL, apiRef(shapes, "FPAIR", "Struct")),
			function("BigReturn", testDLL, apiRef(shapes, "BIG", "Struct")),
			function("BoolReturnNative", testDLL, native("Boolean")),
			function("BoolNativeParam", testDLL, native("Void"), param("flag", native("Boolean"), in)),
			function("FreeTest", testDLL, native("Void"), param("h", apiRef(shapes, "HLOCALTEST", "Typedef"), in)),
			function("ShadowParam", testDLL, pointerTo(smallRef), param("SMALL", native("UInt32"), in)),
			withArch(function("Arm64OnlyFn", testDLL, native("Void")), "arm64"),
			withLastError(function("ValueLastErr", testDLL, native("UInt32"))),
			withLastError(function("PtrRet", testDLL, pointerTo(smallRef))),
			function("UseOther", testDLL, native("Void"), param("p", pointerTo(otherStructRef), in)),
			function("EnumParam", testDLL, apiRef(shapes, "MODE", "Enum"), param("flags", apiRef(shapes, "FLAGSX", "Enum"), in)),
			function("CallbackParam", testDLL, native("Void"), param("cb", apiRef(shapes, "PFN_CALLBACK", "FunctionPointer"), in)),
		},
		Interfaces: map[string]win32meta.ComInterface{
			"ITest": {
				GUID: "aaaaaaaa-0000-0000-0000-000000000001", BaseInterface: "IUnknown", BaseInterfaceApi: systemCom,
				Availability: win32meta.Availability{DocURL: "https://example.invalid/itest"},
				Methods: []win32meta.ComMethod{
					method("GetValue", hresultType(), param("value", pointerTo(native("UInt32")), retval)),
					method("SetName", hresultType(), param("name", pcwstrType(), in)),
					method("SetOptional", hresultType(), param("name", pcwstrType(), in, optional)),
					method("GetSize", smallRef),
					method("IsOk", native("Boolean")),
					method("Scale", hresultType(), param("factor", native("Single"), in)),
					method("SetBig", hresultType(), param("value", apiRef(shapes, "BIG", "Struct"), in)),
					method("GetObject", hresultType(), param("riid", guidPtrType(), in), param("ppv", voidDoublePtrType(), out)),
					method("GetItems", hresultType(), param("count", native("UInt32"), in), param("items", pointerTo(smallRef), in, countedBy(0))),
					method("Plain", native("Void")),
					method("GetChild", hresultType(), param("child", pointerTo(apiRef(shapes, "ITest", "Com")), retval)),
					method("GetBig", apiRef(shapes, "BIG", "Struct"), param("index", native("UInt32"), in)),
				},
			},
			"ITest2": {
				GUID: "aaaaaaaa-0000-0000-0000-000000000002", BaseInterface: "ITest", BaseInterfaceApi: shapes,
				Methods: []win32meta.ComMethod{method("Extra", hresultType(), param("flag", boolType(), in))},
			},
			"IEnumTest": {
				GUID: "aaaaaaaa-0000-0000-0000-000000000003", BaseInterface: "IUnknown", BaseInterfaceApi: systemCom,
				Methods: []win32meta.ComMethod{
					method("Next", hresultType(), param("celt", native("UInt32"), in), param("fetched", pointerTo(native("UInt32")), out)),
					method("Skip", hresultType(), param("celt", native("UInt32"), in)),
				},
			},
		},
	}

	otherMeta := &win32meta.NamespaceMeta{
		Namespace: other,
		Structs:   map[string]win32meta.Struct{"OTHER_STRUCT": {Fields: []win32meta.StructField{field("n", native("Int32"))}}},
		Functions: []win32meta.Function{
			withLastError(function("CloseTest", "OTHER.dll", boolType(), param("h", apiRef(shapes, "HTEST", "Typedef"), in))),
			// Creates the Other→Shapes edge that closes the import cycle; the
			// pipeline severs it and this pointer degrades to unsafe.Pointer.
			function("UseShapes", "OTHER.dll", native("Void"), param("s", pointerTo(smallRef), in)),
		},
	}
	return []*win32meta.NamespaceMeta{foundationMeta, comMeta, shapesMeta, otherMeta}
}
