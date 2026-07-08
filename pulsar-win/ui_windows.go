//go:build windows

// Win32 onboarding window + system tray (goal v2.1 F6), pure syscalls so the
// build stays CGO_ENABLED=0. This layer is PLUMBING around the tested helpers
// in ui_common.go; it could not be run on the build host — see UIPROBE.md for
// the on-hardware checklist. Every non-obvious flag is commented.
package main

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazyDLL("user32.dll")
	shell32  = windows.NewLazyDLL("shell32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")
	gdi32    = windows.NewLazyDLL("gdi32.dll")

	pCreateFontW   = gdi32.NewProc("CreateFontW")
	pSetTextColor  = gdi32.NewProc("SetTextColor")
	pSetBkMode     = gdi32.NewProc("SetBkMode")
	pSendMessageW  = user32.NewProc("SendMessageW")
	pGetSysColorBr = user32.NewProc("GetSysColorBrush")

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
	pMessageBoxW      = user32.NewProc("MessageBoxW")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pLoadIconW        = user32.NewProc("LoadIconW")
)

// appIcon loads the embedded RT_GROUP_ICON named "APP" (rsrc_windows_amd64.syso,
// generated from pulsar.ico by go-winres). Cached; 0 if the resource is
// missing (build without the syso) so the window/tray fall back to default.
var cachedIcon windows.Handle
var iconLoaded bool

func appIcon() windows.Handle {
	if iconLoaded {
		return cachedIcon
	}
	iconLoaded = true
	hInst, _, _ := pGetModuleHandleW.Call(0)
	h, _, _ := pLoadIconW.Call(hInst, uintptr(unsafe.Pointer(u16("APP"))))
	cachedIcon = windows.Handle(h)
	return cachedIcon
}

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
	// wmAppPairDone: posted back to the onboarding window by the pairing
	// goroutine (M6 — the HTTP exchange must not block the wndproc: the
	// window went "Not Responding" for up to 15 s and queued clicks replayed
	// against an already-used code).
	wmAppPairDone = wmApp + 2
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
	menuSoundCmd  = 2003 // "Как включить звук" — reopen the Spotify-step modal
	menuNoPulsar  = 2004 // "Не вижу Pulsar в Spotify?" — open the guide

	// MessageBoxW flags: MB_OK + an information icon. The OS renders the modal,
	// so Cyrillic and DPI are handled for us (unlike the hand-built window).
	mbOK              = 0x00000000
	mbIconInformation = 0x00000040
	mbSetForeground   = 0x00010000
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

// --- Fonts & colors: raw Win32 defaults to an ancient bitmap font; Segoe UI
// (the modern Windows UI font), ClearType-smoothed, is the whole difference
// between "Windows 3.1 dialog" and "clean modern app". ---

const (
	wmSetFont        = 0x0030
	wmCtlColorStatic = 0x0138
	transparentBk    = 1   // TRANSPARENT bk mode
	fwNormal         = 400 // FW_NORMAL
	fwSemibold       = 600 // FW_SEMIBOLD
	clearType        = 5   // CLEARTYPE_QUALITY
	defaultCharset   = 1   // DEFAULT_CHARSET (Unicode face names carry Cyrillic)
	clrText          = 0x001C1C1C
	clrSecondary     = 0x00767676
)

var (
	fontTitle, fontSubtitle, fontBody, fontHint, fontButton, fontCode windows.Handle
)

// mkFont makes a Segoe UI font. height is negative for character (point-ish)
// height; weight is FW_NORMAL/FW_SEMIBOLD.
func mkFont(height, weight int) windows.Handle {
	h, _, _ := pCreateFontW.Call(
		uintptr(int32(height)), 0, 0, 0, uintptr(weight),
		0, 0, 0, defaultCharset, 0, 0, clearType, 0,
		uintptr(unsafe.Pointer(u16("Segoe UI"))))
	return windows.Handle(h)
}

func ensureFonts() {
	if fontBody != 0 {
		return
	}
	fontTitle = mkFont(-30, fwSemibold)
	fontSubtitle = mkFont(-18, fwNormal)
	fontBody = mkFont(-19, fwNormal)
	fontHint = mkFont(-15, fwNormal)
	fontButton = mkFont(-19, fwNormal)
	fontCode = mkFont(-24, fwSemibold)
}

func setFont(control, font windows.Handle) {
	pSendMessageW.Call(uintptr(control), wmSetFont, uintptr(font), 1)
}

// --- Onboarding window ---------------------------------------------------

// onboardingCtx bridges the WndProc callback (a C-callable func without Go
// closures) to the running showOnboardingWindow call. Only one window is up
// at a time, so a package-level pointer is safe.
type onboardingCtx struct {
	dir, coordinator string
	hwnd             windows.Handle // top-level window, owner of the post-pair modal
	hEdit            windows.Handle
	hError           windows.Handle
	hSubmit          windows.Handle
	hSubtitle, hHint windows.Handle // rendered in secondary gray
	result           *Credentials
	done             bool

	// Async pairing handoff (M6): the goroutine writes under mu and posts
	// wmAppPairDone; the wndproc thread reads under mu. The Win32 queue orders
	// the events but is invisible to the Go memory model — hence the mutex.
	mu        sync.Mutex
	busy      bool
	pairCreds *Credentials
	pairErr   error
}

var curOnboarding *onboardingCtx

// onboardingClassReady: RegisterClassExW is once-per-process (a second call
// fails with ERROR_CLASS_ALREADY_EXISTS, which used to silently break every
// re-pair after a run that began unpaired — H5). Registering once also stops
// leaking a syscall.NewCallback per window (finite pool, never freed).
var onboardingClassReady bool

func showOnboardingWindow(dir, coordinatorBase string) (Credentials, error) {
	ctx := &onboardingCtx{dir: dir, coordinator: coordinatorBase}
	curOnboarding = ctx
	defer func() { curOnboarding = nil }()

	hInst, _, _ := pGetModuleHandleW.Call(0)
	className := u16("PulsarOnboarding")

	if !onboardingClassReady {
		cursor, _, _ := pLoadCursorW.Call(0, uintptr(idcArrow))
		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   syscall.NewCallback(onboardingProc),
			hInstance:     windows.Handle(hInst),
			hIcon:         appIcon(),
			hIconSm:       appIcon(),
			hCursor:       windows.Handle(cursor),
			hbrBackground: windows.Handle(colorWindow + 1),
			lpszClassName: className,
		}
		if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			return Credentials{}, err
		}
		onboardingClassReady = true
	}

	// Centered-ish window (CW_USEDEFAULT). Sized generously so the Cyrillic
	// intro (intro line + 3 numbered steps) and the hint never clip — the
	// first hardware run showed steps 2–3 cut off at 440x380 (UIPROBE).
	hwnd, _, err := pCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16(uiWindowTitle))),
		uintptr(wsCaption|wsSysMenu),
		cwUseDefault, cwUseDefault, 500, 540,
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
		ensureFonts()
		if ctx != nil {
			ctx.hwnd = hwnd // owner for the post-pair Spotify-step modal
		}
		// Client width ~484 (500 window minus borders); generous heights so
		// wrapped Cyrillic never clips.
		hTitle := mk("STATIC", uiTitleText, staticCenter, 20, 22, 444, 38, 0)
		hSub := mk("STATIC", uiIntroSubtitle, staticCenter, 20, 62, 444, 24, 0)
		// Intro grew a line (invite path) and the hint gained the guide URL;
		// both boxes get a touch more height so the wrap can't clip on hi-DPI.
		// Intro bottom (288) is flush with the hint top; hint bottom (340) still
		// clears the code field at y=344.
		hIntro := mk("STATIC", uiIntroText, wsChild|wsVisible, 28, 104, 428, 184, 0)
		hHint := mk("STATIC", uiNetworkHintText, wsChild|wsVisible, 28, 288, 428, 52, 0)
		setFont(hTitle, fontTitle)
		setFont(hSub, fontSubtitle)
		setFont(hIntro, fontBody)
		setFont(hHint, fontHint)
		if ctx != nil {
			ctx.hSubtitle, ctx.hHint = hSub, hHint
			ctx.hEdit = mk("EDIT", "", editStyle, 132, 344, 220, 34, idCodeEdit)
			ctx.hSubmit = mk("BUTTON", uiSubmitText, buttonStyle, 182, 392, 120, 38, idSubmit)
			ctx.hError = mk("STATIC", "", staticCenter, 20, 440, 444, 52, 0)
			setFont(ctx.hEdit, fontCode)
			setFont(ctx.hSubmit, fontButton)
			setFont(ctx.hError, fontBody)
		}
		return 0

	case wmCtlColorStatic:
		// Transparent text background + brand text colors (secondary gray for
		// the subtitle and the hint), returning the window's white brush.
		pSetBkMode.Call(wParam, transparentBk)
		color := uintptr(clrText)
		if ctx != nil && (windows.Handle(lParam) == ctx.hSubtitle || windows.Handle(lParam) == ctx.hHint) {
			color = clrSecondary
		}
		pSetTextColor.Call(wParam, color)
		br, _, _ := pGetSysColorBr.Call(colorWindow)
		return br

	case wmAppPairDone:
		if ctx != nil {
			onPairDone(ctx)
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
		// Quit only while an onboarding pump is live. The tray pump runs on
		// this SAME thread right after pairing — an unconditional quit here is
		// how closing a leftover onboarding window used to shut the whole node
		// down in the middle of the air (H5).
		if ctx != nil {
			pPostQuitMessage.Call(0)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func onSubmit(ctx *onboardingCtx) {
	code := getText(ctx.hEdit)
	if len(normalizePairCode(code)) != pairCodeLen {
		setText(ctx.hError, uiBadCodeError)
		return
	}
	ctx.mu.Lock()
	if ctx.busy {
		ctx.mu.Unlock()
		return // pairing already in flight; ignore the double click
	}
	ctx.busy = true
	ctx.mu.Unlock()
	setText(ctx.hSubmit, uiSubmitBusyText)
	setText(ctx.hError, "")
	// The HTTP exchange runs OFF the wndproc (M6): blocking here froze the
	// window ("Not Responding") for up to the 15 s client timeout and queued
	// clicks replayed afterwards, burning a second attempt on a used code.
	hwnd := ctx.hwnd
	go func() {
		creds, err := pairAndSave(ctx.dir, ctx.coordinator, code)
		ctx.mu.Lock()
		if err != nil {
			ctx.pairErr = err
		} else {
			ctx.pairCreds = &creds
		}
		ctx.mu.Unlock()
		// If the user closed the window meanwhile the post just fails — the
		// pump is gone and the caller already returned errWindowClosed.
		pPostMessageW.Call(uintptr(hwnd), wmAppPairDone, 0, 0)
	}()
}

// onPairDone runs on the wndproc thread after the pairing goroutine finishes.
func onPairDone(ctx *onboardingCtx) {
	ctx.mu.Lock()
	creds, err := ctx.pairCreds, ctx.pairErr
	ctx.pairCreds, ctx.pairErr = nil, nil
	ctx.busy = false
	ctx.mu.Unlock()
	if err != nil {
		setText(ctx.hSubmit, uiSubmitText)
		setText(ctx.hError, err.Error())
		return
	}
	ctx.result = creds
	ctx.done = true
	// #4: pairing linked the computer, but nothing plays until Spotify picks
	// "Pulsar" once. Surface that here, while the user is still looking, before
	// the window closes to the tray. Modal, so it blocks the quit until read.
	messageBox(ctx.hwnd, uiSpotifyStepTitle, uiSpotifyStepBody)
	// Destroy the window for real (H5): posting a bare quit left a zombie
	// window on screen that the tray pump kept painting — its close button
	// then killed the node (see wmDestroy). WM_DESTROY posts the quit that
	// ends the onboarding pump.
	pDestroyWindow.Call(uintptr(ctx.hwnd))
}

func setText(h windows.Handle, s string) {
	pSetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(s))))
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
		uFlags:           nifMessage | nifTip | nifIcon,
		uCallbackMessage: trayCallback,
		hIcon:            appIcon(),
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
		case menuSoundCmd:
			// Reprise of the post-pair Spotify step (#4), always available. The
			// tray window is message-only (invisible), so the modal owns itself
			// (owner 0) rather than parenting to a non-displayable window.
			messageBox(0, uiSpotifyStepTitle, uiSpotifyStepBody)
		case menuNoPulsar:
			// #6: firewall / same-Wi-Fi / VPN walkthrough lives in the guide.
			openURL(uiGuideURL)
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
	// #4/#6: the Spotify one-time step and the firewall/"can't see Pulsar" help
	// stay reachable for the whole run, not just at pairing.
	add(mfSeparator, 0, "")
	add(mfString, menuSoundCmd, uiMenuHowToSound)
	add(mfString, menuNoPulsar, uiMenuNoPulsar)
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

// messageBox shows a native modal (the Spotify-step "one more step" panel and
// its tray reprise). A native MessageBox is deliberately low-risk on the blind
// build: the OS owns layout, Cyrillic and DPI, so this can't clip the way a
// hand-built STATIC can. owner may be 0 (tray, no parent window).
func messageBox(owner windows.Handle, title, body string) {
	pMessageBoxW.Call(
		uintptr(owner),
		uintptr(unsafe.Pointer(u16(body))),
		uintptr(unsafe.Pointer(u16(title))),
		uintptr(mbOK|mbIconInformation|mbSetForeground))
}
