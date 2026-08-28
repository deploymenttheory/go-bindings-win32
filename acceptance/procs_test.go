//go:build windows

package acceptance

import (
	"errors"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// TestProcsProbeTable drives the generated availability-probe table: a
// present export resolves, and the table entry is the very proc the
// function dispatches through (same DLL and export name).
func TestProcsProbeTable(t *testing.T) {
	proc := threading.Procs.CreateEvent
	if err := proc.Find(); err != nil {
		t.Fatalf("Procs.CreateEvent.Find(): %v", err)
	}
	if proc.Name() != "CreateEventW" || proc.DLLName() == "" {
		t.Errorf("Procs.CreateEvent = %s!%s, want KERNEL32!CreateEventW", proc.DLLName(), proc.Name())
	}
	// Probing must not panic for any entry, present or not.
	if err := threading.Procs.GetCurrentThreadStackLimits.Find(); err != nil && !errors.Is(err, win32.ErrProcNotFound) {
		t.Errorf("Procs.GetCurrentThreadStackLimits.Find() = %v, want nil or ErrProcNotFound", err)
	}
}
