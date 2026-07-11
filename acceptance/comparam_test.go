//go:build windows

package acceptance

import (
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"
)

// Compile-time assertions that COM interface parameters use the generated
// vtable struct pointers directly (no wrapper types):
//
//   - an [out,retval] COM out parameter is elevated to a return value:
//     Folder.Get_ParentFolder() (*Folder, error) — the IShellFolder** out
//     becomes a *Folder return.
//   - an input COM interface parameter takes the struct pointer:
//     IAppVisibility.Advise(*IAppVisibilityEvents, *uint32) error.
//
// The runtime path (vtable dispatch, HRESULT→error, out-pointer fill) is
// covered by TestComStreamRoundTrip.
var (
	_ func(*shell.Folder) (*shell.Folder, error)                            = (*shell.Folder).Get_ParentFolder
	_ func(*shell.IAppVisibility, *shell.IAppVisibilityEvents, *uint32) error = (*shell.IAppVisibility).Advise
)
