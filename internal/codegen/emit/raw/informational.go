package rawwin

import (
	"sort"
	"strings"
)

// Informational-success annotations.
//
// Some HRESULT APIs distinguish meaningful success codes: IXmlReader::Read
// returns S_FALSE at end of input, the COM enumerator convention
// (IEnum*::Next / ::Skip) returns S_FALSE for "fewer than requested" /
// "skipped past the end", and CoInitializeEx returns S_FALSE when COM was
// already initialized on the thread. The winmd metadata carries no
// attribute for this, so the set is curated here.
//
// Annotated calls return (win32.HRESULT, error) instead of error: err is
// non-nil exactly when the HRESULT failed, and the HRESULT itself preserves
// S_FALSE and other informational successes. Annotation is skipped (with a
// diagnostic for explicit entries) when the call's status is not the plain
// HRESULT return — e.g. after [out,retval] elevation.

// informationalFunctions lists flat functions, keyed "Namespace.Function"
// (metadata names, namespace without the Windows.Win32. prefix).
var informationalFunctions = map[string]bool{
	"System.Com.CoInitializeEx": true,
}

// informationalComMethods lists COM methods, keyed
// "Namespace.Interface.Method" (metadata names).
var informationalComMethods = map[string]bool{
	"Data.Xml.XmlLite.IXmlReader.Read": true,
}

// isInformationalFunction reports whether the flat function is annotated.
func (g *Generator) isInformationalFunction(namespace, function string) bool {
	key := namespace + "." + function
	if !informationalFunctions[key] {
		return false
	}
	g.informationalMatched[key] = true
	return true
}

// isInformationalComMethod reports whether the COM method is annotated,
// either explicitly or via the uniform COM enumerator convention
// (IEnum*::Next / ::Skip).
func (g *Generator) isInformationalComMethod(namespace, iface, method string) bool {
	if strings.HasPrefix(iface, "IEnum") && (method == "Next" || method == "Skip") {
		return true
	}
	key := namespace + "." + iface + "." + method
	if !informationalComMethods[key] {
		return false
	}
	g.informationalMatched[key] = true
	return true
}

// checkInformationalEntries reports curated entries that matched nothing
// during a full emit — a typo, or a winmd bump renamed the API. The
// diagnostics ratchet then surfaces the stale entry.
func (g *Generator) checkInformationalEntries() {
	var stale []string
	for key := range informationalFunctions {
		if !g.informationalMatched[key] {
			stale = append(stale, key)
		}
	}
	for key := range informationalComMethods {
		if !g.informationalMatched[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		g.diag("informational-success entry %s matched no emitted API (stale entry?)", key)
	}
}
