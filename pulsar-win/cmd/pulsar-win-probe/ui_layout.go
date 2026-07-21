package main

const (
	probeBaseDPI         = 96
	probeClientWidthDIP  = 760
	probeClientHeightDIP = 520
	probeMinWidthDIP     = 680
	probeMinHeightDIP    = 440
)

type probeRect struct {
	X, Y, Width, Height int
}

type probeUILayout struct {
	Intro, Devices, RecordDefault, RecordSelected, Stop, Picker, Hide, Status probeRect
}

func probeDIP(value, dpi int) int {
	if dpi <= 0 {
		dpi = probeBaseDPI
	}
	return (value*dpi + probeBaseDPI/2) / probeBaseDPI
}

func probeInitialClientSize(dpi int) (width, height int) {
	return probeDIP(probeClientWidthDIP, dpi), probeDIP(probeClientHeightDIP, dpi)
}

func probeMinimumWindowSize(dpi int) (width, height int) {
	return probeDIP(probeMinWidthDIP, dpi), probeDIP(probeMinHeightDIP, dpi)
}

func probeLayoutFor(dpi, clientWidth, clientHeight int) probeUILayout {
	margin := probeDIP(24, dpi)
	gap := probeDIP(12, dpi)
	buttonGap := probeDIP(10, dpi)
	introHeight := probeDIP(44, dpi)
	buttonHeight := probeDIP(38, dpi)
	statusHeight := probeDIP(52, dpi)

	minimumWidth, minimumHeight := probeMinimumWindowSize(dpi)
	if clientWidth < minimumWidth {
		clientWidth = minimumWidth
	}
	if clientHeight < minimumHeight {
		clientHeight = minimumHeight
	}

	contentWidth := clientWidth - 2*margin
	intro := probeRect{X: margin, Y: margin, Width: contentWidth, Height: introHeight}
	status := probeRect{
		X: margin, Y: clientHeight - margin - statusHeight,
		Width: contentWidth, Height: statusHeight,
	}
	buttonY := status.Y - gap - buttonHeight
	devicesY := intro.Y + intro.Height + gap
	devices := probeRect{
		X: margin, Y: devicesY, Width: contentWidth,
		Height: buttonY - gap - devicesY,
	}

	buttonWidth := (contentWidth - 4*buttonGap) / 5
	button := func(index int) probeRect {
		x := margin + index*(buttonWidth+buttonGap)
		width := buttonWidth
		if index == 4 {
			width = margin + contentWidth - x
		}
		return probeRect{X: x, Y: buttonY, Width: width, Height: buttonHeight}
	}

	return probeUILayout{
		Intro: intro, Devices: devices,
		RecordDefault: button(0), RecordSelected: button(1), Stop: button(2),
		Picker: button(3), Hide: button(4), Status: status,
	}
}
