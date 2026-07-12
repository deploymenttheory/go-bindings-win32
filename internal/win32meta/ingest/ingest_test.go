package ingest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
	"github.com/deploymenttheory/go-bindings-win32/internal/winmd"
)

func ingestAll(t *testing.T) ([]*win32meta.NamespaceMeta, *Ingester) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "metadata", "winmd", "Windows.Win32.winmd")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("committed winmd not present: %v", err)
	}
	file, err := winmd.Open(path)
	if err != nil {
		t.Fatalf("winmd.Open: %v", err)
	}
	ingester := New(file, "test")
	namespaces, err := ingester.Ingest()
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	return namespaces, ingester
}

func namespaceByName(t *testing.T, namespaces []*win32meta.NamespaceMeta, name string) *win32meta.NamespaceMeta {
	t.Helper()
	for _, meta := range namespaces {
		if meta.Namespace == name {
			return meta
		}
	}
	t.Fatalf("namespace %s not found", name)
	return nil
}

func TestIngestWholeWinmd(t *testing.T) {
	namespaces, ingester := ingestAll(t)

	totalFunctions, totalStructs, totalEnums, totalInterfaces, totalConstants := 0, 0, 0, 0, 0
	for _, meta := range namespaces {
		totalFunctions += len(meta.Functions)
		totalStructs += len(meta.Structs)
		totalEnums += len(meta.Enums)
		totalInterfaces += len(meta.Interfaces)
		totalConstants += len(meta.Constants)
	}
	t.Logf("namespaces=%d functions=%d structs=%d enums=%d interfaces=%d constants=%d diagnostics=%d",
		len(namespaces), totalFunctions, totalStructs, totalEnums, totalInterfaces, totalConstants,
		len(ingester.Diagnostics))
	for i, diagnostic := range ingester.Diagnostics {
		if i >= 10 {
			t.Logf("... and %d more diagnostics", len(ingester.Diagnostics)-10)
			break
		}
		t.Logf("diagnostic: %s", diagnostic)
	}
	if len(namespaces) < 100 {
		t.Errorf("namespaces = %d, want >= 100", len(namespaces))
	}
	if totalFunctions < 10000 {
		t.Errorf("functions = %d, want >= 10000", totalFunctions)
	}
	if totalInterfaces < 3000 {
		t.Errorf("interfaces = %d, want >= 3000", totalInterfaces)
	}
}

func TestIngestThreading(t *testing.T) {
	namespaces, _ := ingestAll(t)
	threading := namespaceByName(t, namespaces, "System.Threading")

	var createEvent *win32meta.Function
	for i := range threading.Functions {
		if threading.Functions[i].Name == "CreateEventW" {
			createEvent = &threading.Functions[i]
		}
	}
	if createEvent == nil {
		t.Fatal("CreateEventW not ingested")
	}
	if createEvent.DLL != "KERNEL32.dll" {
		t.Errorf("DLL = %q", createEvent.DLL)
	}
	if !createEvent.SetLastError {
		t.Error("SetLastError not set")
	}
	if createEvent.UnsuffixedName != "CreateEvent" {
		t.Errorf("UnsuffixedName = %q, want CreateEvent", createEvent.UnsuffixedName)
	}
	if createEvent.Return.Kind != "ApiRef" || createEvent.Return.Name != "HANDLE" || createEvent.Return.Api != "Foundation" {
		t.Errorf("Return = %+v", createEvent.Return)
	}
	if len(createEvent.Params) != 4 {
		t.Fatalf("params = %d, want 4", len(createEvent.Params))
	}
	if createEvent.Params[3].Name != "lpName" {
		t.Errorf("param3 name = %q, want lpName", createEvent.Params[3].Name)
	}
	if !createEvent.Params[0].IsOptional {
		t.Error("lpEventAttributes should be optional")
	}
}

func TestIngestFoundation(t *testing.T) {
	namespaces, _ := ingestAll(t)
	foundation := namespaceByName(t, namespaces, "Foundation")

	handle, ok := foundation.Typedefs["HANDLE"]
	if !ok {
		t.Fatal("HANDLE typedef missing")
	}
	// HANDLE's metadata underlying type is void*.
	if handle.Underlying.Kind != "PointerTo" || handle.Underlying.Child == nil || handle.Underlying.Child.Name != "Void" {
		t.Errorf("HANDLE underlying = %+v, want PointerTo Void", handle.Underlying)
	}
	if handle.FreeFunc != "CloseHandle" {
		t.Errorf("HANDLE FreeFunc = %q, want CloseHandle", handle.FreeFunc)
	}
	if len(handle.InvalidValues) == 0 {
		t.Error("HANDLE has no invalid values")
	}

	if _, ok := foundation.Structs["POINT"]; !ok {
		t.Error("POINT struct missing")
	}
	boolTypedef, ok := foundation.Typedefs["BOOL"]
	if !ok {
		t.Fatal("BOOL typedef missing")
	}
	if boolTypedef.Underlying.Name != "Int32" {
		t.Errorf("BOOL underlying = %+v, want Int32", boolTypedef.Underlying)
	}
}

func TestIngestComInterface(t *testing.T) {
	namespaces, _ := ingestAll(t)
	com := namespaceByName(t, namespaces, "System.Com")

	unknown, ok := com.Interfaces["IUnknown"]
	if !ok {
		t.Fatal("IUnknown missing")
	}
	if unknown.GUID != "00000000-0000-0000-c000-000000000046" {
		t.Errorf("IUnknown GUID = %q", unknown.GUID)
	}
	if unknown.BaseInterface != "" {
		t.Errorf("IUnknown base = %q, want none", unknown.BaseInterface)
	}
	if len(unknown.Methods) != 3 {
		t.Fatalf("IUnknown methods = %d, want 3", len(unknown.Methods))
	}
	if unknown.Methods[0].Name != "QueryInterface" {
		t.Errorf("vtable[0] = %s, want QueryInterface", unknown.Methods[0].Name)
	}
	// QueryInterface's ppvObject is the canonical [ComOutPtr] void** out-param;
	// IidParameterIndex is absent from the current winmd, so the ingested
	// default (-1) must survive.
	ppv := unknown.Methods[0].Params[1]
	if ppv.Name != "ppvObject" || !ppv.IsComOutPtr || !ppv.IsOut {
		t.Errorf("ppvObject = %+v, want out [ComOutPtr]", ppv)
	}
	if ppv.IidParamIndex != -1 {
		t.Errorf("ppvObject IidParamIndex = %d, want -1 (attribute absent)", ppv.IidParamIndex)
	}
}

func TestIngestEnumAndStruct(t *testing.T) {
	namespaces, _ := ingestAll(t)

	debug := namespaceByName(t, namespaces, "System.Diagnostics.Debug")
	context, ok := debug.Structs["CONTEXT"]
	if !ok {
		t.Fatal("CONTEXT struct missing")
	}
	// CONTEXT is per-architecture: expect variants beyond the primary.
	if len(context.ArchVariants) == 0 {
		t.Error("CONTEXT has no arch variants")
	}

	windowing := namespaceByName(t, namespaces, "UI.WindowsAndMessaging")
	wndClass, ok := windowing.Structs["WNDCLASSEXW"]
	if !ok {
		t.Fatal("WNDCLASSEXW missing")
	}
	if wndClass.StructSizeField != "cbSize" {
		t.Errorf("WNDCLASSEXW StructSizeField = %q, want cbSize", wndClass.StructSizeField)
	}
	var hasWndProcField bool
	for _, field := range wndClass.Fields {
		if field.Name == "lpfnWndProc" && field.Type.Kind == "ApiRef" && field.Type.TargetKind == "FunctionPointer" {
			hasWndProcField = true
		}
	}
	if !hasWndProcField {
		t.Error("WNDCLASSEXW.lpfnWndProc not projected as FunctionPointer ApiRef")
	}
}
