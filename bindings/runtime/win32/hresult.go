package win32

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// HRESULT is a COM/Win32 HRESULT status code: a 32-bit signed value whose
// sign bit is the failure flag (the C FAILED macro is `hr < 0`). It
// implements error so failed HRESULTs can be returned directly; generated
// bindings only ever return negative (failed) values as errors.
type HRESULT int32

// Failed reports whether the HRESULT indicates failure (FAILED macro).
func (hr HRESULT) Failed() bool { return hr < 0 }

// Error renders the code and, where the system knows one, its message,
// e.g. "HRESULT 0x80070005: Access is denied.".
func (hr HRESULT) Error() string {
	return fmt.Sprintf("HRESULT 0x%08X: %s", uint32(hr), windows.Errno(uint32(hr)).Error())
}

// Is lets errors.Is match a FACILITY_WIN32 HRESULT (0x8007xxxx) against the
// windows.Errno it wraps, so errors.Is(err, windows.ERROR_ACCESS_DENIED)
// is true for E_ACCESSDENIED.
func (hr HRESULT) Is(target error) bool {
	if errno, ok := target.(windows.Errno); ok {
		return uint32(hr)>>16 == 0x8007 && uint32(hr)&0xFFFF == uint32(errno)
	}
	return false
}

// Common HRESULT values. Failure codes are conventionally written in hex
// (0x8000…); HRESULT is signed, so the constants subtract 1<<32 to express
// the same 32-bit pattern as a negative value.
const (
	S_OK    HRESULT = 0
	S_FALSE HRESULT = 1

	E_NOTIMPL      HRESULT = 0x80004001 - 1<<32
	E_NOINTERFACE  HRESULT = 0x80004002 - 1<<32
	E_POINTER      HRESULT = 0x80004003 - 1<<32
	E_ABORT        HRESULT = 0x80004004 - 1<<32
	E_FAIL         HRESULT = 0x80004005 - 1<<32
	E_UNEXPECTED   HRESULT = 0x8000FFFF - 1<<32
	E_ACCESSDENIED HRESULT = 0x80070005 - 1<<32
	E_HANDLE       HRESULT = 0x80070006 - 1<<32
	E_OUTOFMEMORY  HRESULT = 0x8007000E - 1<<32
	E_INVALIDARG   HRESULT = 0x80070057 - 1<<32
)

// ErrIfFailed converts an HRESULT return value to a Go error: nil when the
// call succeeded (non-negative, including S_FALSE and other informational
// successes), or the typed HRESULT when it failed.
func ErrIfFailed(hresult int32) error {
	if hresult >= 0 {
		return nil
	}
	return HRESULT(hresult)
}
