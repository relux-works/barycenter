package main

import (
	"strconv"
	"testing"
)

func TestProbeLayoutScalesAcrossSupportedDPIs(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		dpi := dpi
		t.Run(strconv.Itoa(dpi), func(t *testing.T) {
			width, height := probeInitialClientSize(dpi)
			layout := probeLayoutFor(dpi, width, height)
			minimumControlHeight := probeDIP(38, dpi)
			if layout.RecordDefault.Height != minimumControlHeight || layout.Hide.Height != minimumControlHeight {
				t.Fatalf("button height is not DPI-scaled: %+v", layout)
			}
			if layout.Intro.X != probeDIP(24, dpi) || layout.Status.Y+layout.Status.Height != height-probeDIP(24, dpi) {
				t.Fatalf("outer margins are not DPI-scaled: %+v", layout)
			}
			assertProbeLayoutFits(t, layout, width, height)
		})
	}
}

func TestProbeLayoutUsesAdditionalResizeSpace(t *testing.T) {
	dpi := 144
	baseWidth, baseHeight := probeInitialClientSize(dpi)
	base := probeLayoutFor(dpi, baseWidth, baseHeight)
	wide := probeLayoutFor(dpi, baseWidth+probeDIP(240, dpi), baseHeight+probeDIP(160, dpi))
	if wide.Devices.Width <= base.Devices.Width || wide.Devices.Height <= base.Devices.Height {
		t.Fatalf("device list did not grow with the client area: base=%+v wide=%+v", base.Devices, wide.Devices)
	}
	assertProbeLayoutFits(t, wide, baseWidth+probeDIP(240, dpi), baseHeight+probeDIP(160, dpi))
}

func TestProbeLayoutClampsToMinimumUsableGeometry(t *testing.T) {
	for _, dpi := range []int{96, 192} {
		minimumWidth, minimumHeight := probeMinimumWindowSize(dpi)
		layout := probeLayoutFor(dpi, 1, 1)
		assertProbeLayoutFits(t, layout, minimumWidth, minimumHeight)
		if layout.Devices.Height <= 0 {
			t.Fatalf("device list collapsed at %d DPI: %+v", dpi, layout.Devices)
		}
	}
}

func assertProbeLayoutFits(t *testing.T, layout probeUILayout, width, height int) {
	t.Helper()
	rects := []probeRect{
		layout.Intro, layout.Devices, layout.RecordDefault, layout.RecordSelected,
		layout.Stop, layout.Picker, layout.Hide, layout.Status,
	}
	for _, rect := range rects {
		if rect.X < 0 || rect.Y < 0 || rect.Width <= 0 || rect.Height <= 0 || rect.X+rect.Width > width || rect.Y+rect.Height > height {
			t.Fatalf("control is outside %dx%d client area: %+v", width, height, rect)
		}
	}
	buttons := []probeRect{layout.RecordDefault, layout.RecordSelected, layout.Stop, layout.Picker, layout.Hide}
	for index := 1; index < len(buttons); index++ {
		if buttons[index-1].X+buttons[index-1].Width >= buttons[index].X {
			t.Fatalf("buttons overlap: left=%+v right=%+v", buttons[index-1], buttons[index])
		}
	}
	if layout.Devices.Y+layout.Devices.Height >= layout.RecordDefault.Y || layout.Hide.Y+layout.Hide.Height >= layout.Status.Y {
		t.Fatalf("vertical regions overlap: %+v", layout)
	}
}
