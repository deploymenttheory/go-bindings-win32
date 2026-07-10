//go:build windows

package acceptance

import (
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/ui/shell"
)

// Compile-time assertions that COM interface parameters use idiomatic wrapper
// types rather than raw pointers:
//
//   - an [out,retval] COM out parameter is elevated to a wrapper return value:
//     Folder.Get_ParentFolder() (Folder, error) — the raw IShellFolder** out
//     becomes an idiomatic Folder return.
//   - an input COM interface parameter takes the wrapper value:
//     IAppVisibility.Advise(IAppVisibilityEvents, *uint32) error — the raw
//     callback pointer becomes the idiomatic IAppVisibilityEvents wrapper,
//     whose .Raw is forwarded to the vtable call.
//
// The runtime path (Wrap constructors, .Raw forwarding, vtable dispatch,
// HRESULT→error) is covered by TestIdiomaticComStream.
var (
	_ func(shell.Folder) (shell.Folder, error)                            = shell.Folder.Get_ParentFolder
	_ func(shell.IAppVisibility, shell.IAppVisibilityEvents, *uint32) error = shell.IAppVisibility.Advise
)
