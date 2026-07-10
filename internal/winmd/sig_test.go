package winmd

import "testing"

// TestDecodeAllSignatures brute-forces every method and field signature in
// the committed winmd through the blob decoder. Any construct the decoder
// does not understand fails the test.
func TestDecodeAllSignatures(t *testing.T) {
	file := testFile(t)

	methodFailures := 0
	for i := range file.Tables.Methods {
		method := &file.Tables.Methods[i]
		if _, err := file.MethodSignature(method.Signature); err != nil {
			methodFailures++
			if methodFailures <= 5 {
				t.Errorf("method %s: %v", method.Name, err)
			}
		}
	}
	fieldFailures := 0
	for i := range file.Tables.Fields {
		field := &file.Tables.Fields[i]
		if _, err := file.FieldSignature(field.Signature); err != nil {
			fieldFailures++
			if fieldFailures <= 5 {
				t.Errorf("field %s: %v", field.Name, err)
			}
		}
	}
	t.Logf("decoded %d method sigs (%d failures), %d field sigs (%d failures)",
		len(file.Tables.Methods), methodFailures, len(file.Tables.Fields), fieldFailures)
	if methodFailures > 0 || fieldFailures > 0 {
		t.Fatalf("%d method + %d field signature failures", methodFailures, fieldFailures)
	}
}

// TestKnownSignature spot-checks CreateEventW's decoded shape:
// HANDLE CreateEventW(SECURITY_ATTRIBUTES *lpEventAttributes, BOOL bManualReset,
//
//	BOOL bInitialState, PWSTR lpName)
func TestKnownSignature(t *testing.T) {
	file := testFile(t)

	for _, typeDef := range file.Tables.TypeDefs {
		if typeDef.Name != "Apis" || typeDef.Namespace != "Windows.Win32.System.Threading" {
			continue
		}
		for row := typeDef.MethodFirst; row < typeDef.MethodEnd; row++ {
			method := &file.Tables.Methods[row-1]
			if method.Name != "CreateEventW" {
				continue
			}
			sig, err := file.MethodSignature(method.Signature)
			if err != nil {
				t.Fatalf("MethodSignature: %v", err)
			}
			if sig.Return.Kind != SigNamed || sig.Return.Name != "HANDLE" {
				t.Errorf("return = %+v, want named HANDLE", sig.Return)
			}
			if len(sig.Params) != 4 {
				t.Fatalf("params = %d, want 4", len(sig.Params))
			}
			if sig.Params[0].Kind != SigPointer || sig.Params[0].Child.Name != "SECURITY_ATTRIBUTES" {
				t.Errorf("param0 = %+v, want *SECURITY_ATTRIBUTES", sig.Params[0])
			}
			if sig.Params[1].Kind != SigNamed || sig.Params[1].Name != "BOOL" {
				t.Errorf("param1 = %+v, want BOOL", sig.Params[1])
			}
			// Constness is carried by the [Const] param attribute, not the
			// type: the metadata type here is PWSTR.
			if sig.Params[3].Kind != SigNamed || sig.Params[3].Name != "PWSTR" {
				t.Errorf("param3 = %+v, want PWSTR", sig.Params[3])
			}
			return
		}
	}
	t.Fatal("CreateEventW not found")
}
