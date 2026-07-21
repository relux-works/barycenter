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
		"pIsDialogMessageW", "pSetFocus", `label = "●  " + label`,
		"pInvalidateRect", "ctx.rendering", "hidePageControls", "controlBounds", "appliedBounds",
		"repaintStructuralChrome", "renderSectionBody",
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
		if metrics.ButtonHeight < dip(40, dpi) || metrics.Gutter != dip(10, dpi) || metrics.SidebarWidth < dip(180, dpi) {
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

func TestWindowsDenseSectionsFitMinimumClientAtSupportedDPIs(t *testing.T) {
	assertInside := func(t *testing.T, dpi int, client ShellRect, name string, rects []ShellRect) {
		t.Helper()
		for index, rect := range rects {
			if rect.Width < dip(80, dpi) || rect.Height < dip(34, dpi) || rect.X < client.X ||
				rect.Y < client.Y || rect.Right() > client.Right() || rect.Bottom() > client.Bottom() {
				t.Fatalf("dpi=%d %s control %d is clipped: %+v in %+v", dpi, name, index, rect, client)
			}
		}
	}

	for _, dpi := range []int{96, 120, 144, 192} {
		width, height := windowsShellMinimumClient(dpi)
		layout := layoutWindowsShell(width, height, dpi)
		if layout.Footer.Height > dip(160, dpi) {
			t.Fatalf("dpi=%d Home footer expands into an unbounded empty surface: %+v", dpi, layout.Footer)
		}

		historyBody := layout.Body
		historyBody.Height = dip(160, dpi)
		reportY := historyBody.Bottom() + windowsShellMetrics(dpi).Gutter + dip(144, dpi)
		tracks := layoutWindowsStreamTrackControls(layout.Content, reportY+dip(44, dpi), dpi)
		assertInside(t, dpi, layout.Client, "history", tracks.Rects())

		inboxBody := layout.Body
		inboxBody.Height = dip(220, dpi)
		inbox := layoutWindowsTargetsInboxControls(layout.Content, inboxBody.Bottom(), dpi)
		assertInside(t, dpi, layout.Client, "inbox", inbox.Rects())

		airBody := layout.Body
		airBody.Height = dip(200, dpi)
		air := layoutWindowsAirControls(layout.Content, airBody.Bottom(), dpi)
		assertInside(t, dpi, layout.Client, "air", air.Rects())
	}
}

func TestWindowsProductionRenderIsSectionBounded(t *testing.T) {
	windowSource, err := os.ReadFile("main_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(windowSource)
	sections := []string{
		"if section == ShellCreate || section == ShellJoin || section == ShellTryLocally || section == ShellSettings",
		"if section == ShellHistory || section == ShellSoundboard",
		"if section == ShellInbox",
		"if section == ShellAutomation",
		"if section == ShellAirs",
	}
	previous := -1
	for _, section := range sections {
		index := strings.Index(source, section)
		if index <= previous {
			t.Fatalf("section-specific render gate %q is missing or out of order", section)
		}
		previous = index
	}
	if strings.Count(source, "ctx.renderSectionBody(copy, snapshot, section)\n\t\treturn") < len(sections) {
		t.Fatal("one or more Windows pages can fall through into an off-screen page renderer")
	}
}

func TestWindowsExecutableManifestEnablesModernCommonControls(t *testing.T) {
	manifest, err := os.ReadFile("winres/pulsar.exe.manifest")
	if err != nil {
		t.Fatal(err)
	}
	text := string(manifest)
	for _, required := range []string{"Microsoft.Windows.Common-Controls", `version="6.0.0.0"`, `publicKeyToken="6595b64144ccf1df"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("production manifest is missing %q", required)
		}
	}
}
