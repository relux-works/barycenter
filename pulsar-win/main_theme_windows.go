//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	uxtheme = windows.NewLazySystemDLL("uxtheme.dll")
	dwmapi  = windows.NewLazySystemDLL("dwmapi.dll")

	pGetSysColor           = user32.NewProc("GetSysColor")
	pSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	pInvalidateRect        = user32.NewProc("InvalidateRect")
	pFillRect              = user32.NewProc("FillRect")
	pCreateSolidBrush      = gdi32.NewProc("CreateSolidBrush")
	pSetBkColor            = gdi32.NewProc("SetBkColor")
	pSetWindowTheme        = uxtheme.NewProc("SetWindowTheme")
	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	spiGetHighContrast = 0x0042
	hcfHighContrastOn  = 0x00000001
	colorHighlight     = 13
	colorBtnFace       = 15
	colorGrayText      = 17
	colorBtnText       = 18
	dwmUseDarkMode     = 20
)

type highContrastW struct {
	cbSize            uint32
	dwFlags           uint32
	lpszDefaultScheme *uint16
}

type windowsThemeResources struct {
	mode                     WindowsVisualMode
	palette                  WindowsThemePalette
	backgroundBrush, surface windows.Handle
	button                   windows.Handle
}

func windowsHighContrastEnabled() bool {
	value := highContrastW{cbSize: uint32(unsafe.Sizeof(highContrastW{}))}
	ok, _, _ := pSystemParametersInfoW.Call(spiGetHighContrast, uintptr(value.cbSize), uintptr(unsafe.Pointer(&value)), 0)
	return ok != 0 && value.dwFlags&hcfHighContrastOn != 0
}

func windowsAppsUseLightTheme() *uint32 {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return nil
	}
	result := uint32(value)
	return &result
}

func systemWindowsThemePalette() WindowsThemePalette {
	color := func(index uintptr) uint32 {
		value, _, _ := pGetSysColor.Call(index)
		return uint32(value)
	}
	return WindowsThemePalette{
		Background: color(colorWindow), Surface: color(colorWindow), Text: color(8),
		SecondaryText: color(colorGrayText), Border: color(colorHighlight),
		Button: color(colorBtnFace), ButtonText: color(colorBtnText),
	}
}

func (ctx *mainWindowCtx) releaseTheme() {
	for _, brush := range []windows.Handle{ctx.theme.backgroundBrush, ctx.theme.surface, ctx.theme.button} {
		if brush != 0 {
			pDeleteObject.Call(uintptr(brush))
		}
	}
	ctx.theme = windowsThemeResources{}
}

func (ctx *mainWindowCtx) updateTheme() {
	mode := resolveWindowsVisualMode(windowsHighContrastEnabled(), windowsAppsUseLightTheme())
	palette := windowsThemePalette(mode)
	if mode == WindowsVisualHighContrast {
		palette = systemWindowsThemePalette()
	}
	if ctx.theme.backgroundBrush != 0 && ctx.theme.mode == mode && ctx.theme.palette == palette {
		return
	}
	ctx.releaseTheme()
	background, _, _ := pCreateSolidBrush.Call(uintptr(palette.Background))
	surface, _, _ := pCreateSolidBrush.Call(uintptr(palette.Surface))
	button, _, _ := pCreateSolidBrush.Call(uintptr(palette.Button))
	ctx.theme = windowsThemeResources{
		mode: mode, palette: palette,
		backgroundBrush: windows.Handle(background), surface: windows.Handle(surface), button: windows.Handle(button),
	}

	dark := uint32(0)
	if mode == WindowsVisualDark {
		dark = 1
	}
	pDwmSetWindowAttribute.Call(uintptr(ctx.hwnd), dwmUseDarkMode, uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	var theme *uint16
	if mode == WindowsVisualDark {
		// This progressive stock-control theme is available on the supported
		// Windows 10 baseline; custom brushes still keep static/edit surfaces
		// coherent if a future system declines the theme class.
		theme = u16("DarkMode_Explorer")
	}
	for _, control := range ctx.all {
		pSetWindowTheme.Call(uintptr(control), uintptr(unsafe.Pointer(theme)), 0)
	}
	pInvalidateRect.Call(uintptr(ctx.hwnd), 0, 1)
}

func (ctx *mainWindowCtx) surfaceControl(control windows.Handle) bool {
	if control == ctx.banner || control == ctx.footer {
		return true
	}
	for _, card := range ctx.cards {
		if control == card {
			return true
		}
	}
	return false
}

func (ctx *mainWindowCtx) controlColors(deviceContext uintptr, control windows.Handle, editable, button bool) uintptr {
	palette := ctx.theme.palette
	background := palette.Background
	foreground := palette.Text
	brush := ctx.theme.backgroundBrush
	if editable || ctx.surfaceControl(control) {
		background = palette.Surface
		brush = ctx.theme.surface
	}
	if button {
		background = palette.Button
		foreground = palette.ButtonText
		brush = ctx.theme.button
	}
	if control == ctx.footer {
		foreground = palette.SecondaryText
	}
	pSetBkMode.Call(deviceContext, transparentBk)
	pSetTextColor.Call(deviceContext, uintptr(foreground))
	pSetBkColor.Call(deviceContext, uintptr(background))
	return uintptr(brush)
}

func (ctx *mainWindowCtx) eraseBackground(deviceContext uintptr) bool {
	if ctx.theme.backgroundBrush == 0 {
		return false
	}
	var rect winRect
	pGetClientRect.Call(uintptr(ctx.hwnd), uintptr(unsafe.Pointer(&rect)))
	pFillRect.Call(deviceContext, uintptr(unsafe.Pointer(&rect)), uintptr(ctx.theme.backgroundBrush))
	return true
}
