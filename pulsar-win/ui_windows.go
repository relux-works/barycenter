//go:build windows

// Win32 onboarding window + system tray (goal v2.1 F6), pure syscalls so the
// build stays CGO_ENABLED=0. This layer is PLUMBING around the tested helpers
// in ui_common.go; it could not be run on the build host — see UIPROBE.md for
// the on-hardware checklist. Every non-obvious flag is commented.
package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazyDLL("user32.dll")
	shell32  = windows.NewLazyDLL("shell32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")

	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pGetMessageW      = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pGetWindowTextW   = user32.NewProc("GetWindowTextW")
	pSetWindowTextW   = user32.NewProc("SetWindowTextW")
	pShowWindow       = user32.NewProc("ShowWindow")
	pUpdateWindow     = user32.NewProc("UpdateWindow")
	pLoadCursorW      = user32.NewProc("LoadCursorW")
	pPostMessageW     = user32.NewProc("PostMessageW")
	pCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	pAppendMenuW      = user32.NewProc("AppendMenuW")
	pTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	pGetCursorPos     = user32.NewProc("GetCursorPos")
	pSetForegroundWin = user32.NewProc("SetForegroundWindow")
	pDestroyMenu      = user32.NewProc("DestroyMenu")

	pShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW    = shell32.NewProc("ShellExecuteW")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// Win32 constants (only the ones we use).
const (
	wsOverlapped   = 0x00000000
	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsTabStop      = 0x00010000
	wsBorder       = 0x00800000
	wsExClientEdge = 0x00000200

	cwUseDefault = ^uintptr(0x7FFFFFFF) // 0x80000000 as int

	swShow = 5

	wmDestroy = 0x0002
	wmCommand = 0x0111
	wmApp     = 0x8000 // tray callback message base
	wmRButton = 0x0205 // WM_RBUTTONUP inside the tray lParam
	wmLButton = 0x0202 // WM_LBUTTONUP

	bnClicked = 0

	mfString    = 0x0000
	mfSeparator = 0x0800
	mfGrayed    = 0x0001

	tpmLeftAlign  = 0x0000
	tpmRightBtn   = 0x0002
	tpmReturnCmd  = 0x0100
	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nifMessage    = 0x00000001
	nifIcon       = 0x00000002
	nifTip        = 0x00000004
	trayCallback  = wmApp + 1
	idcArrow      = 32512
	colorWindow   = 5
	esCenter      = 0x0001
	essUppercase  = 0x0008 // ES_UPPERCASE
	editStyle     = wsChild | wsVisible | wsBorder | wsTabStop | esCenter | essUppercase
	buttonStyle   = wsChild | wsVisible | wsTabStop
	staticCenter  = wsChild | wsVisible | 0x0001 /* SS_CENTER */
	idSubmit      = 1001
	idCodeEdit    = 1002
	menuRePairCmd = 2001
	menuQuitCmd   = 2002
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type msg struct {
	hwnd    windows.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type pointStruct struct{ x, y int32 }

type notifyIconData struct {
	cbSize           uint32
	hWnd             windows.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
}

func u16(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

// --- Onboarding window ---------------------------------------------------

// onboardingCtx bridges the WndProc callback (a C-callable func without Go
// closures) to the running showOnboardingWindow call. Only one window is up
// at a time, so a package-level pointer is safe.
type onboardingCtx struct {
	dir, coordinator string
	hEdit            windows.Handle
	hError           windows.Handle
	hSubmit          windows.Handle
	result           *Credentials
	done             bool
}

var curOnboarding *onboardingCtx

func showOnboardingWindow(dir, coordinatorBase string) (Credentials, error) {
	ctx := &onboardingCtx{dir: dir, coordinator: coordinatorBase}
	curOnboarding = ctx
	defer func() { curOnboarding = nil }()

	hInst, _, _ := pGetModuleHandleW.Call(0)
	className := u16("PulsarOnboarding")
	cursor, _, _ := pLoadCursorW.Call(0, uintptr(idcArrow))

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   syscall.NewCallback(onboardingProc),
		hInstance:     windows.Handle(hInst),
		hCursor:       windows.Handle(cursor),
		hbrBackground: windows.Handle(colorWindow + 1),
		lpszClassName: className,
	}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return Credentials{}, err
	}

	// 420x340 centered-ish window (CW_USEDEFAULT position).
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16(uiWindowTitle))),
		uintptr(wsCaption|wsSysMenu),
		cwUseDefault, cwUseDefault, 440, 380,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return Credentials{}, err
	}
	pShowWindow.Call(hwnd, swShow)
	pUpdateWindow.Call(hwnd)
	pSetForegroundWin.Call(hwnd)

	pumpMessages()

	if ctx.result != nil {
		return *ctx.result, nil
	}
	return Credentials{}, errWindowClosed
}

var errWindowClosed = syscall.Errno(1223) // ERROR_CANCELLED — user closed it

func onboardingProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	ctx := curOnboarding
	switch message {
	case 0x0001: // WM_CREATE — build the child controls
		hInst, _, _ := pGetModuleHandleW.Call(0)
		mk := func(class, text string, style uint32, x, y, w, h int, id int) windows.Handle {
			h2, _, _ := pCreateWindowExW.Call(0,
				uintptr(unsafe.Pointer(u16(class))),
				uintptr(unsafe.Pointer(u16(text))),
				uintptr(style),
				uintptr(x), uintptr(y), uintptr(w), uintptr(h),
				uintptr(hwnd), uintptr(id), hInst, 0)
			return windows.Handle(h2)
		}
		mk("STATIC", uiTitleText, staticCenter, 20, 16, 400, 26, 0)
		mk("STATIC", uiIntroText, wsChild|wsVisible, 20, 50, 400, 96, 0)
		mk("STATIC", uiNetworkHintText, wsChild|wsVisible, 20, 150, 400, 34, 0)
		if ctx != nil {
			ctx.hEdit = mk("EDIT", "", editStyle, 120, 196, 200, 28, idCodeEdit)
			ctx.hSubmit = mk("BUTTON", uiSubmitText, buttonStyle, 160, 236, 120, 32, idSubmit)
			ctx.hError = mk("STATIC", "", staticCenter, 20, 276, 400, 40, 0)
		}
		return 0

	case wmCommand:
		id := wParam & 0xFFFF
		if id == idSubmit && ctx != nil {
			onSubmit(ctx)
			return 0
		}
		r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return r

	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func onSubmit(ctx *onboardingCtx) {
	code := getText(ctx.hEdit)
	if len(normalizePairCode(code)) != pairCodeLen {
		pSetWindowTextW.Call(uintptr(ctx.hError), uintptr(unsafe.Pointer(u16(uiBadCodeError))))
		return
	}
	pSetWindowTextW.Call(uintptr(ctx.hSubmit), uintptr(unsafe.Pointer(u16(uiSubmitBusyText))))
	creds, err := pairAndSave(ctx.dir, ctx.coordinator, code)
	if err != nil {
		pSetWindowTextW.Call(uintptr(ctx.hSubmit), uintptr(unsafe.Pointer(u16(uiSubmitText))))
		pSetWindowTextW.Call(uintptr(ctx.hError), uintptr(unsafe.Pointer(u16(err.Error()))))
		return
	}
	ctx.result = &creds
	ctx.done = true
	pPostQuitMessage.Call(0) // leave the message loop; caller has the creds
}

func getText(h windows.Handle) string {
	buf := make([]uint16, 256)
	pGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func pumpMessages() {
	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 { // 0 = WM_QUIT, -1 = error
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// --- System tray ---------------------------------------------------------

var curTray *TrayState
var trayHwnd windows.Handle

// awaitShutdown runs the tray's Win32 message loop as the main-thread
// blocker. It returns when the user picks Quit (trayProc posts WM_QUIT). The
// sig channel is unused on Windows — the tray owns shutdown.
func awaitShutdown(state *TrayState, sig <-chan struct{}) {
	runTrayLoop(state)
}

func runTrayLoop(state *TrayState) {
	curTray = state
	hInst, _, _ := pGetModuleHandleW.Call(0)
	className := u16("PulsarTray")
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   syscall.NewCallback(trayProc),
		hInstance:     windows.Handle(hInst),
		lpszClassName: className,
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	// Message-only window (HWND_MESSAGE = ^uintptr(2)) to receive tray events.
	hwnd, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(u16("PulsarTray"))),
		0, 0, 0, 0, 0, ^uintptr(2), 0, hInst, 0)
	trayHwnd = windows.Handle(hwnd)

	addTrayIcon(trayHwnd)
	pumpMessages()
	removeTrayIcon(trayHwnd)
}

func trayIconData(hwnd windows.Handle) notifyIconData {
	nid := notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifMessage | nifTip,
		uCallbackMessage: trayCallback,
	}
	tip := windows.StringToUTF16(uiWindowTitle)
	copy(nid.szTip[:], tip)
	return nid
}

func addTrayIcon(hwnd windows.Handle) {
	nid := trayIconData(hwnd)
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

func removeTrayIcon(hwnd windows.Handle) {
	nid := trayIconData(hwnd)
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func trayProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case trayCallback:
		// lParam low word is the mouse message. Right- or left-click both open
		// the menu — a tray with no default action should always be clickable.
		lw := lParam & 0xFFFF
		if lw == wmRButton || lw == wmLButton {
			showTrayMenu(hwnd)
		}
		return 0
	case wmCommand:
		switch wParam & 0xFFFF {
		case menuRePairCmd:
			if curTray != nil && curTray.OnRePair != nil {
				curTray.OnRePair()
			}
		case menuQuitCmd:
			if curTray != nil && curTray.OnQuit != nil {
				curTray.OnQuit()
			}
			pPostQuitMessage.Call(0)
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func showTrayMenu(hwnd windows.Handle) {
	menu, _, _ := pCreatePopupMenu.Call()
	add := func(flags uint32, id uintptr, text string) {
		pAppendMenuW.Call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(u16(text))))
	}
	// Status lines are disabled (informational).
	if curTray != nil {
		add(mfString|mfGrayed, 0, trayStatusLine(curTray.Connected()))
		if curTray.Identity != "" {
			add(mfString|mfGrayed, 0, curTray.Identity)
		}
		add(mfSeparator, 0, "")
		add(mfString, menuRePairCmd, uiMenuRepair)
	}
	add(mfSeparator, 0, "")
	add(mfString, menuQuitCmd, uiMenuQuit)

	var pt pointStruct
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWin.Call(uintptr(hwnd)) // so the menu dismisses on outside click
	pTrackPopupMenu.Call(menu, uintptr(tpmLeftAlign|tpmRightBtn), uintptr(pt.x), uintptr(pt.y), 0, uintptr(hwnd), 0)
	pDestroyMenu.Call(menu)
}

// openURL launches the default browser (guide/bot links from the window).
func openURL(url string) {
	pShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(u16("open"))),
		uintptr(unsafe.Pointer(u16(url))), 0, 0, swShow)
}
