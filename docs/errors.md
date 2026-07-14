# Error handling

Win32 reports failure four different ways, and the bindings surface each one
honestly — a function's Go signature tells you which domain it uses.

## The four domains

| Domain | How the API reports failure | Go shape |
|---|---|---|
| **`GetLastError`** | A `BOOL`/handle return plus a thread-local error code you fetch separately. The metadata marks these `SetLastError`. | `BOOL` + `SetLastError` → **`error`**; a value + `SetLastError` → **`(T, error)`**. The error is a `windows.Errno`. |
| **`HRESULT`** | A 32-bit status returned directly; negative = failure. Used by COM and many newer flat APIs. | **`error`** via `win32.ErrIfFailed` (nil when the `HRESULT` is ≥ 0). The error is a `win32.HRESULT`. A curated set of APIs whose success codes matter returns **`(win32.HRESULT, error)`** — see below. |
| **`NTSTATUS`** | A 32-bit status from the native (`ntdll`) layer. | Returned as the typed value; compare against the `STATUS_*` constants. |
| **`NET_API_STATUS`** and other **domain codes** | A `DWORD` return code specific to a subsystem (networking, setup, registry…). No `SetLastError`. | Returned as **`uint32`** (or the typed enum); compare against the subsystem's constants (`NERR_Success`, `ERROR_*`). |

The bindings only lower a return to `error` when the contract actually
supports it (`SetLastError` or `HRESULT`). A `NET_API_STATUS` stays a `uint32`
because that *is* the API's error channel — pretending otherwise would hide
information.

## GetLastError (`SetLastError`)

```go
// BOOL + SetLastError → error only.
if err := threading.SetEvent(handle); err != nil {
	// err is a windows.Errno, e.g. "The handle is invalid."
}

// Handle + SetLastError → (T, error). Failure sentinels come from the
// [InvalidHandleValue] metadata; on failure the handle is the sentinel and
// err is the GetLastError value.
h, err := threading.OpenEvent(access, false, "name")
if err != nil { /* ... */ }
```

You can match specific codes with `errors.Is`:

```go
import "golang.org/x/sys/windows"

if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) { /* ... */ }
```

## HRESULT

Flat functions and COM methods that return `HRESULT` become error-returning:

```go
var stream *systemcom.IStream
if err := structuredstorage.CreateStreamOnHGlobal(0, true, &stream); err != nil {
	return err // a win32.HRESULT, e.g. "HRESULT 0x80070057: The parameter is incorrect."
}
// COM methods likewise return error:
if err := stream.Seek(0, 0, &pos); err != nil { /* ... */ }
```

The failure error is the typed `win32.HRESULT`, so `errors.Is` works against
the runtime's sentinels — and a `FACILITY_WIN32` code matches the
`windows.Errno` it wraps:

```go
if errors.Is(err, win32.E_NOINTERFACE) { /* ... */ }
if errors.Is(err, windows.ERROR_ACCESS_DENIED) { /* matches E_ACCESSDENIED */ }
```

`win32.Succeeded(hr)` and `win32.ErrIfFailed(hr)` are in the runtime.

### Informational successes (S_FALSE)

Most APIs treat every non-negative `HRESULT` alike, so the bindings collapse
them to a nil error. A curated set of APIs distinguishes success codes —
`IXmlReader::Read` returns `S_FALSE` at end of input, the COM enumerator
convention (`IEnum*::Next` / `::Skip`) returns `S_FALSE` for "fewer than
requested", `CoInitializeEx` returns `S_FALSE` when COM was already
initialized. Those return `(win32.HRESULT, error)`: the error is non-nil only
on failure, and the `HRESULT` preserves the success code:

```go
for {
	hr, err := reader.Read(&nodeType)
	if err != nil {
		return err
	}
	if hr == win32.S_FALSE {
		break // end of input
	}
	// process the node
}
```

The set is curated in the generator
(`internal/codegen/emit/raw/informational.go`) because the winmd metadata
carries no attribute for this; additions are welcome where an API's success
codes are documented to matter.

## Domain codes (NET_API_STATUS etc.)

```go
status := netmanagement.NetUserAdd("", 1, buf, &parmErr)
switch status {
case 0:    // NERR_Success
case 5:    // ERROR_ACCESS_DENIED — needs Administrator
case 2224: // NERR_UserExists
}
```

The subsystem's own constants live in the same package
(`netmanagement.NERR_Success`, …), so you compare without importing anything
extra.

## Hooking your logger

There is no global error hook — errors flow back through normal Go return
values, so wrap them where you call:

```go
if err := doThing(); err != nil {
	return fmt.Errorf("provisioning widget %q: %w", name, err)
}
```
