// Command volume prints the current output device with volume and mute status
// gathered through Core Audio APIs bindings.
//
// It shows 
// - how to handle WSTR Variant return values 
// - how to handle COM interfaces that are returned as IUnknown and need to be cast to the correct interface type
// - how to handle COM initialization and apartment threading
//   TODO: missing constants in the bindings namespace for CoInitializeEx flags
// - define CLSID_MMDeviceEnumerator, as it is missing in the media/audio namespace but exists in
//   https://github.com/microsoft/win32metadata/blob/29896383c51d9dd6a2ea0ec6304d095baca9418c/generation/WinSDK/RecompiledIdlHeaders/um/mmdeviceapi.idl#L488
//   TODO: Why is this missing?
// - investigate into combining foundation.PROPERTYKEY and foundation.DEVPROPKEY, is the same ?
//
//	go run ./examples/volume
//
//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/devices/properties"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/media/audio"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/media/audio/endpoints"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell/propertiessystem"
)

const (
	COINIT_APARTMENTTHREADED = 0x2
	COINIT_MULTITHREADED     = 0x0
	COINIT_DISABLE_OLE1DDE   = 0x4
	COINIT_SPEED_OVER_MEMORY = 0x8
)

var CLSID_MMDeviceEnumerator win32.GUID = win32.GUID{Data1: 0xbcde0395, Data2: 0xe52f, Data3: 0x467c, Data4: [8]byte{0x8e, 0x3d, 0xc4, 0x57, 0x92, 0x91, 0x69, 0x2e}}

func CreateDeviceEnumerator() (*audio.IMMDeviceEnumerator, error) {
	var out *win32.IUnknown
	if err := com.CoCreateInstance(&CLSID_MMDeviceEnumerator, nil, com.CLSCTX_ALL, &audio.IID_IMMDeviceEnumerator, &out); err != nil {
		return nil, err
	}
	deviceEnum := win32.Cast[audio.IMMDeviceEnumerator](out)
	return deviceEnum, nil
}

func main() {
	fmt.Println("report actual master volume")
	fmt.Println("---------------------------")

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if hr, err := com.CoInitializeEx(COINIT_APARTMENTTHREADED); err != nil {
		log.Fatalf("CoInitializeEx: thread already in an incompatible apartment: hr=%d %v", hr, err)
	}
	defer com.CoUninitialize()

	var deviceEnum, _ = CreateDeviceEnumerator()
	defer deviceEnum.Release()

	var mmd *audio.IMMDevice
	if err := deviceEnum.GetDefaultAudioEndpoint(audio.ERender, audio.EConsole, &mmd); err != nil {
		log.Fatalf("GetDefaultAudioEndpoint: err=%v", err)
	}
	defer mmd.Release()

	var ps *propertiessystem.IPropertyStore
	if err := mmd.OpenPropertyStore(com.STGM_READ, &ps); err != nil {
		log.Fatalf("OpenPropertyStore: err=%v", err)
	}
	defer ps.Release()

	var pk_fn = foundation.PROPERTYKEY{
		Fmtid: properties.DEVPKEY_Device_FriendlyName.Fmtid,
		Pid:   properties.DEVPKEY_Device_FriendlyName.Pid,
	}
	var pv structuredstorage.PROPVARIANT
	if err := ps.GetValue(&pk_fn, &pv); err != nil {
		log.Fatalf("GetValue: err=%v", err)
	}

	// The friendly name is returned as VT_LPWSTR. The first union slot holds
	// the pointer to its UTF-16 buffer.
	namePtr := *(*uintptr)(unsafe.Pointer(&pv.Anonymous.Data[1]))
	deviceName := win32.UTF16ToString((*uint16)(unsafe.Pointer(namePtr)))
	fmt.Printf("Default playback device: %s\n", deviceName)

	// var asm2Unknown *win32.IUnknown
	// if err := mmd.Activate(&audio.IID_IAudioSessionManager2, com.CLSCTX_ALL, nil, &asm2Unknown); err != nil {
	// 	log.Fatalf("GetValue: err=%v", err)
	// }
	// asm2 := win32.Cast[audio.IAudioSessionManager2](asm2Unknown)
	// defer asm2.Release()

	var aevUnknown *win32.IUnknown
	if err := mmd.Activate(&endpoints.IID_IAudioEndpointVolume, com.CLSCTX_ALL, nil, &aevUnknown); err != nil {
		log.Fatalf("Activate AudioEndpointVolume: err=%v", err)
	}
	aev := win32.Cast[endpoints.IAudioEndpointVolume](aevUnknown)
	defer aev.Release()

	var vol float32
	aev.GetMasterVolumeLevelScalar(&vol)
	fmt.Printf("  Current Volume: %d\n", int(vol * 100))

	var isMute foundation.BOOL
	aev.GetMute(&isMute)
	fmt.Printf("  Current Mute: %d\n", isMute)

	fmt.Printf("done.\n")


}
