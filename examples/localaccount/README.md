# localaccount — local user account lifecycle

A runnable program that drives a **Windows local user account** through its
full lifecycle using the generated Win32 bindings: create, query, enumerate,
and delete. Everything runs against `netapi32.dll`'s
`NetworkManagement.NetManagement` surface.

## What it shows

| Step | Win32 API | Symbol source |
|---|---|---|
| Assemble the account definition | `USER_INFO_1` struct + `USER_PRIV_USER`, `UF_SCRIPT` | `bindings/win32/networkmanagement/netmanagement` |
| Fill the struct's `PWSTR` fields | `UTF16Ptr` | **runtime** (`bindings/runtime/win32`) |
| Create the account | `NetUserAdd(server, level, …)` | `bindings/win32/networkmanagement/netmanagement` |
| Read it back | `NetUserGetInfo` + `NetApiBufferFree` | `bindings/win32/networkmanagement/netmanagement` |
| List local accounts | `NetUserEnum` (level 0) | `bindings/win32/networkmanagement/netmanagement` |
| Delete it | `NetUserDel(server, name)` | `bindings/win32/networkmanagement/netmanagement` |

The program imports the generated
`bindings/win32/networkmanagement/netmanagement` package (plus the shared
runtime for UTF-16 conversion). That one package holds everything the example
needs: the improved calls (`NetUserAdd` takes a Go `string` server), the
`USER_INFO_1`/`USER_INFO_0` structs, the `USER_PRIV_*`/`UF_*`/`NERR_*`
constants, and the `NetApiBufferFree` pass-through all live there. The `PWSTR`
struct fields are raw UTF-16 pointers, which the **runtime**'s `UTF16Ptr`
produces.

## Running it

Creating a local account modifies the machine and needs **Administrator**
rights, so mutation is opt-in.

```sh
# Dry run — no changes. Assembles the USER_INFO_1 it would submit and does a
# read-only enumeration of existing accounts. Safe to run as any user.
go run ./examples/localaccount

# Real, self-cleaning lifecycle: create → inspect → delete. Run from an
# elevated (Administrator) prompt.
go run ./examples/localaccount -apply

# Create and inspect, but leave the account in place for you to examine.
go run ./examples/localaccount -apply -keep

# Choose the account name (default: gobindwin-demo-<pid>).
go run ./examples/localaccount -apply -name my-temp-account
```

The `-apply` run always deletes the account it created — even if the
inspection step fails — unless you pass `-keep`. Without elevation it prints a
clear "access denied — run as Administrator" message and exits non-zero.

## Why the return is `uint32`, not `error`

The `NetUser*` functions report failure through a `NET_API_STATUS` return code
(a `DWORD`), not via `GetLastError`. The bindings only lower a return
to `error` when the metadata marks the function `SetLastError`, so these keep
their `uint32` status — which the example compares against `NERR_Success` (0),
`NERR_UserExists` (2224), and `ERROR_ACCESS_DENIED` (5). This is the honest
mapping: the binding surfaces exactly what the API contract provides.
