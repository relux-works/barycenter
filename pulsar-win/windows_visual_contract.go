package main

import "math"

type WindowsVisualMode uint8

const (
	WindowsVisualLight WindowsVisualMode = iota
	WindowsVisualDark
	WindowsVisualHighContrast
)

type WindowsThemePalette struct {
	Background, Surface, Text, SecondaryText, Border, Button, ButtonText uint32
}

type WindowsShellMetrics struct {
	Margin, Gutter, SidebarWidth, HeaderHeight, ButtonHeight, CardHeight int
}

func windowsRGB(red, green, blue uint8) uint32 {
	// COLORREF stores bytes as 0x00BBGGRR.
	return uint32(red) | uint32(green)<<8 | uint32(blue)<<16
}

func resolveWindowsVisualMode(highContrast bool, appsUseLightTheme *uint32) WindowsVisualMode {
	if highContrast {
		return WindowsVisualHighContrast
	}
	if appsUseLightTheme != nil && *appsUseLightTheme == 0 {
		return WindowsVisualDark
	}
	return WindowsVisualLight
}

func windowsThemePalette(mode WindowsVisualMode) WindowsThemePalette {
	if mode == WindowsVisualDark {
		return WindowsThemePalette{
			Background: windowsRGB(32, 32, 32), Surface: windowsRGB(43, 43, 43),
			Text: windowsRGB(245, 245, 245), SecondaryText: windowsRGB(200, 200, 200),
			Border: windowsRGB(78, 78, 78), Button: windowsRGB(52, 52, 52), ButtonText: windowsRGB(245, 245, 245),
		}
	}
	return WindowsThemePalette{
		Background: windowsRGB(243, 243, 243), Surface: windowsRGB(255, 255, 255),
		Text: windowsRGB(28, 28, 28), SecondaryText: windowsRGB(80, 80, 80),
		Border: windowsRGB(210, 210, 210), Button: windowsRGB(255, 255, 255), ButtonText: windowsRGB(28, 28, 28),
	}
}

func windowsShellMetrics(dpi int) WindowsShellMetrics {
	return WindowsShellMetrics{
		Margin: dip(24, dpi), Gutter: dip(10, dpi), SidebarWidth: dip(200, dpi),
		HeaderHeight: dip(40, dpi), ButtonHeight: dip(40, dpi), CardHeight: dip(104, dpi),
	}
}

func windowsShellMinimumClient(dpi int) (width, height int) {
	return dip(900, dpi), dip(680, dpi)
}

func windowsShellPreferredClient(dpi int) (width, height int) {
	return dip(1040, dpi), dip(700, dpi)
}

func windowsColorContrast(foreground, background uint32) float64 {
	luminance := func(color uint32) float64 {
		channel := func(value uint32) float64 {
			normalized := float64(value) / 255
			if normalized <= 0.04045 {
				return normalized / 12.92
			}
			return math.Pow((normalized+0.055)/1.055, 2.4)
		}
		red := channel(color & 0xff)
		green := channel((color >> 8) & 0xff)
		blue := channel((color >> 16) & 0xff)
		return 0.2126*red + 0.7152*green + 0.0722*blue
	}
	first, second := luminance(foreground), luminance(background)
	if first < second {
		first, second = second, first
	}
	return (first + 0.05) / (second + 0.05)
}
