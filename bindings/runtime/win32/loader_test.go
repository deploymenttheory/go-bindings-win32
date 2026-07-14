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
	if err := NewDLL("kernel32.dll").NewProc("NoSuchExport__winmd").Find(); err == nil {
		t.Error("Find on a missing export succeeded")
	}
	if err := NewDLL("no-such-dll-go-bindings-win32.dll").NewProc("X").Find(); err == nil {
		t.Error("Find on a missing DLL succeeded")
	}
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
