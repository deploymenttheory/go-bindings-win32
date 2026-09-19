//go:build windows

// osd is a small on-screen display example built on top of pure Win32 bindings that can
// be used as a brief example on how to implement the windowing logic.

package osd

import (
	"syscall"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/dwm"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/gdi"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/libraryloader"
	wm "github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

const (
	windowClass  = "GoOSDWindow"
	windowWidth  = 284
	windowHeight = 80
	timerID      = uintptr(1)
)

// OSD represents an overlay window for transient information.
type OSD struct {
	hwnd        foundation.HWND
	instance    foundation.HINSTANCE
	proc        uintptr
	isOpen      bool
	enforceQuit bool

	headingFont gdi.HFONT
	headingText string

	textFont gdi.HFONT
	text     string
	progress int32

	color foundation.COLORREF

	workingarea foundation.RECT

	timeoutSecs   int
	activeTimerID uintptr
}

// Create a new OSD window.
func New() (*OSD, error) {
	instance, err := libraryloader.GetModuleHandle(nil)
	if err != nil {
		return nil, err
	}

	// OSD to be returned
	this := &OSD{
		instance:    foundation.HINSTANCE(instance),
		headingText: "<heading>",
		text:        "<text>",
	}
	if err := this.defineLayout(); err != nil {
		return nil, err
	}

	this.proc = syscall.NewCallback(this.windowProc)

	className := win32.UTF16Ptr(windowClass)
	class := wm.WNDCLASSW{
		LpfnWndProc:   wm.WNDPROC(this.proc),
		HInstance:     this.instance,
		LpszClassName: className,
	}
	if _, err := wm.RegisterClass(&class); err != nil {
		return nil, err
	}

	classNameString := windowClass
	this.hwnd, err = wm.CreateWindowEx(
		wm.WS_EX_TOPMOST|wm.WS_EX_LAYERED|wm.WS_EX_TRANSPARENT|wm.WS_EX_NOACTIVATE,
		&classNameString,
		nil,
		wm.WS_POPUP,
		this.workingarea.Right-windowWidth-12,
		this.workingarea.Bottom-windowHeight-12,
		windowWidth,
		windowHeight,
		0,
		0,
		this.instance,
		nil,
	)
	if err != nil {
		return nil, err
	}

	if err := wm.SetLayeredWindowAttributes(this.hwnd, 0, 235, wm.LWA_ALPHA); err != nil {
		return nil, err
	}

	_ = dwm.DwmSetWindowAttribute(
		this.hwnd,
		uint32(dwm.DWMWA_WINDOW_CORNER_PREFERENCE),
		[]byte{byte(dwm.DWMWCP_ROUNDSMALL), 0, 0, 0},
	)

	this.isOpen = false
	return this, nil
}

// defineLayout loads the desktop metrics used to position the OSD.
func (this *OSD) defineLayout() error {
	metrics := wm.NONCLIENTMETRICSW{
		CbSize: uint32(unsafe.Sizeof(wm.NONCLIENTMETRICSW{})),
	}
	if err := wm.SystemParametersInfo(
		wm.SPI_GETNONCLIENTMETRICS,
		uint32(unsafe.Sizeof(metrics)),
		unsafe.Pointer(&metrics), 0,
	); err != nil {
		return err
	}

	fontFace := "Segoe UI"

	this.headingFont = gdi.CreateFont(-20,
		0, 0, 0, int32(gdi.FW_DEMIBOLD),
		0, 0, 0,
		uint32(gdi.DEFAULT_CHARSET),
		uint32(gdi.OUT_DEFAULT_PRECIS),
		uint32(gdi.CLIP_DEFAULT_PRECIS),
		uint32(gdi.DEFAULT_QUALITY),
		uint32(gdi.DEFAULT_PITCH)|uint32(gdi.FF_DONTCARE),
		&fontFace,
	)

	this.textFont = gdi.CreateFont(-14,
		0, 0, 0, int32(gdi.FW_NORMAL),
		0, 0, 0,
		uint32(gdi.DEFAULT_CHARSET),
		uint32(gdi.OUT_DEFAULT_PRECIS),
		uint32(gdi.CLIP_DEFAULT_PRECIS),
		uint32(gdi.DEFAULT_QUALITY),
		uint32(gdi.DEFAULT_PITCH)|uint32(gdi.FF_DONTCARE),
		&fontFace,
	)

	this.color = foundation.COLORREF(gdi.GetSysColor(gdi.COLOR_WINDOWTEXT))
	_ = wm.SystemParametersInfo(wm.SPI_GETWORKAREA, 0, unsafe.Pointer(&this.workingarea), 0)
	return nil
}

// startTimer starts or restarts the timer for hiding the osd window.
func (this *OSD) startTimer() {
	if this.hwnd != 0 {
		if this.activeTimerID != 0 {
			wm.KillTimer(this.hwnd, this.activeTimerID)
			this.activeTimerID = 0
		}

		if this.timeoutSecs > 0 {
			this.activeTimerID, _ = wm.SetTimer(this.hwnd, timerID, uint32(this.timeoutSecs)*1000, 0)
		}
	}
}

// stopTimer disables the hide timer.
func (this *OSD) stopTimer() {
	if this.hwnd != 0 && this.activeTimerID != 0 {
		wm.KillTimer(this.hwnd, this.activeTimerID)
		this.activeTimerID = 0
	}
}

func (this *OSD) invalidate() {
	if this.hwnd != 0 {
		gdi.InvalidateRect(this.hwnd, nil, true)
	}
}

// ===== Public OSD functions =====

// SetHeading updates the heading and shows the window.
func (this *OSD) SetHeading(title string) *OSD {
	this.headingText = title
	if !this.isOpen {
		wm.ShowWindow(this.hwnd, wm.SW_SHOW)
	}
	this.invalidate()
	this.startTimer()
	return this
}

// SetMessage updates the message text and shows the window.
func (this *OSD) SetMessage(text string) *OSD {
	this.text = text
	if !this.isOpen {
		wm.ShowWindow(this.hwnd, wm.SW_SHOW)
	}
	this.invalidate()
	this.startTimer()
	return this
}

// SetProgress updates the progress value and shows the window.
func (this *OSD) SetProgress(n int32) *OSD {
	this.progress = n
	if !this.isOpen {
		wm.ShowWindow(this.hwnd, wm.SW_SHOW)
	}
	this.invalidate()
	this.startTimer()
	return this
}

// SetTimeout sets how long the OSD stays visible.
func (this *OSD) SetTimeout(secs int) *OSD {
	this.timeoutSecs = secs
	this.startTimer()
	return this
}

// Close hides the OSD window programatically.
func (this *OSD) Close() error {
	if this.isOpen {
		return wm.PostMessage(this.hwnd, wm.WM_CLOSE, 0, 0)
	}
	return nil
}

// Destroy stops the OSD and optionally quits the app message loop.
func (this *OSD) Destroy(withQuit bool) {
	if this.hwnd != 0 {
		this.enforceQuit = withQuit
		_ = wm.PostMessage(this.hwnd, wm.WM_DESTROY, 0, 0)
	}
}

// windowProc handles Windows messages for the OSD display window.
func (this *OSD) windowProc(hwnd foundation.HWND, message uint32, wParam foundation.WPARAM, lParam foundation.LPARAM) uintptr {
	switch message {
	case wm.WM_PAINT:
		var paint gdi.PAINTSTRUCT
		dc := gdi.BeginPaint(hwnd, &paint)
		var clientRect foundation.RECT

		wm.GetClientRect(hwnd, &clientRect)
		backgroundBrush := gdi.GetSysColorBrush(gdi.COLOR_WINDOW)
		gdi.FillRect(dc, &clientRect, backgroundBrush)
		gdi.SetBkMode(dc, int32(gdi.TRANSPARENT))
		gdi.SetTextColor(dc, this.color)

		gdi.SelectObject(dc, gdi.HGDIOBJ(this.headingFont))
		gdi.DrawTextEx(dc,
			win32.UTF16Ptr(this.headingText), int32(len(this.headingText)),
			&foundation.RECT{Left: 12, Top: 12, Right: 272, Bottom: 48},
			gdi.DT_TOP+gdi.DT_LEFT, nil,
		)

		if this.progress >= 0 {
			progressRect := foundation.RECT{
				Left:   12,
				Top:    48,
				Right:  272,
				Bottom: 68,
			}

			trackBrush := gdi.CreateSolidBrush(foundation.COLORREF(0x00D0eeD0))
			gdi.FillRect(dc, &progressRect, trackBrush)
			gdi.DeleteObject(gdi.HGDIOBJ(trackBrush))

			progressRect.Right = progressRect.Left + ((progressRect.Right-progressRect.Left)*this.progress)/100
			completedBrush := gdi.CreateSolidBrush(foundation.COLORREF(0x00D0d0D0))
			gdi.FillRect(dc, &progressRect, completedBrush)
			gdi.DeleteObject(gdi.HGDIOBJ(completedBrush))
		}

		gdi.SelectObject(dc, gdi.HGDIOBJ(this.textFont))
		gdi.DrawTextEx(dc,
			win32.UTF16Ptr(this.text), int32(len(this.text)),
			&foundation.RECT{Left: 12, Top: 48, Right: 272, Bottom: 68},
			gdi.DT_TOP+gdi.DT_LEFT, nil,
		)

		gdi.EndPaint(hwnd, &paint)
		return 0

	case wm.WM_TIMER:
		if uintptr(wParam) == this.activeTimerID {
			this.stopTimer()
			wm.ShowWindow(hwnd, wm.SW_HIDE)
		}
		return 0

	case wm.WM_DESTROY:
		wm.CloseWindow(this.hwnd)
		this.isOpen = false
		this.hwnd = 0
		if this.enforceQuit {
			wm.PostQuitMessage(0)
		}
		return 0

	default:
		return uintptr(wm.DefWindowProc(hwnd, message, wParam, lParam))
	}
}
