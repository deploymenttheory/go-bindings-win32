# osd — on-screen display window

A small Windows-only example that shows how to create a simple translucent OSD window
using the generated Win32 bindings. It mirrors the project layout from the attached
`go-osd` sample: a thin main entry point and a separate `osd` package for the window
logic.

```sh
go run ./examples/osd
```

The sample will open a floating OSD window with a heading, a text line, and a progress
bar, then animate it while a Windows message loop stays alive until the program exits.

## What it demonstrates

- Creating a layered, topmost window with `WS_EX_LAYERED` + `WS_EX_TRANSPARENT`
- Registering a custom window proc and working with `WM_PAINT`/`WM_TIMER`
- Drawing text and a progress bar through the GDI bindings
- Using the generated `ui/windowsandmessaging` APIs in a minimal real-world loop

## Notes

- This sample is Windows-only and intentionally matches the repo’s `//go:build windows`
  convention.
- It is intentionally small and meant as a starting point for your own OSD or desktop overlay project.
