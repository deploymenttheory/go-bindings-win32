// Package naming holds the Win32 → Go naming rules shared by all emitters.
package naming

import "strings"

// goReservedWords are identifiers a parameter may not use: Go keywords plus
// predeclared identifiers and names the generated code itself binds (imports
// and locals in generated bodies).
var goReservedWords = map[string]bool{
	// keywords
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
	// predeclared
	"any": true, "bool": true, "byte": true, "error": true, "int": true,
	"int8": true, "int16": true, "int32": true, "int64": true, "rune": true,
	"string": true, "uint": true, "uint8": true, "uint16": true, "uint32": true,
	"uint64": true, "uintptr": true, "float32": true, "float64": true,
	"true": true, "false": true, "nil": true, "len": true, "cap": true,
	"new": true, "make": true, "copy": true, "append": true, "panic": true,
	"recover": true, "print": true, "println": true, "close": true, "delete": true,
	"complex": true, "complex64": true, "complex128": true, "imag": true, "real": true,
	"min": true, "max": true, "clear": true,
	// names bound by generated code
	"unsafe": true, "syscall": true, "win32": true, "err": true, "ret": true,
	"self": true, // COM method receiver
}

// Export makes a metadata identifier usable as an exported Go package-level
// name: leading underscores are trimmed and the first letter is capitalized
// ("select" → "Select", "_TP_POOL" → "TP_POOL"). Case-collapsed collisions
// are caught by the generator's per-package name claims.
func Export(name string) string {
	name = strings.TrimLeft(name, "_")
	if name == "" {
		return "X"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// ParamName escapes a metadata parameter name for use as a Go parameter.
func ParamName(name string) string {
	if name == "" {
		return "param"
	}
	if goReservedWords[name] {
		return name + "_"
	}
	return name
}

// PackagePath converts a short namespace ("System.Threading") to the
// generated package's directory path below the output root
// ("system/threading").
func PackagePath(namespace string) string {
	segments := strings.Split(namespace, ".")
	for i, segment := range segments {
		segments[i] = strings.ToLower(segment)
	}
	return strings.Join(segments, "/")
}

// PackageName returns the Go package name for a namespace: the lowercased
// final segment ("System.Threading" → "threading").
func PackageName(namespace string) string {
	segments := strings.Split(namespace, ".")
	return strings.ToLower(segments[len(segments)-1])
}

// ImportAlias returns the alias generated files use for a cross-namespace
// import. Namespaces can share a leaf name ("Graphics.Printing" /
// "Storage.Xps.Printing"), so the alias joins all segments
// ("storagexpsprinting") when the path has more than one, keeping aliases
// unique per full namespace.
func ImportAlias(namespace string) string {
	segments := strings.Split(namespace, ".")
	if len(segments) == 1 {
		return strings.ToLower(segments[0])
	}
	return strings.ToLower(strings.Join(segments, ""))
}
