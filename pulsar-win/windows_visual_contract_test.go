package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsVisualModeResolutionHonorsHighContrastAndTheme(t *testing.T) {
	light, dark := uint32(1), uint32(0)
	cases := []struct {
		highContrast bool
		setting      *uint32
		want         WindowsVisualMode
	}{
		{setting: nil, want: WindowsVisualLight},
		{setting: &light, want: WindowsVisualLight},
		{setting: &dark, want: WindowsVisualDark},
		{highContrast: true, setting: &dark, want: WindowsVisualHighContrast},
	}
	for _, test := range cases {
		if got := resolveWindowsVisualMode(test.highContrast, test.setting); got != test.want {
			t.Fatalf("resolveWindowsVisualMode(%v, %v) = %v, want %v", test.highContrast, test.setting, got, test.want)
		}
	}
}

func TestWindowsDefaultPalettesMeetTextContrastContract(t *testing.T) {
	for _, mode := range []WindowsVisualMode{WindowsVisualLight, WindowsVisualDark} {
		palette := windowsThemePalette(mode)
		for name, foreground := range map[string]uint32{"text": palette.Text, "secondary": palette.SecondaryText} {
			for surfaceName, background := range map[string]uint32{"background": palette.Background, "surface": palette.Surface} {
				if contrast := windowsColorContrast(foreground, background); contrast < 4.5 {
					t.Fatalf("%v %s on %s contrast %.2f is below 4.5:1", mode, name, surfaceName, contrast)
				}
			}
		}
		if contrast := windowsColorContrast(palette.ButtonText, palette.Button); contrast < 4.5 {
			t.Fatalf("%v button contrast %.2f is below 4.5:1", mode, contrast)
		}
	}
}

func TestWindowsProductionWindowWiresThemeAccessibilityAndFocusContracts(t *testing.T) {
	themeSource, err := os.ReadFile("main_theme_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	windowSource, err := os.ReadFile("main_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(themeSource) + string(windowSource)
	for _, required := range []string{
		"SystemParametersInfoW", "spiGetHighContrast", "AppsUseLightTheme",
		"DwmSetWindowAttribute", "DarkMode_Explorer", "wmSysColor", "wmSetting", "wmTheme",
		"pIsDialogMessageW", "pSetFocus", "BM_SETSTATE: visible non-color selection",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("production Windows shell is missing %s integration", required)
		}
	}
}

func TestWindowsShellMetricsScaleAtSupportedDPIs(t *testing.T) {
	var previous WindowsShellMetrics
	for _, dpi := range []int{96, 120, 144, 192} {
		metrics := windowsShellMetrics(dpi)
		minimumWidth, minimumHeight := windowsShellMinimumClient(dpi)
		preferredWidth, preferredHeight := windowsShellPreferredClient(dpi)
		if metrics.ButtonHeight < dip(40, dpi) || metrics.Gutter != dip(12, dpi) || metrics.SidebarWidth < dip(180, dpi) {
			t.Fatalf("DPI %d metrics violate the interaction contract: %+v", dpi, metrics)
		}
		if preferredWidth <= minimumWidth || preferredHeight <= minimumHeight {
			t.Fatalf("DPI %d preferred client does not exceed minimum", dpi)
		}
		if dpi > 96 && (metrics.Margin <= previous.Margin || metrics.ButtonHeight <= previous.ButtonHeight) {
			t.Fatalf("DPI %d metrics did not scale from %+v to %+v", dpi, previous, metrics)
		}
		previous = metrics
	}
}
