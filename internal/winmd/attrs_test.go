package winmd

import "testing"

// TestDecodeAllAttributes brute-forces every custom attribute blob in the
// committed winmd through the decoder and reports the attribute-name
// histogram. Only a tiny failure rate for exotic constructs is tolerated.
func TestDecodeAllAttributes(t *testing.T) {
	file := testFile(t)

	histogram := map[string]int{}
	failures := 0
	for row := range file.Tables.CustomAttributes {
		attr, err := file.decodeAttribute(&file.Tables.CustomAttributes[row])
		if err != nil {
			failures++
			if failures <= 5 {
				t.Logf("row %d: %v", row, err)
			}
			continue
		}
		histogram[attr.Name]++
	}
	total := len(file.Tables.CustomAttributes)
	t.Logf("decoded %d/%d attributes; distinct names: %d", total-failures, total, len(histogram))
	for _, name := range []string{
		"DocumentationAttribute", "SupportedArchitectureAttribute",
		"SupportedOSPlatformAttribute", "GuidAttribute", "FlagsAttribute",
		"NativeTypedefAttribute", "RAIIFreeAttribute", "InvalidHandleValueAttribute",
		"ConstAttribute", "NativeArrayInfoAttribute", "MemorySizeAttribute",
		"UnicodeAttribute", "AnsiAttribute",
	} {
		if histogram[name] == 0 {
			t.Errorf("attribute %s: decoded 0 instances", name)
		}
	}
	if failures > 0 {
		t.Errorf("%d attribute blobs failed to decode", failures)
	}
}

// TestGuidAttribute checks IUnknown's [Guid] fixed args reassemble to the
// canonical IID 00000000-0000-0000-C000-000000000046.
func TestGuidAttribute(t *testing.T) {
	file := testFile(t)

	for typeDefRow := range file.Tables.TypeDefs {
		typeDef := &file.Tables.TypeDefs[typeDefRow]
		if typeDef.Name != "IUnknown" || typeDef.Namespace != "Windows.Win32.System.Com" {
			continue
		}
		attrs := file.AttributesFor(CodedIndex{Table: tableTypeDef, Row: uint32(typeDefRow + 1)})
		for _, attr := range attrs {
			if attr.Name != "GuidAttribute" {
				continue
			}
			if len(attr.Fixed) != 11 {
				t.Fatalf("GuidAttribute fixed args = %d, want 11", len(attr.Fixed))
			}
			data1, _ := attr.Fixed[0].(uint32)
			data2, _ := attr.Fixed[1].(uint16)
			data3, _ := attr.Fixed[2].(uint16)
			if data1 != 0 || data2 != 0 || data3 != 0 {
				t.Errorf("IUnknown GUID head = %x-%x-%x, want 0-0-0", data1, data2, data3)
			}
			if b, _ := attr.Fixed[3].(byte); b != 0xC0 {
				t.Errorf("IUnknown GUID data4[0] = %#x, want 0xC0", b)
			}
			if b, _ := attr.Fixed[10].(byte); b != 0x46 {
				t.Errorf("IUnknown GUID data4[7] = %#x, want 0x46", b)
			}
			return
		}
		t.Fatal("IUnknown has no GuidAttribute")
	}
	t.Fatal("IUnknown not found")
}

// TestDllImportViaImplMap checks the P/Invoke info for SetEvent.
func TestDllImportViaImplMap(t *testing.T) {
	file := testFile(t)

	// Find SetEvent's MethodDef row.
	var setEventRow uint32
	for _, typeDef := range file.Tables.TypeDefs {
		if typeDef.Name != "Apis" || typeDef.Namespace != "Windows.Win32.System.Threading" {
			continue
		}
		for row := typeDef.MethodFirst; row < typeDef.MethodEnd; row++ {
			if file.Tables.Methods[row-1].Name == "SetEvent" {
				setEventRow = row
			}
		}
	}
	if setEventRow == 0 {
		t.Fatal("SetEvent not found")
	}
	for _, implMap := range file.Tables.ImplMaps {
		if implMap.MemberForwarded.Table != tableMethodDef || implMap.MemberForwarded.Row != setEventRow {
			continue
		}
		dll := file.Tables.ModuleRefs[implMap.ImportScope-1]
		if dll != "KERNEL32.dll" {
			t.Errorf("SetEvent DLL = %q, want KERNEL32.dll", dll)
		}
		if implMap.ImportName != "SetEvent" {
			t.Errorf("SetEvent entry point = %q", implMap.ImportName)
		}
		const pinvokeSupportsLastError = 0x0040
		if implMap.MappingFlags&pinvokeSupportsLastError == 0 {
			t.Error("SetEvent missing SupportsLastError mapping flag")
		}
		return
	}
	t.Fatal("SetEvent has no ImplMap row")
}
