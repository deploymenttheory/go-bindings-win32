//go:build windows

package main

// osd is a small on-screen display example built on top of the
// generated Win32 bindings. It mirrors the layout from the attached external
// project: a thin example entry point and a local osd package for the windowing
// logic.
//
//	go run ./examples/osd

import (
	"fmt"
	"log"
	"runtime"
	"time"

	wm "github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
	osd "github.com/deploymenttheory/go-bindings-win32/examples/osd/osd"
)

func main() {
	log.Print("osd starting...")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	osd, err := osd.New()
	if err != nil {
		panic(fmt.Errorf("create OSD: %w", err))
	}

	osd.
		SetHeading("Speaker Volume").
		SetTimeout(3)

	go func() {
		for i := 0; i <= 100; i += 1 {
			osd.
				SetMessage(fmt.Sprintf("Speaker Volume: %d%%", i)).
				SetProgress(int32(i))
			time.Sleep(20 * time.Millisecond)
		}
		// wait for automatic closing
		time.Sleep(4000 * time.Millisecond)

		osd.SetHeading("Microphone Volume")
		for i := 0; i <= 100; i += 1 {
			osd.
				SetMessage(fmt.Sprintf(" %3d%%", i)).
				SetProgress(int32(i))
			time.Sleep(20 * time.Millisecond)
		}
		// wait for automatic closing
		time.Sleep(4000 * time.Millisecond)

		// exit the window loop and example
		osd.Destroy(true)
	}()

	// A standard Windows message loop that exits when WM_QUIT is received.
	var message wm.MSG
	for {
		if err := wm.GetMessage(&message, 0, 0, 0); err != nil {
			if message.Message == wm.WM_QUIT {
				break
			}
			panic(fmt.Errorf("message loop: %w", err))
		}
		wm.TranslateMessage(&message)
		wm.DispatchMessage(&message)
	}
	fmt.Println("done.")
} // main()

// End.
