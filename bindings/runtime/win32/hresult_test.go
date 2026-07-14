package win32

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestErrIfFailed(t *testing.T) {
	if err := ErrIfFailed(int32(S_OK)); err != nil {
		t.Fatalf("S_OK returned error %v", err)
	}
	if err := ErrIfFailed(int32(S_FALSE)); err != nil {
		t.Fatalf("S_FALSE (informational success) returned error %v", err)
	}
	err := ErrIfFailed(int32(E_FAIL))
	if err == nil {
		t.Fatal("E_FAIL returned nil")
	}
	hr, ok := err.(HRESULT)
	if !ok {
		t.Fatalf("failure error is %T, want HRESULT", err)
	}
	if hr != E_FAIL || !hr.Failed() {
		t.Fatalf("got %v (0x%08X), want E_FAIL", hr, uint32(hr))
	}
}

func TestHRESULTErrorsIs(t *testing.T) {
	err := ErrIfFailed(int32(E_ACCESSDENIED))
	if !errors.Is(err, E_ACCESSDENIED) {
		t.Error("errors.Is(err, E_ACCESSDENIED) = false")
	}
	// FACILITY_WIN32 HRESULTs match the Win32 errno they wrap (stdlib
	// syscall.Errno — the same type as x/sys/windows' ERROR_* constants).
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		t.Error("errors.Is(err, syscall.ERROR_ACCESS_DENIED) = false")
	}
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		t.Error("E_ACCESSDENIED matched ERROR_FILE_NOT_FOUND")
	}
	// Non-FACILITY_WIN32 codes never match an errno.
	if errors.Is(ErrIfFailed(int32(E_FAIL)), syscall.Errno(0x4005)) {
		t.Error("E_FAIL matched an errno by low bits")
	}
}

func TestHRESULTErrorMessage(t *testing.T) {
	msg := E_ACCESSDENIED.Error()
	if !strings.Contains(msg, "0x80070005") {
		t.Errorf("message %q does not contain the code", msg)
	}
}
