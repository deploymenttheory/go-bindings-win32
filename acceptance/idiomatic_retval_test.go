//go:build windows

package acceptance

import (
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/com/events"
)

// Compile-time assertion that COM [out,retval] elevation produced a
// value-returning getter: IEventClass.Get_EventClassID, whose single
// [out,retval] BSTR parameter is lifted to the first Go return value, so the
// method expression has type func(events.IEventClass) (foundation.BSTR, error).
// (The elevation's runtime path — vtable dispatch, HRESULT→error, out-pointer
// fill — is identical to and covered by TestIdiomaticComStream.)
var _ func(events.IEventClass) (foundation.BSTR, error) = events.IEventClass.Get_EventClassID
