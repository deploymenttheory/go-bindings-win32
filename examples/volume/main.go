//go:build windows

// Command volume reports the default playback device with its master volume
// and mute state, through the generated Core Audio bindings.
//
// It is the COM counterpart to the other examples: an apartment, a coclass
// created by CLSID, interfaces obtained as IUnknown and cast to their real
// type, a property store read through a PROPVARIANT, and a device-scoped
// interface activated off the endpoint.
//
//	go run ./examples/volume
package main

import (
	"fmt"
	"log"
	"runtime"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/devices/functiondiscovery"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/media/audio"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/media/audio/endpoints"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com/structuredstorage"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/variant"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell/propertiessystem"
)

func main() {
	fmt.Println("default playback device")
	fmt.Println("-----------------------")

	// An apartment belongs to a thread, so pin this goroutine to the one it
	// is entered on. CoInitializeEx is an informational-success API: S_FALSE
	// means this thread was already in a compatible apartment, which is a
	// success, so the HRESULT is returned alongside the error.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if _, err := com.CoInitializeEx(com.COINIT_APARTMENTTHREADED); err != nil {
		log.Fatalf("CoInitializeEx: %v", err)
	}
	defer com.CoUninitialize()

	device, err := defaultRenderDevice()
	if err != nil {
		log.Fatalf("default render device: %v", err)
	}
	defer device.Release()

	name, err := friendlyName(device)
	if err != nil {
		log.Fatalf("friendly name: %v", err)
	}
	fmt.Printf("name:       %s\n", name)

	level, muted, err := masterVolume(device)
	if err != nil {
		log.Fatalf("master volume: %v", err)
	}
	fmt.Printf("volume:     %.0f%%\n", level*100)
	fmt.Printf("muted:      %t\n", muted)
}

// defaultRenderDevice creates the device enumerator and asks it for the
// endpoint the system plays through.
//
// MMDeviceEnumerator is a coclass: it has no layout of its own, so it exists
// in the bindings only as its CLSID. CoCreateInstance hands the new object
// back as *win32.IUnknown — the shape of every riid-selected out-param —
// which win32.Cast reinterprets as the interface the riid asked for. No
// AddRef happens: the reference the factory handed out is now held through
// the typed pointer.
func defaultRenderDevice() (*audio.IMMDevice, error) {
	var unknown *win32.IUnknown
	if err := com.CoCreateInstance(&audio.CLSID_MMDeviceEnumerator, nil,
		com.CLSCTX_ALL, &audio.IID_IMMDeviceEnumerator, &unknown); err != nil {
		return nil, fmt.Errorf("CoCreateInstance(MMDeviceEnumerator): %w", err)
	}
	enumerator := win32.Cast[audio.IMMDeviceEnumerator](unknown)
	defer enumerator.Release()

	var device *audio.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(audio.ERender, audio.EConsole, &device); err != nil {
		return nil, fmt.Errorf("GetDefaultAudioEndpoint: %w", err)
	}
	return device, nil
}

// friendlyName reads the device's display name out of its property store.
//
// A PROPVARIANT is a tagged union: the whole struct is one anonymous union,
// whose first overlay is the struct carrying the vt discriminant. Each union
// member has a typed accessor, so the discriminant is a real VARENUM and the
// payload arrives as its own type — no unsafe.Pointer arithmetic over the
// backing bytes.
//
// The variant owns whatever it points at, so it must be cleared.
func friendlyName(device *audio.IMMDevice) (string, error) {
	var store *propertiessystem.IPropertyStore
	if err := device.OpenPropertyStore(com.STGM_READ, &store); err != nil {
		return "", fmt.Errorf("OpenPropertyStore: %w", err)
	}
	defer store.Release()

	var value structuredstorage.PROPVARIANT
	if err := store.GetValue(&functiondiscovery.PKEY_Device_FriendlyName, &value); err != nil {
		return "", fmt.Errorf("GetValue(PKEY_Device_FriendlyName): %w", err)
	}
	defer structuredstorage.PropVariantClear(&value)

	tagged := value.Anonymous.Anonymous()
	if tagged.Vt != variant.VT_LPWSTR {
		return "", fmt.Errorf("friendly name is %v, want VT_LPWSTR", tagged.Vt)
	}
	return win32.UTF16ToString((*uint16)(*tagged.Anonymous.PwszVal())), nil
}

// masterVolume activates the endpoint-volume interface on the device and
// reads the current scalar level and mute flag.
//
// IMMDevice::Activate is another riid-selected out-param, so it too yields
// *win32.IUnknown. GetMute's [out] BOOL is exposed as *bool.
func masterVolume(device *audio.IMMDevice) (level float32, muted bool, err error) {
	var unknown *win32.IUnknown
	if err := device.Activate(&endpoints.IID_IAudioEndpointVolume,
		com.CLSCTX_ALL, nil, &unknown); err != nil {
		return 0, false, fmt.Errorf("Activate(IAudioEndpointVolume): %w", err)
	}
	volume := win32.Cast[endpoints.IAudioEndpointVolume](unknown)
	defer volume.Release()

	if err := volume.GetMasterVolumeLevelScalar(&level); err != nil {
		return 0, false, fmt.Errorf("GetMasterVolumeLevelScalar: %w", err)
	}
	if err := volume.GetMute(&muted); err != nil {
		return 0, false, fmt.Errorf("GetMute: %w", err)
	}
	return level, muted, nil
}
