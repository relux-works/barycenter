//go:build windows

package main

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pMoveWindow               = user32.NewProc("MoveWindow")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	pGetDpiForSystem          = user32.NewProc("GetDpiForSystem")
	pSetProcessDPIContext     = user32.NewProc("SetProcessDpiAwarenessContext")
	pEnableWindow             = user32.NewProc("EnableWindow")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pIsChild                  = user32.NewProc("IsChild")
	pIsDialogMessageW         = user32.NewProc("IsDialogMessageW")
	pTranslateAcceleratorW    = user32.NewProc("TranslateAcceleratorW")
	pCreateAcceleratorTableW  = user32.NewProc("CreateAcceleratorTableW")
	pDestroyAcceleratorTableW = user32.NewProc("DestroyAcceleratorTable")
	pDragAcceptFiles          = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW           = shell32.NewProc("DragQueryFileW")
	pDragFinish               = shell32.NewProc("DragFinish")
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pRtlMoveMemory            = kernel32.NewProc("RtlMoveMemory")
	pGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
)

const (
	wsMinimizeBox     = 0x00020000
	wsMaximizeBox     = 0x00010000
	wsThickFrame      = 0x00040000
	wsClipChildren    = 0x02000000
	wsExControlParent = 0x00010000
	wsGroup           = 0x00020000

	wmClose      = 0x0010
	wmSize       = 0x0005
	wmTimer      = 0x0113
	wmGetMinMax  = 0x0024
	wmDPIChanged = 0x02E0
	wmDropFiles  = 0x0233

	swHide    = 0
	swRestore = 9

	mainRefreshTimer = 1

	idShellHome          = 3001
	idShellCreate        = 3002
	idShellJoin          = 3003
	idShellTry           = 3004
	idShellHistory       = 3005
	idShellSettings      = 3006
	idShellAction        = 3010
	idShellRecord        = 3011
	idShellDND           = 3012
	idShellEnglish       = 3013
	idShellRussian       = 3014
	idShellOpen          = 3020
	idShellCancel        = 3021
	idShellCue           = 3022
	idShellFile          = 3023
	idShellDelete        = 3024
	idShellInput         = 3025
	idShellOutput        = 3026
	idShellIdentityInput = 3027
	idShellRecovery      = 3028
	idShellDraftNext     = 3030
	idShellRoute         = 3031
	idShellDelivery      = 3032
	idShellSend          = 3033
	idShellPhaseDelete   = 3034
	idShellHistoryNext   = 3035
	idShellHistoryDelete = 3036
	idShellHistoryReplay = 3037
	idShellHistoryBlock  = 3038
	idShellOutgoingFile  = 3039
	idShellReportReason  = 3040
	idShellReportDetails = 3041
	idShellHistoryReport = 3042

	bsPushButton = 0x00000000
	bsMultiline  = 0x00002000
	ssLeft       = 0x00000000

	fVirtKey       = 0x01
	fShift         = 0x04
	fControl       = 0x08
	vkComma        = 0xBC
	vkEscape       = 0x1B
	emSetLimitText = 0x00C5
)

type winRect struct{ left, top, right, bottom int32 }
type minMaxInfo struct {
	reserved, maxSize, maxPosition, minTrackSize, maxTrackSize pointStruct
}
type accel struct {
	fVirt byte
	_     byte
	key   uint16
	cmd   uint16
}

type mainFonts struct {
	dpi                int
	title, body, small windows.Handle
}

type mainWindowCtx struct {
	hwnd          windows.Handle
	shell         *WindowsShell
	nav           map[ShellSection]windows.Handle
	title         windows.Handle
	banner        windows.Handle
	body          windows.Handle
	home          [3]windows.Handle
	cards         [3]windows.Handle
	footer        windows.Handle
	detail        windows.Handle
	identityInput windows.Handle
	recovery      windows.Handle
	cue           windows.Handle
	file          windows.Handle
	outgoingFile  windows.Handle
	delete        windows.Handle
	input         windows.Handle
	output        windows.Handle
	draftNext     windows.Handle
	route         windows.Handle
	delivery      windows.Handle
	send          windows.Handle
	phaseDelete   windows.Handle
	historyNext   windows.Handle
	historyDelete windows.Handle
	historyReplay windows.Handle
	historyBlock  windows.Handle
	reportReason  windows.Handle
	reportLabel   windows.Handle
	reportDetails windows.Handle
	historyReport windows.Handle
	record        windows.Handle
	dnd           windows.Handle
	english       windows.Handle
	russian       windows.Handle
	all           []windows.Handle
	fonts         mainFonts
}

var (
	mainCtx        *mainWindowCtx
	mainHwnd       windows.Handle
	mainAccel      windows.Handle
	mainClassReady bool
)

func currentMainWindowOwner() uintptr { return uintptr(mainHwnd) }

func preferredWindowsShellLocale() ShellLocale {
	p := kernel32.NewProc("GetUserDefaultUILanguage")
	language, _, _ := p.Call()
	if language&0x3ff == 0x19 { // LANG_RUSSIAN
		return ShellRussian
	}
	return ShellEnglish
}

func createMainWindow(shell *WindowsShell) windows.Handle {
	if shell == nil {
		return 0
	}
	// The embedded manifest is authoritative. The call makes unpackaged/dev
	// builds PerMonitorV2 as well; ERROR_ACCESS_DENIED simply means it was set.
	pSetProcessDPIContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	hInst, _, _ := pGetModuleHandleW.Call(0)
	className := u16("PulsarMainWindow")
	if !mainClassReady {
		cursor, _, _ := pLoadCursorW.Call(0, uintptr(idcArrow))
		wc := wndClassExW{
			cbSize: uint32(unsafe.Sizeof(wndClassExW{})), lpfnWndProc: syscall.NewCallback(mainWindowProc),
			hInstance: windows.Handle(hInst), hIcon: appIcon(), hIconSm: appIcon(),
			hCursor: windows.Handle(cursor), hbrBackground: windows.Handle(colorWindow + 1),
			lpszClassName: className,
		}
		if result, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); result == 0 {
			return 0
		}
		mainClassReady = true
	}
	dpi, _, _ := pGetDpiForSystem.Call()
	if dpi == 0 {
		dpi = 96
	}
	hwnd, _, _ := pCreateWindowExW.Call(
		wsExControlParent, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(u16("Pulsar"))),
		wsOverlapped|wsCaption|wsSysMenu|wsMinimizeBox|wsMaximizeBox|wsThickFrame|wsClipChildren,
		cwUseDefault, cwUseDefault, uintptr(dip(780, int(dpi))), uintptr(dip(500, int(dpi))),
		0, 0, hInst, 0)
	if hwnd == 0 {
		return 0
	}
	mainHwnd = windows.Handle(hwnd)
	mainCtx = &mainWindowCtx{hwnd: mainHwnd, shell: shell}
	// WM_CREATE arrived before mainCtx was assigned; build after creation so
	// the callback never has to recover Go pointers from CREATESTRUCT.
	mainCtx.createControls()
	pDragAcceptFiles.Call(hwnd, 1)
	mainCtx.installAccelerators()
	mainCtx.render()
	mainCtx.layout()
	pSetTimer.Call(hwnd, mainRefreshTimer, 1000, 0)
	return mainHwnd
}

func (ctx *mainWindowCtx) createControls() {
	hInst, _, _ := pGetModuleHandleW.Call(0)
	mk := func(ex uint32, class, text string, style uint32, id int) windows.Handle {
		h, _, _ := pCreateWindowExW.Call(uintptr(ex), uintptr(unsafe.Pointer(u16(class))),
			uintptr(unsafe.Pointer(u16(text))), uintptr(style), 0, 0, 1, 1,
			uintptr(ctx.hwnd), uintptr(id), hInst, 0)
		handle := windows.Handle(h)
		ctx.all = append(ctx.all, handle)
		return handle
	}
	ctx.nav = map[ShellSection]windows.Handle{}
	for index, section := range shellSections {
		ctx.nav[section] = mk(0, "BUTTON", "", buttonStyle|wsGroup|bsMultiline, idShellHome+index)
	}
	ctx.title = mk(0, "STATIC", "", wsChild|wsVisible|ssLeft, 0)
	ctx.banner = mk(wsExClientEdge, "STATIC", "", wsChild|wsVisible|ssLeft, 0)
	ctx.body = mk(0, "STATIC", "", wsChild|wsVisible|ssLeft, 0)
	ctx.home[0] = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellCreate)
	ctx.home[1] = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellJoin)
	ctx.home[2] = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellTry)
	for i := range ctx.cards {
		ctx.cards[i] = mk(wsExClientEdge, "STATIC", "", wsChild|wsVisible|ssLeft, 0)
	}
	ctx.footer = mk(wsExClientEdge, "STATIC", "", wsChild|wsVisible|ssLeft, 0)
	ctx.detail = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellAction)
	ctx.identityInput = mk(wsExClientEdge, "EDIT", "", wsChild|wsVisible|wsTabStop|0x0080, idShellIdentityInput)
	ctx.recovery = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellRecovery)
	ctx.cue = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellCue)
	ctx.file = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellFile)
	ctx.outgoingFile = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellOutgoingFile)
	ctx.delete = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellDelete)
	ctx.input = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellInput)
	ctx.output = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellOutput)
	ctx.draftNext = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellDraftNext)
	ctx.route = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellRoute)
	ctx.delivery = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellDelivery)
	ctx.send = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellSend)
	ctx.phaseDelete = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellPhaseDelete)
	ctx.historyNext = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellHistoryNext)
	ctx.historyDelete = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellHistoryDelete)
	ctx.historyReplay = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellHistoryReplay)
	ctx.historyBlock = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellHistoryBlock)
	ctx.reportReason = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellReportReason)
	ctx.reportLabel = mk(0, "STATIC", "", wsChild|wsVisible|ssLeft, 0)
	ctx.reportDetails = mk(wsExClientEdge, "EDIT", "", wsChild|wsVisible|wsTabStop|0x0080, idShellReportDetails)
	pSendMessageW.Call(uintptr(ctx.reportDetails), emSetLimitText, 2000, 0)
	ctx.historyReport = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellHistoryReport)
	ctx.record = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellRecord)
	ctx.dnd = mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellDND)
	ctx.english = mk(0, "BUTTON", "English", buttonStyle|bsPushButton, idShellEnglish)
	ctx.russian = mk(0, "BUTTON", "Русский", buttonStyle|bsPushButton, idShellRussian)
	ctx.updateFonts()
}

func (ctx *mainWindowCtx) installAccelerators() {
	entries := []accel{
		{fVirtKey | fControl, 0, '0', idShellOpen},
		{fVirtKey | fControl, 0, '1', idShellCreate},
		{fVirtKey | fControl, 0, '2', idShellJoin},
		{fVirtKey | fControl | fShift, 0, 'T', idShellTry},
		{fVirtKey | fControl | fShift, 0, 'R', idShellRecord},
		{fVirtKey | fControl | fShift, 0, 'D', idShellDND},
		{fVirtKey | fControl, 0, vkComma, idShellSettings},
		{fVirtKey, 0, vkEscape, idShellCancel},
	}
	h, _, _ := pCreateAcceleratorTableW.Call(uintptr(unsafe.Pointer(&entries[0])), uintptr(len(entries)))
	mainAccel = windows.Handle(h)
}

func (ctx *mainWindowCtx) updateFonts() {
	dpi := ctx.dpi()
	if ctx.fonts.dpi == dpi {
		return
	}
	for _, font := range []windows.Handle{ctx.fonts.title, ctx.fonts.body, ctx.fonts.small} {
		if font != 0 {
			pDeleteObject.Call(uintptr(font))
		}
	}
	ctx.fonts = mainFonts{
		dpi:   dpi,
		title: mkFont(-dip(26, dpi), fwSemibold),
		body:  mkFont(-dip(17, dpi), fwNormal),
		small: mkFont(-dip(14, dpi), fwNormal),
	}
	for _, control := range ctx.all {
		setFont(control, ctx.fonts.body)
	}
	setFont(ctx.title, ctx.fonts.title)
	setFont(ctx.banner, ctx.fonts.small)
	for _, card := range ctx.cards {
		setFont(card, ctx.fonts.small)
	}
	setFont(ctx.footer, ctx.fonts.small)
}

func (ctx *mainWindowCtx) dpi() int {
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(ctx.hwnd))
	if dpi == 0 {
		return 96
	}
	return int(dpi)
}

func move(control windows.Handle, rect ShellRect) {
	pMoveWindow.Call(uintptr(control), uintptr(rect.X), uintptr(rect.Y), uintptr(rect.Width), uintptr(rect.Height), 1)
}

func showControl(control windows.Handle, visible bool) {
	command := uintptr(swHide)
	if visible {
		command = swShow
	}
	pShowWindow.Call(uintptr(control), command)
}

func (ctx *mainWindowCtx) layout() {
	var client winRect
	pGetClientRect.Call(uintptr(ctx.hwnd), uintptr(unsafe.Pointer(&client)))
	layout := layoutWindowsShell(int(client.right-client.left), int(client.bottom-client.top), ctx.dpi())
	gap, pad := dip(8, layout.DPI), dip(10, layout.DPI)
	navHeight := dip(42, layout.DPI)
	for index, section := range shellSections {
		move(ctx.nav[section], ShellRect{X: layout.Sidebar.X, Y: layout.Sidebar.Y + index*(navHeight+gap), Width: layout.Sidebar.Width, Height: navHeight})
	}
	dndWidth, recordWidth := dip(150, layout.DPI), dip(140, layout.DPI)
	move(ctx.title, ShellRect{X: layout.Header.X, Y: layout.Header.Y, Width: layout.Header.Width - dndWidth - recordWidth - gap*2, Height: layout.Header.Height})
	move(ctx.dnd, ShellRect{X: layout.Header.Right() - dndWidth - recordWidth - gap, Y: layout.Header.Y, Width: dndWidth, Height: layout.Header.Height})
	move(ctx.record, ShellRect{X: layout.Header.Right() - recordWidth, Y: layout.Header.Y, Width: recordWidth, Height: layout.Header.Height})
	move(ctx.banner, ShellRect{X: layout.Banner.X + pad, Y: layout.Banner.Y + pad, Width: layout.Banner.Width - pad*2, Height: layout.Banner.Height - pad*2})
	move(ctx.body, layout.Body)
	for index := range ctx.home {
		move(ctx.home[index], ShellRect{X: layout.Body.X + index*(layout.Body.Width-gap*2)/3 + index*gap, Y: layout.Body.Y + dip(42, layout.DPI), Width: (layout.Body.Width - gap*2) / 3, Height: dip(48, layout.DPI)})
		move(ctx.cards[index], ShellRect{X: layout.Cards[index].X + pad, Y: layout.Cards[index].Y + pad, Width: layout.Cards[index].Width - pad*2, Height: layout.Cards[index].Height - pad*2})
	}
	move(ctx.footer, ShellRect{X: layout.Footer.X + pad, Y: layout.Footer.Y + pad, Width: layout.Footer.Width - pad*2, Height: layout.Footer.Height - pad*2})
	move(ctx.detail, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap, Width: dip(220, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.identityInput, ShellRect{X: layout.Content.X, Y: layout.Body.Y + dip(72, layout.DPI), Width: layout.Body.Width, Height: dip(34, layout.DPI)})
	move(ctx.recovery, ShellRect{X: layout.Content.X + dip(232, layout.DPI), Y: layout.Body.Bottom() + gap, Width: dip(220, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.cue, ShellRect{X: layout.Content.X + dip(232, layout.DPI), Y: layout.Body.Bottom() + gap, Width: dip(150, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.file, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap + dip(52, layout.DPI), Width: dip(180, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.delete, ShellRect{X: layout.Content.X + dip(192, layout.DPI), Y: layout.Body.Bottom() + gap + dip(52, layout.DPI), Width: dip(150, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.input, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap + dip(104, layout.DPI), Width: dip(180, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.output, ShellRect{X: layout.Content.X + dip(192, layout.DPI), Y: layout.Body.Bottom() + gap + dip(104, layout.DPI), Width: dip(180, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.english, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap, Width: dip(130, layout.DPI), Height: dip(42, layout.DPI)})
	move(ctx.russian, ShellRect{X: layout.Content.X + dip(142, layout.DPI), Y: layout.Body.Bottom() + gap, Width: dip(130, layout.DPI), Height: dip(42, layout.DPI)})
	move(ctx.draftNext, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap, Width: dip(132, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.route, ShellRect{X: layout.Content.X + dip(140, layout.DPI), Y: layout.Body.Bottom() + gap, Width: dip(160, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.delivery, ShellRect{X: layout.Content.X + dip(308, layout.DPI), Y: layout.Body.Bottom() + gap, Width: dip(160, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.send, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap + dip(52, layout.DPI), Width: dip(132, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.phaseDelete, ShellRect{X: layout.Content.X + dip(140, layout.DPI), Y: layout.Body.Bottom() + gap + dip(52, layout.DPI), Width: dip(132, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.outgoingFile, ShellRect{X: layout.Content.X + dip(280, layout.DPI), Y: layout.Body.Bottom() + gap + dip(52, layout.DPI), Width: dip(188, layout.DPI), Height: dip(44, layout.DPI)})
	move(ctx.historyNext, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap + dip(104, layout.DPI), Width: dip(112, layout.DPI), Height: dip(40, layout.DPI)})
	move(ctx.historyDelete, ShellRect{X: layout.Content.X + dip(120, layout.DPI), Y: layout.Body.Bottom() + gap + dip(104, layout.DPI), Width: dip(112, layout.DPI), Height: dip(40, layout.DPI)})
	move(ctx.historyReplay, ShellRect{X: layout.Content.X + dip(240, layout.DPI), Y: layout.Body.Bottom() + gap + dip(104, layout.DPI), Width: dip(112, layout.DPI), Height: dip(40, layout.DPI)})
	move(ctx.historyBlock, ShellRect{X: layout.Content.X + dip(360, layout.DPI), Y: layout.Body.Bottom() + gap + dip(104, layout.DPI), Width: dip(112, layout.DPI), Height: dip(40, layout.DPI)})
	reportY := layout.Body.Bottom() + gap + dip(152, layout.DPI)
	move(ctx.reportReason, ShellRect{X: layout.Content.X, Y: reportY, Width: dip(124, layout.DPI), Height: dip(40, layout.DPI)})
	move(ctx.reportLabel, ShellRect{X: layout.Content.X + dip(132, layout.DPI), Y: reportY + dip(3, layout.DPI), Width: dip(108, layout.DPI), Height: dip(36, layout.DPI)})
	move(ctx.reportDetails, ShellRect{X: layout.Content.X + dip(248, layout.DPI), Y: reportY + dip(3, layout.DPI), Width: dip(108, layout.DPI), Height: dip(34, layout.DPI)})
	move(ctx.historyReport, ShellRect{X: layout.Content.X + dip(364, layout.DPI), Y: reportY, Width: dip(104, layout.DPI), Height: dip(40, layout.DPI)})
}

func (ctx *mainWindowCtx) render() {
	if ctx.shell == nil {
		return
	}
	snapshot := ctx.shell.Snapshot()
	section := ctx.shell.Section()
	copy := NewShellCopy(ctx.shell.Locale())
	for candidate, control := range ctx.nav {
		label := copy.Section(candidate)
		if candidate == section {
			label = "> " + label
		}
		setText(control, label)
	}
	setText(ctx.title, copy.Section(section))
	banner := copy.Connection(snapshot)
	if snapshot.Identity != "" {
		banner += "\r\n" + snapshot.Identity
	}
	if snapshot.Recording == ShellRecordingActive || snapshot.Recording == ShellRecordingProcessing {
		banner += "\r\n" + copy.Text(txtRecordingHelp)
	}
	setText(ctx.banner, banner)
	recordText := copy.Text(txtStartRecording)
	if snapshot.Recording == ShellRecordingActive || snapshot.Recording == ShellRecordingProcessing {
		recordText = copy.Text(txtStopRecording)
	}
	setText(ctx.record, recordText)
	pEnableWindow.Call(uintptr(ctx.record), boolWord(shellRecordingEnabled(snapshot)))
	setText(ctx.dnd, copy.Text(txtDND)+": "+copy.DND(snapshot.DND))
	pEnableWindow.Call(uintptr(ctx.dnd), boolWord(shellDNDEnabled(snapshot)))

	home := section == ShellHome
	for _, control := range ctx.home {
		showControl(control, home)
	}
	for _, control := range ctx.cards {
		showControl(control, home)
	}
	showControl(ctx.footer, home)
	showControl(ctx.detail, !home && (section == ShellCreate || section == ShellJoin || section == ShellTryLocally))
	identityPage := section == ShellCreate || section == ShellJoin
	showControl(ctx.identityInput, identityPage)
	showControl(ctx.recovery, identityPage && snapshot.RecoveryExportRequired)
	pEnableWindow.Call(uintptr(ctx.identityInput), boolWord(snapshot.IdentityOperation != ShellIdentityWorking && snapshot.IdentityOperation != ShellIdentityActive && !snapshot.RecoveryExportRequired))
	if copy.locale == ShellRussian {
		setText(ctx.recovery, "Сохранить файл восстановления...")
	} else {
		setText(ctx.recovery, "Save recovery file...")
	}
	tryPage := section == ShellTryLocally
	busy := shellLocalCaptureBusy(snapshot)
	detailEnabled := !tryPage || (snapshot.SelfTestAvailable && !busy && snapshot.Recording != ShellRecordingActive && snapshot.Recording != ShellRecordingProcessing)
	if identityPage && (snapshot.IdentityOperation == ShellIdentityWorking || snapshot.IdentityOperation == ShellIdentityActive || snapshot.RecoveryExportRequired) {
		detailEnabled = false
	}
	pEnableWindow.Call(uintptr(ctx.detail), boolWord(detailEnabled))
	for _, control := range []windows.Handle{ctx.cue, ctx.file, ctx.delete, ctx.input, ctx.output} {
		showControl(control, tryPage)
	}
	localEnabled := snapshot.SelfTestAvailable && !busy && snapshot.Recording != ShellRecordingActive && snapshot.Recording != ShellRecordingProcessing
	pEnableWindow.Call(uintptr(ctx.cue), boolWord(localEnabled))
	pEnableWindow.Call(uintptr(ctx.file), boolWord(localEnabled))
	pEnableWindow.Call(uintptr(ctx.delete), boolWord(localEnabled && snapshot.LocalDraftAvailable))
	pEnableWindow.Call(uintptr(ctx.input), boolWord(localEnabled && len(snapshot.CaptureInputs) > 1))
	pEnableWindow.Call(uintptr(ctx.output), boolWord(localEnabled && len(snapshot.AudioOutputs) > 1))
	if copy.locale == ShellRussian {
		setText(ctx.cue, "Проиграть сигнал")
		setText(ctx.file, "Выбрать WAV-файл")
		setText(ctx.delete, "Удалить черновик")
		setText(ctx.input, "Сменить вход")
		setText(ctx.output, "Сменить выход")
	} else {
		setText(ctx.cue, "Play cue")
		setText(ctx.file, "Choose WAV file")
		setText(ctx.delete, "Delete draft")
		setText(ctx.input, "Next input")
		setText(ctx.output, "Next output")
	}
	showControl(ctx.english, section == ShellSettings)
	showControl(ctx.russian, section == ShellSettings)
	historyPage := section == ShellHistory
	for _, control := range []windows.Handle{ctx.draftNext, ctx.route, ctx.delivery, ctx.send, ctx.phaseDelete, ctx.outgoingFile, ctx.historyNext, ctx.historyDelete, ctx.historyReplay, ctx.historyBlock, ctx.reportReason, ctx.reportLabel, ctx.reportDetails, ctx.historyReport} {
		showControl(control, historyPage)
	}
	hasDraft := len(snapshot.PhaseOneDrafts) > 0
	hasHistory := len(snapshot.PhaseOneHistory) > 0
	setText(ctx.draftNext, map[bool]string{true: "Next draft", false: "След. черновик"}[copy.locale == ShellEnglish])
	setText(ctx.route, copy.Route(snapshot.SelectedPhaseOneRoute))
	setText(ctx.delivery, copy.Delivery(snapshot.SelectedPhaseOneDelivery))
	if copy.locale == ShellRussian {
		setText(ctx.outgoingFile, "Добавить аудиофайл...")
		if hasDraft && snapshot.PhaseOneDrafts[snapshot.SelectedPhaseOneDraft].FallbackConfirmationAvailable {
			setText(ctx.send, "Подтвердить: после текущего")
		} else {
			setText(ctx.send, "Отправить / повторить")
		}
		setText(ctx.phaseDelete, "Удалить черновик")
		setText(ctx.historyNext, "След. запись")
		setText(ctx.historyDelete, "Удалить навсегда")
		setText(ctx.historyReplay, "Повторить")
		setText(ctx.historyBlock, "Заблокировать отправителя")
		setText(ctx.reportReason, "Причина: "+copy.ModerationReason(snapshot.SelectedReportReason))
		setText(ctx.reportLabel, "Детали (необязательно)")
		setText(ctx.historyReport, "Отправить жалобу")
	} else {
		setText(ctx.outgoingFile, "Add audio file...")
		if hasDraft && snapshot.PhaseOneDrafts[snapshot.SelectedPhaseOneDraft].FallbackConfirmationAvailable {
			setText(ctx.send, "Confirm: after current")
		} else {
			setText(ctx.send, "Send / retry")
		}
		setText(ctx.phaseDelete, "Delete draft")
		setText(ctx.historyNext, "Next item")
		setText(ctx.historyDelete, "Delete permanently")
		setText(ctx.historyReplay, "Replay")
		setText(ctx.historyBlock, "Block sender")
		setText(ctx.reportReason, "Reason: "+copy.ModerationReason(snapshot.SelectedReportReason))
		setText(ctx.reportLabel, "Details (optional)")
		setText(ctx.historyReport, "Submit report")
	}
	pEnableWindow.Call(uintptr(ctx.draftNext), boolWord(len(snapshot.PhaseOneDrafts) > 1))
	pEnableWindow.Call(uintptr(ctx.send), boolWord(hasDraft))
	pEnableWindow.Call(uintptr(ctx.phaseDelete), boolWord(hasDraft))
	pEnableWindow.Call(uintptr(ctx.outgoingFile), boolWord(snapshot.SelfTestAvailable && !busy))
	pEnableWindow.Call(uintptr(ctx.historyNext), boolWord(len(snapshot.PhaseOneHistory) > 1))
	selectedHistory := ShellPhaseOneHistoryItem{}
	if hasHistory {
		selectedHistory = snapshot.PhaseOneHistory[snapshot.SelectedHistoryItem]
	}
	showControl(ctx.historyDelete, historyPage && hasHistory && selectedHistory.CanDelete)
	showControl(ctx.historyReplay, historyPage && hasHistory && selectedHistory.CanReplay)
	showControl(ctx.historyBlock, historyPage && hasHistory && selectedHistory.CanBlock)
	showControl(ctx.reportReason, historyPage && hasHistory && selectedHistory.CanReport)
	showControl(ctx.reportLabel, historyPage && hasHistory && selectedHistory.CanReport)
	showControl(ctx.reportDetails, historyPage && hasHistory && selectedHistory.CanReport)
	showControl(ctx.historyReport, historyPage && hasHistory && selectedHistory.CanReport)
	pEnableWindow.Call(uintptr(ctx.historyDelete), boolWord(hasHistory && selectedHistory.CanDelete))
	pEnableWindow.Call(uintptr(ctx.historyReplay), boolWord(hasHistory && selectedHistory.CanReplay))
	pEnableWindow.Call(uintptr(ctx.historyBlock), boolWord(hasHistory && selectedHistory.CanBlock))
	pEnableWindow.Call(uintptr(ctx.reportReason), boolWord(hasHistory && selectedHistory.CanReport))
	pEnableWindow.Call(uintptr(ctx.reportDetails), boolWord(hasHistory && selectedHistory.CanReport))
	pEnableWindow.Call(uintptr(ctx.historyReport), boolWord(hasHistory && selectedHistory.CanReport))

	if home {
		setText(ctx.body, copy.Text(txtPrimary)+"\r\n"+copy.Body(section, snapshot))
		setText(ctx.home[0], copy.Text(txtCreate)+"  Ctrl+1")
		setText(ctx.home[1], copy.Text(txtJoin)+"  Ctrl+2")
		setText(ctx.home[2], copy.Text(txtTry)+"  Ctrl+Shift+T")
		setText(ctx.cards[0], copy.Text(txtPresence)+"\r\n"+copy.Presence(snapshot))
		route := snapshot.RouteName
		if route == "" {
			route = copy.Text(txtNoRoute)
		}
		setText(ctx.cards[1], copy.Text(txtRouting)+"\r\n"+route)
		playing := snapshot.NowPlaying
		if playing == "" {
			playing = copy.Text(txtSilence)
		}
		setText(ctx.cards[2], copy.Text(txtNowPlaying)+"\r\n"+playing)
		setText(ctx.footer, copy.Text(txtLocalControls)+"\r\n"+copy.Recording(snapshot)+"\r\n"+
			copy.RecordingShortcut(snapshot.RecordingShortcut, snapshot.RecordingShortcutKey)+"\r\n"+
			copy.Text(txtDND)+": "+copy.DND(snapshot.DND)+"    "+copy.Text(txtVolume)+fmtPercent(snapshot.Volume)+
			"\r\n\r\n"+copy.Text(txtHistoryTitle)+"\r\n"+copy.Text(txtNoHistory))
	} else {
		body := copy.Body(section, snapshot)
		if section == ShellHistory {
			body += "\r\n\r\n" + copy.Draft(snapshot)
		}
		setText(ctx.body, body)
		if key := shellPrimaryAction(section); key != "" {
			setText(ctx.detail, copy.Text(key))
		}
	}
}

func fmtPercent(value int) string {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	digits := []byte{'0'}
	if value >= 100 {
		digits = []byte("100")
	} else if value >= 10 {
		digits = []byte{byte('0' + value/10), byte('0' + value%10)}
	} else {
		digits[0] = byte('0' + value)
	}
	return ": " + string(digits) + "%"
}

func windowText(control windows.Handle) string {
	length, _, _ := pGetWindowTextLengthW.Call(uintptr(control))
	if length == 0 || length > 2000 {
		return ""
	}
	buffer := make([]uint16, length+1)
	written, _, _ := pGetWindowTextW.Call(uintptr(control), uintptr(unsafe.Pointer(&buffer[0])), length+1)
	if written == 0 {
		return ""
	}
	return windows.UTF16ToString(buffer[:written])
}

func boolWord(value bool) uintptr {
	if value {
		return 1
	}
	return 0
}

func mainWindowProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	ctx := mainCtx
	switch message {
	case wmCommand:
		if ctx == nil || ctx.shell == nil {
			break
		}
		id := int(wParam & 0xffff)
		sectionByID := map[int]ShellSection{
			idShellHome: ShellHome, idShellCreate: ShellCreate, idShellJoin: ShellJoin,
			idShellTry: ShellTryLocally, idShellHistory: ShellHistory, idShellSettings: ShellSettings,
		}
		if section, ok := sectionByID[id]; ok {
			ctx.shell.Select(section)
			ctx.render()
			return 0
		}
		actions := ctx.shell.Actions()
		snapshot := ctx.shell.Snapshot()
		switch id {
		case idShellOpen:
			showMainWindow(true)
		case idShellAction:
			switch ctx.shell.Section() {
			case ShellCreate:
				if actions.Create != nil {
					actions.Create(windowText(ctx.identityInput))
				}
			case ShellJoin:
				if actions.Join != nil {
					actions.Join(windowText(ctx.identityInput))
				}
			case ShellTryLocally:
				if snapshot.SelfTestAvailable && actions.TryLocally != nil {
					actions.TryLocally()
				}
			}
		case idShellRecord:
			if shellRecordingEnabled(snapshot) && actions.ToggleRecording != nil {
				actions.ToggleRecording()
			}
		case idShellCancel:
			if (snapshot.Recording == ShellRecordingActive || snapshot.Recording == ShellRecordingProcessing || shellLocalCaptureBusy(snapshot)) && actions.CancelRecording != nil {
				actions.CancelRecording()
			}
		case idShellCue:
			if actions.PlayBuiltinCue != nil {
				actions.PlayBuiltinCue()
			}
		case idShellFile:
			if actions.ChooseLocalFile != nil {
				actions.ChooseLocalFile()
			}
		case idShellDelete:
			if actions.DeleteLocalDraft != nil {
				actions.DeleteLocalDraft()
			}
		case idShellInput:
			if actions.SelectNextInput != nil {
				actions.SelectNextInput()
			}
		case idShellOutput:
			if actions.SelectNextOutput != nil {
				actions.SelectNextOutput()
			}
		case idShellRecovery:
			if actions.SaveRecovery != nil {
				if path := chooseWindowsRecoveryDestination(uintptr(hwnd), ctx.shell.Locale()); path != "" {
					actions.SaveRecovery(path)
				}
			}
		case idShellDraftNext:
			if actions.SelectNextPhaseOneDraft != nil {
				actions.SelectNextPhaseOneDraft()
			}
		case idShellRoute:
			if actions.SelectNextPhaseOneRoute != nil {
				actions.SelectNextPhaseOneRoute()
			}
		case idShellDelivery:
			if actions.SelectNextPhaseOneDelivery != nil {
				actions.SelectNextPhaseOneDelivery()
			}
		case idShellSend:
			if actions.SendSelectedDraft != nil {
				actions.SendSelectedDraft()
			}
		case idShellPhaseDelete:
			if actions.DeleteSelectedDraft != nil {
				actions.DeleteSelectedDraft()
			}
		case idShellOutgoingFile:
			if actions.ChooseOutgoingFile != nil {
				actions.ChooseOutgoingFile()
			}
		case idShellHistoryNext:
			if actions.SelectNextHistoryItem != nil {
				actions.SelectNextHistoryItem()
				setText(ctx.reportDetails, "")
			}
		case idShellHistoryDelete:
			if actions.DeleteSelectedHistoryItem != nil {
				actions.DeleteSelectedHistoryItem()
			}
		case idShellHistoryReplay:
			if actions.ReplaySelectedHistoryItem != nil {
				actions.ReplaySelectedHistoryItem()
			}
		case idShellHistoryBlock:
			if actions.BlockSelectedHistoryActor != nil {
				actions.BlockSelectedHistoryActor()
			}
		case idShellReportReason:
			if actions.SelectNextReportReason != nil {
				actions.SelectNextReportReason()
			}
		case idShellHistoryReport:
			if actions.ReportSelectedHistoryItem != nil {
				actions.ReportSelectedHistoryItem(windowText(ctx.reportDetails))
			}
		case idShellDND:
			if shellDNDEnabled(snapshot) && actions.SetDND != nil {
				next := ShellDNDMessagesOnly
				if snapshot.DND == ShellDNDMessagesOnly {
					next = ShellDNDAllowAll
				}
				actions.SetDND(next)
			}
		case idShellEnglish:
			ctx.shell.SetLocale(ShellEnglish)
		case idShellRussian:
			ctx.shell.SetLocale(ShellRussian)
		}
		ctx.render()
		return 0
	case wmTimer:
		if ctx != nil {
			ctx.render()
		}
		return 0
	case wmSize:
		if ctx != nil {
			ctx.layout()
		}
		return 0
	case wmDropFiles:
		if ctx != nil && ctx.shell != nil {
			snapshot := ctx.shell.Snapshot()
			if snapshot.SelfTestAvailable && !shellLocalCaptureBusy(snapshot) && snapshot.Recording != ShellRecordingActive && snapshot.Recording != ShellRecordingProcessing {
				if file, ok := windowsDroppedAudioFile(wParam); ok {
					if action := ctx.shell.Actions().AcceptDroppedFile; action != nil {
						action(file)
					}
				}
			} else {
				pDragFinish.Call(wParam)
			}
		}
		return 0
	case wmDPIChanged:
		if lParam != 0 {
			var suggested winRect
			pRtlMoveMemory.Call(
				uintptr(unsafe.Pointer(&suggested)), lParam, unsafe.Sizeof(suggested))
			pSetWindowPos.Call(uintptr(hwnd), 0, uintptr(suggested.left), uintptr(suggested.top),
				uintptr(suggested.right-suggested.left), uintptr(suggested.bottom-suggested.top), 0x0014)
		}
		if ctx != nil {
			ctx.updateFonts()
			ctx.layout()
		}
		return 0
	case wmGetMinMax:
		if lParam != 0 {
			var info minMaxInfo
			pRtlMoveMemory.Call(
				uintptr(unsafe.Pointer(&info)), lParam, unsafe.Sizeof(info))
			dpi := 96
			if ctx != nil {
				dpi = ctx.dpi()
			}
			info.minTrackSize = pointStruct{int32(dip(700, dpi)), int32(dip(500, dpi))}
			pRtlMoveMemory.Call(
				lParam, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
		}
		return 0
	case wmClose:
		pShowWindow.Call(uintptr(hwnd), swHide)
		return 0
	case wmCtlColorStatic:
		pSetBkMode.Call(wParam, transparentBk)
		pSetTextColor.Call(wParam, clrText)
		brush, _, _ := pGetSysColorBr.Call(colorWindow)
		return brush
	case wmDestroy:
		pDragAcceptFiles.Call(uintptr(hwnd), 0)
		pKillTimer.Call(uintptr(hwnd), mainRefreshTimer)
		mainHwnd = 0
		return 0
	}
	result, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
}

func windowsDroppedAudioFile(drop uintptr) (WindowsBrokeredAudioFile, bool) {
	defer pDragFinish.Call(drop)
	length, _, _ := pDragQueryFileW.Call(drop, 0, 0, 0)
	if length == 0 || length > 32767 {
		return WindowsBrokeredAudioFile{}, false
	}
	buffer := make([]uint16, length+1)
	written, _, _ := pDragQueryFileW.Call(drop, 0, uintptr(unsafe.Pointer(&buffer[0])), length+1)
	if written == 0 {
		return WindowsBrokeredAudioFile{}, false
	}
	path := windows.UTF16ToString(buffer)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return WindowsBrokeredAudioFile{}, false
	}
	return WindowsBrokeredAudioFile{
		DisplayName: filepath.Base(path), SizeBytes: info.Size(),
		Open: func() (io.ReadCloser, error) { return os.Open(path) },
	}, true
}

func showMainWindow(explicit bool) {
	if mainHwnd == 0 {
		return
	}
	pShowWindow.Call(uintptr(mainHwnd), swRestore)
	pUpdateWindow.Call(uintptr(mainHwnd))
	if explicit {
		pSetForegroundWin.Call(uintptr(mainHwnd))
	}
}

func mainOwnsMessage(message *msg) bool {
	if mainHwnd == 0 || message == nil {
		return false
	}
	if message.hwnd == mainHwnd {
		return true
	}
	child, _, _ := pIsChild.Call(uintptr(mainHwnd), uintptr(message.hwnd))
	return child != 0
}

func translateMainMessage(message *msg) bool {
	if !mainOwnsMessage(message) {
		return false
	}
	if mainAccel != 0 {
		translated, _, _ := pTranslateAcceleratorW.Call(uintptr(mainHwnd), uintptr(mainAccel), uintptr(unsafe.Pointer(message)))
		if translated != 0 {
			return true
		}
	}
	handled, _, _ := pIsDialogMessageW.Call(uintptr(mainHwnd), uintptr(unsafe.Pointer(message)))
	return handled != 0
}

func destroyMainWindow() {
	if mainHwnd != 0 {
		pDestroyWindow.Call(uintptr(mainHwnd))
	}
	if mainAccel != 0 {
		pDestroyAcceleratorTableW.Call(uintptr(mainAccel))
		mainAccel = 0
	}
	if mainCtx != nil {
		for _, font := range []windows.Handle{mainCtx.fonts.title, mainCtx.fonts.body, mainCtx.fonts.small} {
			if font != 0 {
				pDeleteObject.Call(uintptr(font))
			}
		}
	}
	mainCtx = nil
}
