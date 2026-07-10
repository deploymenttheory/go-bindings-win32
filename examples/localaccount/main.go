// Command localaccount demonstrates the full lifecycle of a Windows local
// user account through the generated Win32 bindings: add (NetUserAdd), query
// (NetUserGetInfo), enumerate (NetUserEnum), and delete (NetUserDel) — all in
// netapi32.dll's NetworkManagement.NetManagement surface.
//
// It uses ONLY the idiomatic package (plus the shared runtime) — the
// idiomatic layer is self-contained: it re-exports the USER_INFO_1 struct,
// the typed constants, and pass-through helpers like NetApiBufferFree, so a
// consumer never imports bindings/win32 directly. The runtime provides the
// UTF-16 conversion the struct's PWSTR fields need.
//
// Creating a local account modifies the system and requires Administrator
// rights, so mutation is gated behind -apply. Without it the program does a
// dry run: it assembles and prints the USER_INFO_1 it *would* submit, plus a
// harmless read-only enumeration of existing accounts. With -apply it runs
// the real, self-cleaning lifecycle (the created account is always deleted,
// even on failure, unless you pass -keep).
//
//	go run ./examples/localaccount                 # dry run (no changes)
//	go run ./examples/localaccount -apply          # create + inspect + delete (admin)
//	go run ./examples/localaccount -apply -keep    # create + inspect, leave it
//
//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/networkmanagement/netmanagement"
)

// Selected NET_API_STATUS / Win32 error codes returned by the NetUser* APIs.
const (
	nerrSuccess        = 0    // NERR_Success
	nerrUserExists     = 2224 // NERR_UserExists
	nerrUserNotFound   = 2221 // NERR_UserNotFound
	errorAccessDenied  = 5    // ERROR_ACCESS_DENIED (need Administrator)
	uf_NORMAL_ACCOUNT  = 512  // UF_NORMAL_ACCOUNT (lives in a separate const group)
	infoLevel1         = 1    // USER_INFO_1
	maxPreferredLength = 0xFFFFFFFF
)

func main() {
	apply := flag.Bool("apply", false, "actually create and delete the account (requires Administrator)")
	keep := flag.Bool("keep", false, "with -apply, do not delete the created account")
	name := flag.String("name", fmt.Sprintf("gobindwin-demo-%d", os.Getpid()), "account name to create")
	flag.Parse()

	// A deliberately ephemeral, disabled-friendly demo account.
	info := netmanagement.USER_INFO_1{
		Usri1_name:     foundation.PWSTR(win32.UTF16Ptr(*name)),
		Usri1_password: foundation.PWSTR(win32.UTF16Ptr("P@ssw0rd-" + randomish())),
		Usri1_priv:     netmanagement.USER_PRIV_USER,
		Usri1_comment:  foundation.PWSTR(win32.UTF16Ptr("Temporary account created by go-bindings-win32 example")),
		Usri1_flags:    netmanagement.UF_SCRIPT | netmanagement.USER_ACCOUNT_FLAGS(uf_NORMAL_ACCOUNT),
	}

	fmt.Printf("account:  %s\n", *name)
	fmt.Printf("priv:     USER_PRIV_USER\n")
	fmt.Printf("flags:    UF_SCRIPT | UF_NORMAL_ACCOUNT (0x%x)\n\n", uint32(info.Usri1_flags))

	// Read-only enumeration works without admin and without mutating anything.
	listLocalAccounts()

	if !*apply {
		fmt.Println("\nDry run: no changes made. Re-run as Administrator with -apply to")
		fmt.Println("create the account, inspect it, and delete it (self-cleaning).")
		return
	}

	if err := createAccount(*name, &info); err != nil {
		fmt.Fprintln(os.Stderr, "\n"+err.Error())
		os.Exit(1)
	}
	fmt.Printf("\ncreated %q\n", *name)

	if !*keep {
		// Always clean up the account we created, even if inspection fails.
		defer func() {
			if status := netmanagement.NetUserDel("", *name); status == nerrSuccess {
				fmt.Printf("deleted %q\n", *name)
			} else {
				fmt.Fprintf(os.Stderr, "cleanup: NetUserDel returned %d\n", status)
			}
		}()
	}

	inspectAccount(*name)
	listLocalAccounts()
}

// createAccount submits the USER_INFO_1 via the idiomatic NetUserAdd (Go
// string server, typed status). Level 1 tells NetUserAdd to read a
// USER_INFO_1; the struct is passed as its raw bytes.
func createAccount(name string, info *netmanagement.USER_INFO_1) error {
	var parmErr uint32
	status := netmanagement.NetUserAdd("", infoLevel1, (*byte)(unsafe.Pointer(info)), &parmErr)
	switch status {
	case nerrSuccess:
		return nil
	case errorAccessDenied:
		return fmt.Errorf("NetUserAdd: access denied — run this from an elevated (Administrator) prompt")
	case nerrUserExists:
		return fmt.Errorf("NetUserAdd: an account named %q already exists", name)
	default:
		return fmt.Errorf("NetUserAdd failed: status %d (invalid parameter index %d)", status, parmErr)
	}
}

// inspectAccount reads the account back with NetUserGetInfo (level 1) and
// prints a couple of fields, showing how to consume a NetApiBuffer result.
func inspectAccount(name string) {
	var buffer *byte
	status := netmanagement.NetUserGetInfo("", name, infoLevel1, &buffer)
	if status != nerrSuccess {
		fmt.Fprintf(os.Stderr, "NetUserGetInfo returned %d\n", status)
		return
	}
	defer netmanagement.NetApiBufferFree(unsafe.Pointer(buffer))

	got := (*netmanagement.USER_INFO_1)(unsafe.Pointer(buffer))
	fmt.Printf("readback: name=%q priv=%d comment=%q\n",
		win32.UTF16ToString((*uint16)(got.Usri1_name)),
		got.Usri1_priv,
		win32.UTF16ToString((*uint16)(got.Usri1_comment)))
}

// listLocalAccounts enumerates level-0 (name-only) local accounts via
// NetUserEnum and prints them. This is read-only and needs no privileges.
func listLocalAccounts() {
	var (
		buffer      *byte
		read, total uint32
		resume      uint32
	)
	status := netmanagement.NetUserEnum("", 0, netmanagement.FILTER_NORMAL_ACCOUNT,
		&buffer, maxPreferredLength, &read, &total, &resume)
	if status != nerrSuccess || buffer == nil {
		fmt.Fprintf(os.Stderr, "NetUserEnum returned %d\n", status)
		return
	}
	defer netmanagement.NetApiBufferFree(unsafe.Pointer(buffer))

	// Level 0 is an array of USER_INFO_0 { PWSTR usri0_name }.
	names := unsafe.Slice((*netmanagement.USER_INFO_0)(unsafe.Pointer(buffer)), read)
	fmt.Printf("local accounts (%d):", read)
	for i := range names {
		fmt.Printf(" %s", win32.UTF16ToString((*uint16)(names[i].Usri0_name)))
	}
	fmt.Println()
}

// randomish returns a short, dependency-free varying suffix for the demo
// password so repeated runs do not reuse an identical value.
func randomish() string {
	const digits = "0123456789abcdef"
	pid := os.Getpid()
	return string([]byte{digits[pid&0xf], digits[(pid>>4)&0xf], digits[(pid>>8)&0xf], 'X', '!'})
}
