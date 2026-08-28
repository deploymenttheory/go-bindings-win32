//go:build windows

package win32

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestLoaderResolvesSystemProc(t *testing.T) {
	proc := NewDLL("kernel32.dll").NewProc("GetTickCount64")
	if err := proc.Find(); err != nil {
		t.Fatalf("Find(GetTickCount64): %v", err)
	}
	addr := proc.Addr()
	if addr == 0 {
		t.Fatal("Addr returned 0")
	}
	if ticks, _, _ := syscall.SyscallN(addr); ticks == 0 {
		t.Error("GetTickCount64 through the loader returned 0")
	}
}

func TestLoaderMissingExportAndDLL(t *testing.T) {
	err := NewDLL("kernel32.dll").NewProc("NoSuchExport__winmd").Find()
	if err == nil {
		t.Fatal("Find on a missing export succeeded")
	}
	var procErr *ProcError
	if !errors.As(err, &procErr) || procErr.DLL != "kernel32.dll" || procErr.Proc != "NoSuchExport__winmd" {
		t.Errorf("missing export: err = %#v, want *ProcError naming the DLL and export", err)
	}
	if !errors.Is(err, ErrProcNotFound) || errors.Is(err, ErrDLLNotFound) {
		t.Errorf("missing export: errors.Is = (proc %v, dll %v), want (true, false)", errors.Is(err, ErrProcNotFound), errors.Is(err, ErrDLLNotFound))
	}
	const errorProcNotFound = syscall.Errno(127)
	if !errors.Is(err, errorProcNotFound) {
		t.Errorf("missing export does not unwrap to ERROR_PROC_NOT_FOUND: %v", err)
	}

	err = NewDLL("no-such-dll-go-bindings-win32.dll").NewProc("X").Find()
	if err == nil {
		t.Fatal("Find on a missing DLL succeeded")
	}
	if !errors.As(err, &procErr) || procErr.Proc != "" {
		t.Errorf("missing DLL: err = %#v, want *ProcError with empty Proc", err)
	}
	if !errors.Is(err, ErrDLLNotFound) || errors.Is(err, ErrProcNotFound) {
		t.Errorf("missing DLL: errors.Is = (dll %v, proc %v), want (true, false)", errors.Is(err, ErrDLLNotFound), errors.Is(err, ErrProcNotFound))
	}
}

// TestLoaderAddrPanicsWithProcError proves the panic value is the typed
// error, so a recover handler can errors.As it.
func TestLoaderAddrPanicsWithProcError(t *testing.T) {
	defer func() {
		recovered := recover()
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("Addr panicked with %T, want error", recovered)
		}
		if !errors.Is(err, ErrProcNotFound) {
			t.Fatalf("Addr panicked with %v, want ErrProcNotFound", err)
		}
	}()
	NewDLL("kernel32.dll").NewProc("NoSuchExport__winmd").Addr()
	t.Fatal("Addr on a missing export did not panic")
}

// TestLoaderIgnoresWorkingDirectory proves the System32-only policy: a decoy
// DLL planted in the current working directory must never be opened. The
// load has to fail with ERROR_MOD_NOT_FOUND (the search never saw the file);
// ERROR_BAD_EXE_FORMAT would mean the loader opened our garbage decoy.
func TestLoaderIgnoresWorkingDirectory(t *testing.T) {
	const decoy = "go-bindings-win32-decoy.dll"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, decoy), []byte("not a PE file"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	err := NewDLL(decoy).NewProc("X").Find()
	if err == nil {
		t.Fatal("decoy DLL in the working directory was loaded")
	}
	const errorModNotFound = syscall.Errno(126)
	if !errors.Is(err, errorModNotFound) {
		t.Fatalf("load failed with %v, want ERROR_MOD_NOT_FOUND (the decoy must never be opened)", err)
	}
}
