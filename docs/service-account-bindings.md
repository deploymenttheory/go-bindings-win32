# Native managed service-account bindings

The ingester corrects four managed-account exports from the metadata's
`NETAPI32.dll` to `LOGONCLI.dll`: Add, Remove, Is and Enumerate. Microsoft's
[Add](https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netaddserviceaccount),
[Remove](https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netremoveserviceaccount)
and [Is](https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netisserviceaccount),
plus [Enumerate](https://learn.microsoft.com/windows/win32/api/lmaccess/nf-lmaccess-netenumerateserviceaccounts),
remarks explicitly identify Logoncli, despite contradictory requirements tables.
Read-only inspection of the installed ActiveDirectory module also identifies
Logoncli for its Add/Remove/Is/Query P/Invokes. Query retains its documented
Netapi32 mapping because its public contract has no Logoncli requirement.
Both DLLs resolve these exports on
the development host; this change follows the documented implementation DLL
rather than claiming Netapi32 forwarding is universally absent.

`NetAddServiceAccountUTF16` supplements the existing string wrapper. It accepts
caller-owned UTF-16 pointers, including a NULL password; it allocates no hidden
string copies. `ServerName` must be NULL and `AccountName` must be a valid
NUL-terminated buffer. Keep all inputs alive until the synchronous call returns,
then erase sensitive buffers. The exact native `foundation.NTSTATUS` is returned;
it must not be treated as a Win32 last-error code. Probe the existing
`Procs.NetAddServiceAccount` export before calling the companion.

The ordinary documented existing-account installation path passes flag1 and a
NULL password. [Install-ADServiceAccount](https://learn.microsoft.com/powershell/module/activedirectory/install-adserviceaccount)
also supports an explicit standalone-account password for an RODC-only site;
read-only module inspection confirms the existing SDK-header flag2 path for that
form. The companion exposes the pointer contract without implementing workflow
or permission decisions. Passing flag0 can create an sMSA directory object;
calling Remove with flag0 can delete one. Binding callers must choose flags
deliberately.

Generator tests check compatibility of the old signature, nullable native
parameters, NTSTATUS preservation, no extra export and no inflated coverage.
Ingest tests check the narrow module correction. The Windows acceptance test
**only calls `Proc.Find`**, never Add, Remove, Is, Query or Enumerate. It performs
no account mutation or local account-state probe. Generation and native test
compilation support amd64/arm64. Live managed-account behavior belongs in a
separately authorized domain-member qualification, not this bindings test.
