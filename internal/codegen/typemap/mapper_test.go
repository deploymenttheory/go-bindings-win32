package typemap

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

const modulePath = "github.com/deploymenttheory/go-bindings-win32"

func native(name string) win32meta.TypeRef { return win32meta.TypeRef{Kind: "Native", Name: name} }

func pointerTo(child win32meta.TypeRef) win32meta.TypeRef {
	return win32meta.TypeRef{Kind: "PointerTo", Child: &child}
}

func apiRef(api, name, kind string) win32meta.TypeRef {
	return win32meta.TypeRef{Kind: "ApiRef", Api: api, Name: name, TargetKind: kind}
}

// testRegistry builds an in-memory Registry with the handful of definitions
// the table cases reference.
func testRegistry() *pipeline.Registry {
	handle := &win32meta.Typedef{Underlying: pointerTo(native("Void"))}
	pwstr := &win32meta.Typedef{Underlying: pointerTo(native("Char"))}
	boolT := &win32meta.Typedef{Underlying: native("Int32")}
	return &pipeline.Registry{
		TypedefIndex: map[string]*win32meta.Typedef{
			"Foundation.HANDLE": handle,
			"Foundation.PWSTR":  pwstr,
			"Foundation.BOOL":   boolT,
		},
		EnumBaseIndex:  map[string]string{"Test.A.MODE": "int32"},
		StructIndex:    map[string]*win32meta.Struct{"Test.A.S": {}, "Test.B.T": {}},
		DelegateIndex:  map[string]*win32meta.FuncPointer{"Test.A.PFN": {}},
		InterfaceIndex: map[string]*win32meta.ComInterface{"Test.A.IFoo": {}},
	}
}

func newMapper() *Mapper {
	return &Mapper{
		Registry:     testRegistry(),
		ModulePath:   modulePath,
		Blocked:      map[string]map[string]bool{"Test.A": {"Test.Blocked": true}},
		SkippedTypes: map[string]bool{"Test.A.SKIPPED": true},
	}
}

func TestGoTypeTable(t *testing.T) {
	ctx := Context{Namespace: "Test.A"}
	cases := []struct {
		name        string
		ref         win32meta.TypeRef
		ctx         Context
		wantGo      string
		wantKind    Kind
		wantImports map[string]string
		wantDiag    bool
	}{
		{name: "void", ref: native("Void"), wantGo: "", wantKind: KindVoid},
		{name: "int32", ref: native("Int32"), wantGo: "int32", wantKind: KindScalar},
		{name: "float", ref: native("Single"), wantGo: "float32", wantKind: KindScalar},
		{name: "double", ref: native("Double"), wantGo: "float64", wantKind: KindScalar},
		{name: "bool", ref: native("Boolean"), wantGo: "bool", wantKind: KindScalar},
		{name: "guid", ref: native("Guid"), wantGo: "win32.GUID", wantKind: KindGUID},
		{name: "unknown native degrades", ref: native("Decimal"), wantGo: "uintptr", wantKind: KindUnsupported, wantDiag: true},
		{name: "void pointer", ref: pointerTo(native("Void")), wantGo: "unsafe.Pointer", wantKind: KindPointer},
		{name: "int pointer", ref: pointerTo(native("UInt32")), wantGo: "*uint32", wantKind: KindPointer},
		{name: "pointer to degraded pointee stays a pointer", ref: pointerTo(native("Decimal")), wantGo: "unsafe.Pointer", wantKind: KindPointer, wantDiag: true},
		{name: "handle typedef", ref: apiRef("Foundation", "HANDLE", "Typedef"), wantGo: "foundation.HANDLE", wantKind: KindHandleTypedef,
			wantImports: map[string]string{"foundation": modulePath + "/bindings/win32/foundation"}},
		{name: "pointer typedef", ref: apiRef("Foundation", "PWSTR", "Typedef"), wantGo: "foundation.PWSTR", wantKind: KindPointerTypedef,
			wantImports: map[string]string{"foundation": modulePath + "/bindings/win32/foundation"}},
		{name: "scalar typedef", ref: apiRef("Foundation", "BOOL", "Typedef"), wantGo: "foundation.BOOL", wantKind: KindScalarTypedef,
			wantImports: map[string]string{"foundation": modulePath + "/bindings/win32/foundation"}},
		// The import is recorded before the typedef lookup fails; writeFile's
		// usage pruning drops it again, so the stray entry is harmless.
		{name: "unknown typedef degrades", ref: apiRef("Foundation", "NOPE", "Typedef"), wantGo: "uintptr", wantKind: KindUnsupported, wantDiag: true,
			wantImports: map[string]string{"foundation": modulePath + "/bindings/win32/foundation"}},
		{name: "local enum unqualified", ref: apiRef("Test.A", "MODE", "Enum"), wantGo: "MODE", wantKind: KindEnum},
		{name: "local struct", ref: apiRef("Test.A", "S", "Struct"), wantGo: "S", wantKind: KindStruct},
		{name: "cross-namespace struct qualified", ref: apiRef("Test.B", "T", "Struct"), wantGo: "testb.T", wantKind: KindStruct,
			wantImports: map[string]string{"testb": modulePath + "/bindings/win32/test/b"}},
		{name: "union", ref: apiRef("Test.A", "U", "Union"), wantGo: "U", wantKind: KindUnion},
		{name: "delegate", ref: apiRef("Test.A", "PFN", "FunctionPointer"), wantGo: "PFN", wantKind: KindFuncPtr},
		{name: "com pointer", ref: apiRef("Test.A", "IFoo", "Com"), wantGo: "*IFoo", wantKind: KindComPtr},
		{name: "pointer to com", ref: pointerTo(apiRef("Test.A", "IFoo", "Com")), wantGo: "**IFoo", wantKind: KindPointer},
		{name: "skipped type degrades", ref: apiRef("Test.A", "SKIPPED", "Struct"), wantGo: "uintptr", wantKind: KindUnsupported, wantDiag: true},
		{name: "blocked com flattens", ref: apiRef("Test.Blocked", "IBar", "Com"), wantGo: "unsafe.Pointer", wantKind: KindPointer, wantDiag: true},
		{name: "blocked delegate flattens", ref: apiRef("Test.Blocked", "PFN", "FunctionPointer"), wantGo: "uintptr", wantKind: KindFuncPtr, wantDiag: true},
		{name: "blocked struct degrades", ref: apiRef("Test.Blocked", "X", "Struct"), wantGo: "uintptr", wantKind: KindUnsupported, wantDiag: true},
		{name: "fixed array", ref: win32meta.TypeRef{Kind: "Array", ArrayLen: 4, Child: &win32meta.TypeRef{Kind: "Native", Name: "Byte"}}, wantGo: "[4]byte", wantKind: KindArray},
		{name: "array without length degrades", ref: win32meta.TypeRef{Kind: "Array", Child: &win32meta.TypeRef{Kind: "Native", Name: "Byte"}}, wantGo: "uintptr", wantKind: KindUnsupported, wantDiag: true},
		{name: "unknown kind degrades", ref: win32meta.TypeRef{Kind: "Bogus"}, wantGo: "uintptr", wantKind: KindUnsupported, wantDiag: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMapper()
			imports := ImportSet{}
			c := tc.ctx
			if c.Namespace == "" {
				c = ctx
			}
			got := m.GoType(&tc.ref, c, imports)
			if got.GoType != tc.wantGo || got.Kind != tc.wantKind {
				t.Errorf("GoType = (%q, %v), want (%q, %v)", got.GoType, got.Kind, tc.wantGo, tc.wantKind)
			}
			for alias, path := range tc.wantImports {
				if imports[alias] != path {
					t.Errorf("import %s = %q, want %q", alias, imports[alias], path)
				}
			}
			if tc.wantImports == nil && len(imports) != 0 {
				t.Errorf("unexpected imports recorded: %v", imports)
			}
			if (len(m.Diagnostics) > 0) != tc.wantDiag {
				t.Errorf("diagnostics = %v, wantDiag=%v", m.Diagnostics, tc.wantDiag)
			}
		})
	}
}

func TestArgClassOf(t *testing.T) {
	cases := []struct {
		name string
		res  Resolved
		want ArgClass
	}{
		{"int scalar", Resolved{GoType: "int32", Kind: KindScalar}, ArgScalar},
		{"float32 unsupported", Resolved{GoType: "float32", Kind: KindScalar}, ArgUnsupported},
		{"float64 unsupported", Resolved{GoType: "float64", Kind: KindScalar}, ArgUnsupported},
		{"bool unsupported", Resolved{GoType: "bool", Kind: KindScalar}, ArgUnsupported},
		{"enum", Resolved{GoType: "MODE", Kind: KindEnum}, ArgScalar},
		{"handle typedef", Resolved{GoType: "HANDLE", Kind: KindHandleTypedef}, ArgScalar},
		{"scalar typedef", Resolved{GoType: "BOOL", Kind: KindScalarTypedef}, ArgScalar},
		{"func ptr", Resolved{GoType: "PFN", Kind: KindFuncPtr}, ArgScalar},
		{"degraded", Resolved{GoType: "uintptr", Kind: KindUnsupported}, ArgScalar},
		{"pointer", Resolved{GoType: "*T", Kind: KindPointer}, ArgPointer},
		{"pointer typedef", Resolved{GoType: "PWSTR", Kind: KindPointerTypedef}, ArgPointer},
		{"com pointer", Resolved{GoType: "*IFoo", Kind: KindComPtr}, ArgPointer},
		{"struct by value", Resolved{GoType: "S", Kind: KindStruct}, ArgUnsupported},
		{"union by value", Resolved{GoType: "U", Kind: KindUnion}, ArgUnsupported},
		{"array by value", Resolved{GoType: "[4]byte", Kind: KindArray}, ArgUnsupported},
		{"guid by value", Resolved{GoType: "win32.GUID", Kind: KindGUID}, ArgUnsupported},
		{"void", Resolved{GoType: "", Kind: KindVoid}, ArgUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ArgClassOf(tc.res, tc.res.GoType); got != tc.want {
				t.Errorf("ArgClassOf = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestImportPaths(t *testing.T) {
	m := &Mapper{ModulePath: modulePath}
	if got := m.ImportPathFor("System.Threading"); got != modulePath+"/bindings/win32/system/threading" {
		t.Errorf("ImportPathFor = %q", got)
	}
	if got := m.RuntimeImportPath(); got != modulePath+"/bindings/runtime/win32" {
		t.Errorf("RuntimeImportPath = %q", got)
	}
	m.LocalBindingsRoot = "/bindings/wdk/"
	m.Win32ModulePath = "example.com/win32"
	if got := m.ImportPathFor("Foundation"); got != modulePath+"/bindings/wdk/foundation" {
		t.Errorf("ImportPathFor with local root = %q", got)
	}
	if got := m.RuntimeImportPath(); got != "example.com/win32/bindings/runtime/win32" {
		t.Errorf("RuntimeImportPath with win32 module = %q", got)
	}
}
