//go:build windows

package main

import (
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
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pRtlMoveMemory            = kernel32.NewProc("RtlMoveMemory")
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

	swHide    = 0
	swRestore = 9

	mainRefreshTimer = 1

	idShellHome     = 3001
	idShellCreate   = 3002
	idShellJoin     = 3003
	idShellTry      = 3004
	idShellHistory  = 3005
	idShellSettings = 3006
	idShellAction   = 3010
	idShellRecord   = 3011
	idShellDND      = 3012
	idShellEnglish  = 3013
	idShellRussian  = 3014
	idShellOpen     = 3020

	bsPushButton = 0x00000000
	bsMultiline  = 0x00002000
	ssLeft       = 0x00000000

	fVirtKey = 0x01
	fShift   = 0x04
	fControl = 0x08
	vkComma  = 0xBC
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
	hwnd    windows.Handle
	shell   *WindowsShell
	nav     map[ShellSection]windows.Handle
	title   windows.Handle
	banner  windows.Handle
	body    windows.Handle
	home    [3]windows.Handle
	cards   [3]windows.Handle
	footer  windows.Handle
	detail  windows.Handle
	record  windows.Handle
	dnd     windows.Handle
	english windows.Handle
	russian windows.Handle
	all     []windows.Handle
	fonts   mainFonts
}

var (
	mainCtx        *mainWindowCtx
	mainHwnd       windows.Handle
	mainAccel      windows.Handle
	mainClassReady bool
)

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
	move(ctx.english, ShellRect{X: layout.Content.X, Y: layout.Body.Bottom() + gap, Width: dip(130, layout.DPI), Height: dip(42, layout.DPI)})
	move(ctx.russian, ShellRect{X: layout.Content.X + dip(142, layout.DPI), Y: layout.Body.Bottom() + gap, Width: dip(130, layout.DPI), Height: dip(42, layout.DPI)})
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
	if snapshot.Recording == ShellRecordingActive {
		banner += "\r\n" + copy.Text(txtRecordingHelp)
	}
	setText(ctx.banner, banner)
	recordText := copy.Text(txtStartRecording)
	if snapshot.Recording == ShellRecordingActive {
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
	detailEnabled := section != ShellTryLocally || snapshot.SelfTestAvailable
	pEnableWindow.Call(uintptr(ctx.detail), boolWord(detailEnabled))
	showControl(ctx.english, section == ShellSettings)
	showControl(ctx.russian, section == ShellSettings)

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
			copy.Text(txtDND)+": "+copy.DND(snapshot.DND)+"    "+copy.Text(txtVolume)+fmtPercent(snapshot.Volume)+
			"\r\n\r\n"+copy.Text(txtHistoryTitle)+"\r\n"+copy.Text(txtNoHistory))
	} else {
		setText(ctx.body, copy.Body(section, snapshot))
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
					actions.Create()
				}
			case ShellJoin:
				if actions.Join != nil {
					actions.Join()
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
		pKillTimer.Call(uintptr(hwnd), mainRefreshTimer)
		mainHwnd = 0
		return 0
	}
	result, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result
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
