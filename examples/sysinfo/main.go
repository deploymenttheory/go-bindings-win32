// Command sysinfo prints host information gathered through the generated
// Win32 bindings — computer name, current user, processor topology, physical
// memory, and the (manifest-dependent) reported OS version.
//
// Everything it does is read-only and needs no privileges, so it is the
// gentlest possible tour of the bindings. It imports ONLY the idiomatic
// packages (plus the shared runtime for UTF-16 conversion) — never
// bindings/win32.
//
// It shows three recurring Win32 patterns the bindings surface as-is:
//   - "size probe then fill": call a string API once with a nil buffer to
//     learn the required length, allocate, then call again (GetComputerNameEx,
//     GetUserName).
//   - a struct with a self-size field you must set before the call
//     (MEMORYSTATUSEX.DwLength, OSVERSIONINFOW.DwOSVersionInfoSize).
//   - a value struct filled by a void function (SYSTEM_INFO via
//     GetNativeSystemInfo), including a field that is a C union.
//
//	go run ./examples/sysinfo
//
//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/foundation"
	sysinfo "github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/systeminformation"
	"github.com/deploymenttheory/go-bindings-win32/opinionated/idiomatic/win32/system/windowsprogramming"
)

func main() {
	fmt.Println("host information (via go-bindings-win32)")
	fmt.Println("----------------------------------------")

	if host, err := computerName(sysinfo.ComputerNameDnsHostname); err == nil {
		fmt.Printf("hostname:   %s\n", host)
	} else {
		fmt.Printf("hostname:   (error: %v)\n", err)
	}

	if user, err := userName(); err == nil {
		fmt.Printf("user:       %s\n", user)
	} else {
		fmt.Printf("user:       (error: %v)\n", err)
	}

	printProcessorInfo()
	printMemory()
	printVersion()
}

// computerName reads a computer name using the classic size-probe-then-fill
// pattern: the first call with a nil buffer fails with ERROR_MORE_DATA and
// writes the required length (including the terminating NUL) into size.
func computerName(kind sysinfo.COMPUTER_NAME_FORMAT) (string, error) {
	var size uint32
	_ = sysinfo.GetComputerNameEx(kind, nil, &size) // probe: expected to fail, sets size
	if size == 0 {
		return "", fmt.Errorf("GetComputerNameEx reported zero length")
	}
	buffer := make([]uint16, size)
	if err := sysinfo.GetComputerNameEx(kind, foundation.PWSTR(&buffer[0]), &size); err != nil {
		return "", err
	}
	return win32.UTF16ToString(&buffer[0]), nil
}

// userName reads the current user via GetUserName (also size-probe-then-fill;
// its count is in characters, including the NUL).
func userName() (string, error) {
	var size uint32
	_ = windowsprogramming.GetUserName(nil, &size) // probe
	if size == 0 {
		return "", fmt.Errorf("GetUserName reported zero length")
	}
	buffer := make([]uint16, size)
	if err := windowsprogramming.GetUserName(foundation.PWSTR(&buffer[0]), &size); err != nil {
		return "", err
	}
	return win32.UTF16ToString(&buffer[0]), nil
}

// printProcessorInfo fills a SYSTEM_INFO via GetNativeSystemInfo (a void
// function that writes into the struct you hand it). The leading field is a C
// union whose low 16 bits hold the processor architecture.
func printProcessorInfo() {
	var info sysinfo.SYSTEM_INFO
	sysinfo.GetNativeSystemInfo(&info)

	fmt.Printf("cpus:       %d logical\n", info.DwNumberOfProcessors)
	fmt.Printf("page size:  %d bytes (allocation granularity %d)\n",
		info.DwPageSize, info.DwAllocationGranularity)
	// The leading union is exposed as a correctly sized backing blob; its
	// low 16 bits hold wProcessorArchitecture (the dwOemId overlay).
	fmt.Printf("arch:       %s\n", processorArch(uint16(info.Anonymous.Data[0]&0xFFFF)))
}

// processorArch decodes wProcessorArchitecture (PROCESSOR_ARCHITECTURE_*).
func processorArch(code uint16) string {
	switch code {
	case 9:
		return "x64 (AMD64)"
	case 12:
		return "ARM64"
	case 5:
		return "ARM"
	case 0:
		return "x86"
	default:
		return fmt.Sprintf("unknown (%d)", code)
	}
}

// printMemory fills MEMORYSTATUSEX. Its DwLength self-size field must be set
// to sizeof(struct) before the call, or the API rejects it.
func printMemory() {
	var status sysinfo.MEMORYSTATUSEX
	status.DwLength = uint32(unsafe.Sizeof(status))
	if err := sysinfo.GlobalMemoryStatusEx(&status); err != nil {
		fmt.Printf("memory:     (error: %v)\n", err)
		return
	}
	const mib = 1 << 20
	fmt.Printf("memory:     %d MiB total, %d MiB free (%d%% in use)\n",
		status.UllTotalPhys/mib, status.UllAvailPhys/mib, status.DwMemoryLoad)
}

// printVersion reads the OS version via GetVersionEx. Note the classic Win32
// gotcha: without an application compatibility manifest, Windows shims this to
// report 6.2 (Windows 8) on Windows 10/11. It is included here precisely to
// demonstrate the struct-with-self-size pattern and that caveat — for a true
// build number, read the registry (see the localaccount example's siblings).
func printVersion() {
	var version sysinfo.OSVERSIONINFOW
	version.DwOSVersionInfoSize = uint32(unsafe.Sizeof(version))
	if err := sysinfo.GetVersionEx(&version); err != nil {
		fmt.Printf("version:    (error: %v)\n", err)
		return
	}
	fmt.Printf("version:    %d.%d build %d (GetVersionEx; manifest-dependent)\n",
		version.DwMajorVersion, version.DwMinorVersion, version.DwBuildNumber)
}
