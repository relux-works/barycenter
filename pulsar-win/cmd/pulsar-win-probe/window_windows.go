//go:build windows

package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"relux.works/duet/pulsar-win/internal/winprobe"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")

	pRegisterClassExW                 = user32.NewProc("RegisterClassExW")
	pCreateWindowExW                  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                   = user32.NewProc("DefWindowProcW")
	pGetMessageW                      = user32.NewProc("GetMessageW")
	pTranslateMessage                 = user32.NewProc("TranslateMessage")
	pDispatchMessageW                 = user32.NewProc("DispatchMessageW")
	pPostMessageW                     = user32.NewProc("PostMessageW")
	pPostQuitMessage                  = user32.NewProc("PostQuitMessage")
	pShowWindow                       = user32.NewProc("ShowWindow")
	pIsWindowVisible                  = user32.NewProc("IsWindowVisible")
	pSetForegroundWindow              = user32.NewProc("SetForegroundWindow")
	pSetWindowTextW                   = user32.NewProc("SetWindowTextW")
	pDestroyWindow                    = user32.NewProc("DestroyWindow")
	pSendMessageW                     = user32.NewProc("SendMessageW")
	pLoadCursorW                      = user32.NewProc("LoadCursorW")
	pLoadIconW                        = user32.NewProc("LoadIconW")
	pRegisterHotKey                   = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey                 = user32.NewProc("UnregisterHotKey")
	pCreatePopupMenu                  = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                      = user32.NewProc("AppendMenuW")
	pTrackPopupMenu                   = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                      = user32.NewProc("DestroyMenu")
	pGetCursorPos                     = user32.NewProc("GetCursorPos")
	pSetTimer                         = user32.NewProc("SetTimer")
	pKillTimer                        = user32.NewProc("KillTimer")
	pMessageBoxW                      = user32.NewProc("MessageBoxW")
	pMoveWindow                       = user32.NewProc("MoveWindow")
	pGetClientRect                    = user32.NewProc("GetClientRect")
	pSetWindowPos                     = user32.NewProc("SetWindowPos")
	pGetDpiForWindow                  = user32.NewProc("GetDpiForWindow")
	pGetDpiForSystem                  = user32.NewProc("GetDpiForSystem")
	pSetProcessDPIContext             = user32.NewProc("SetProcessDpiAwarenessContext")
	pAdjustWindowRectExForDPI         = user32.NewProc("AdjustWindowRectExForDpi")
	pShellNotifyIconW                 = shell32.NewProc("Shell_NotifyIconW")
	pGetModuleHandleW                 = kernel32.NewProc("GetModuleHandleW")
	pGetCurrentPackageFamilyName      = kernel32.NewProc("GetCurrentPackageFamilyName")
	pRtlMoveMemory                    = kernel32.NewProc("RtlMoveMemory")
	pWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	pWTSUnregisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
	pCreateFontW                      = gdi32.NewProc("CreateFontW")
	pDeleteObject                     = gdi32.NewProc("DeleteObject")
)

var currentApp *probeApp

const (
	wmCreate           = 0x0001
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmClose            = 0x0010
	wmGetMinMax        = 0x0024
	wmSetFont          = 0x0030
	wmQueryEndSession  = lifecycleWMQueryEndSession
	wmEndSession       = lifecycleWMEndSession
	wmCommand          = 0x0111
	wmTimer            = 0x0113
	wmPowerBroadcast   = lifecycleWMPowerBroadcast
	wmWTSSessionChange = lifecycleWMWTSSessionChange
	wmHotkey           = 0x0312
	wmApp              = 0x8000
	wmRButtonUp        = 0x0205
	wmLButtonUp        = 0x0202
	wmDPIChanged       = 0x02E0

	wmAppTray                 = wmApp + 1
	wmAppDevicesReady         = wmApp + 2
	wmAppPermissionReady      = wmApp + 3
	wmAppCaptureReady         = wmApp + 4
	wmAppCaptureStarted       = wmApp + 5
	wmAppCaptureTerminal      = wmApp + 6
	wmAppPickerTerminal       = wmApp + 7
	wmAppCleanupReady         = wmApp + 8
	wmAppLifecycleIdleCleanup = wmApp + 9
	wmAppLifecycleRearm       = wmApp + 10

	idRecordDefault  = 1001
	idRecordSelected = 1002
	idStop           = 1003
	idPicker         = 1004
	idHide           = 1005
	idDeviceList     = 1006
	idStatus         = 1007

	menuRecordDefault  = 2001
	menuRecordSelected = 2002
	menuStop           = 2003
	menuPicker         = 2004
	menuToggleWindow   = 2005
	menuQuit           = 2006
	menuForceQuit      = 2007

	hotkeyID             = 1
	destroyRetryTimer    = 0xD5
	lifecycleRetryTimer  = 0xD6
	notifyForThisSession = 0

	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsTabStop          = 0x00010000
	wsVScroll          = 0x00200000
	wsBorder           = 0x00800000
	wsExToolWindow     = 0x00000080
	cwUseDefault       = ^uintptr(0x7fffffff)
	swHide             = 0
	swShow             = 5
	swRestore          = 9
	idcArrow           = 32512
	idiApplication     = 32512
	colorWindow        = 5
	defaultCharset     = 1
	clearTypeQuality   = 5
	fwNormal           = 400

	modControl  = 0x0002
	modShift    = 0x0004
	modNoRepeat = 0x4000
	vkR         = 0x52

	lbAddString    = 0x0180
	lbResetContent = 0x0184
	lbGetCurSel    = 0x0188
	lbSetCurSel    = 0x0186
	lbnSelChange   = 1

	mfString       = 0x0000
	mfSeparator    = 0x0800
	mfGrayed       = 0x0001
	tmpReturnCmd   = 0x0100
	tmpRightButton = 0x0002

	nimAdd     = 0
	nimDelete  = 2
	nifMessage = 1
	nifIcon    = 2
	nifTip     = 4
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
	pt      point
	private uint32
}

type point struct{ x, y int32 }

type windowRect struct{ left, top, right, bottom int32 }

type minMaxInfo struct {
	reserved, maxSize, maxPosition, minTrackSize, maxTrackSize point
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             windows.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     windows.Handle
}

func (a *probeApp) createWindows() error {
	// Per-monitor v2 must be selected before any HWND is created. The package
	// targets Windows 10 2004+, where this context and the DPI APIs are present.
	if aware, _, callErr := pSetProcessDPIContext.Call(^uintptr(3)); aware == 0 { // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 == -4
		return fmt.Errorf("SetProcessDpiAwarenessContext(PMv2): %w", callErr)
	}
	hInstance, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, idcArrow)
	icon, _, _ := pLoadIconW.Call(0, idiApplication)
	classes := []struct {
		name string
		proc uintptr
	}{
		{name: "PulsarProbeLifecycle", proc: syscall.NewCallback(hiddenWindowProc)},
		{name: "PulsarProbeMain", proc: syscall.NewCallback(mainWindowProc)},
	}
	for _, class := range classes {
		name := utf16(class.name)
		wc := wndClassExW{cbSize: uint32(unsafe.Sizeof(wndClassExW{})), lpfnWndProc: class.proc, hInstance: windows.Handle(hInstance), hIcon: windows.Handle(icon), hIconSm: windows.Handle(icon), hCursor: windows.Handle(cursor), hbrBackground: windows.Handle(colorWindow + 1), lpszClassName: name}
		result, _, callErr := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if result == 0 && callErr != windows.ERROR_CLASS_ALREADY_EXISTS {
			return fmt.Errorf("RegisterClassExW(%s): %w", class.name, callErr)
		}
	}
	hidden, _, callErr := pCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(utf16("PulsarProbeLifecycle"))), uintptr(unsafe.Pointer(utf16("Pulsar Probe lifecycle"))), 0, 0, 0, 1, 1, 0, 0, hInstance, 0)
	if hidden == 0 {
		return fmt.Errorf("create hidden top-level lifecycle window: %w", callErr)
	}
	a.hidden = windows.Handle(hidden)
	dpi, _, _ := pGetDpiForSystem.Call()
	if dpi == 0 {
		dpi = probeBaseDPI
	}
	clientWidth, clientHeight := probeInitialClientSize(int(dpi))
	outer := windowRect{right: int32(clientWidth), bottom: int32(clientHeight)}
	pAdjustWindowRectExForDPI.Call(uintptr(unsafe.Pointer(&outer)), wsOverlappedWindow|wsVisible, 0, 0, dpi)
	main, _, callErr := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16("PulsarProbeMain"))), uintptr(unsafe.Pointer(utf16("Pulsar hardware verification"))), wsOverlappedWindow|wsVisible, cwUseDefault, cwUseDefault, uintptr(outer.right-outer.left), uintptr(outer.bottom-outer.top), 0, 0, hInstance, 0)
	if main == 0 {
		return fmt.Errorf("create visible picker-owner window: %w", callErr)
	}
	a.main = windows.Handle(main)
	a.addTrayIcon()
	return nil
}

func mainWindowProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	a := currentApp
	if a != nil {
		if result, suppressed := (confirmedShutdownMessageGate{shutdown: &a.shutdown}).enter(message, nil); suppressed {
			return result
		}
	}
	switch message {
	case wmCreate:
		if a == nil {
			return 0
		}
		hInstance, _, _ := pGetModuleHandleW.Call(0)
		create := func(class, text string, style uintptr, x, y, width, height, id int) windows.Handle {
			h, _, callErr := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(utf16(class))), uintptr(unsafe.Pointer(utf16(text))), style, uintptr(x), uintptr(y), uintptr(width), uintptr(height), uintptr(hwnd), uintptr(id), hInstance, 0)
			if h == 0 {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "control_create", SelectedAPIPath: "CreateWindowExW(" + class + ")", FailureCause: callErr.Error(), Fields: map[string]any{"controlId": id}})
			}
			return windows.Handle(h)
		}
		a.intro = create("STATIC", "Diagnostic verification tool — select an input, then exercise the packaged API paths. Ctrl+Shift+R toggles capture.", wsChild|wsVisible, 0, 0, 1, 1, 0)
		a.list = create("LISTBOX", "", wsChild|wsVisible|wsBorder|wsVScroll|wsTabStop, 0, 0, 1, 1, idDeviceList)
		a.recordDefaultControl = create("BUTTON", "Record default", wsChild|wsVisible|wsTabStop, 0, 0, 1, 1, idRecordDefault)
		a.recordSelectedControl = create("BUTTON", "Record selected", wsChild|wsVisible|wsTabStop, 0, 0, 1, 1, idRecordSelected)
		a.stopControl = create("BUTTON", "Stop", wsChild|wsVisible|wsTabStop, 0, 0, 1, 1, idStop)
		a.pickerControl = create("BUTTON", "Open picker", wsChild|wsVisible|wsTabStop, 0, 0, 1, 1, idPicker)
		a.hideControl = create("BUTTON", "Hide", wsChild|wsVisible|wsTabStop, 0, 0, 1, 1, idHide)
		a.status = create("STATIC", "Discovering input devices...", wsChild|wsVisible, 0, 0, 1, 1, idStatus)
		if !a.controlsCreated() {
			return ^uintptr(0)
		}
		a.updateUIFont(hwnd)
		a.layoutUI(hwnd)
		return 0
	case wmSize:
		if a != nil {
			a.layoutUI(hwnd)
		}
		return 0
	case wmDPIChanged:
		if lParam != 0 {
			var suggested windowRect
			pRtlMoveMemory.Call(uintptr(unsafe.Pointer(&suggested)), lParam, unsafe.Sizeof(suggested))
			pSetWindowPos.Call(uintptr(hwnd), 0, uintptr(suggested.left), uintptr(suggested.top), uintptr(suggested.right-suggested.left), uintptr(suggested.bottom-suggested.top), 0x0014)
		}
		if a != nil {
			a.updateUIFont(hwnd)
			a.layoutUI(hwnd)
		}
		return 0
	case wmGetMinMax:
		if lParam != 0 {
			dpi := probeWindowDPI(hwnd)
			width, height := probeMinimumWindowSize(dpi)
			outer := windowRect{right: int32(width), bottom: int32(height)}
			pAdjustWindowRectExForDPI.Call(uintptr(unsafe.Pointer(&outer)), wsOverlappedWindow|wsVisible, 0, 0, uintptr(dpi))
			var info minMaxInfo
			pRtlMoveMemory.Call(uintptr(unsafe.Pointer(&info)), lParam, unsafe.Sizeof(info))
			info.minTrackSize = point{x: outer.right - outer.left, y: outer.bottom - outer.top}
			pRtlMoveMemory.Call(lParam, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
		}
		return 0
	case wmCommand:
		if a == nil {
			return 0
		}
		id := int(wParam & 0xffff)
		notify := int((wParam >> 16) & 0xffff)
		switch id {
		case idRecordDefault:
			a.requestRecord(recordDefault)
		case idRecordSelected:
			a.requestRecord(recordSelected)
		case idStop:
			a.enqueue(waiterCommand{kind: "stop", reason: winprobe.ReasonUserStop})
		case idPicker:
			a.requestPicker()
		case idHide:
			a.hideMainWindow("button_window_hide")
		case idDeviceList:
			if notify == lbnSelChange {
				selection, _, _ := pSendMessageW.Call(uintptr(a.list), lbGetCurSel, 0, 0)
				a.mu.Lock()
				a.selected = int(selection)
				a.pendingMode = recordSelected
				a.mu.Unlock()
			}
		}
		return 0
	case wmClose:
		if a != nil {
			a.hideMainWindow("close_window_hide")
		}
		return 0
	case wmDestroy:
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func probeWindowDPI(hwnd windows.Handle) int {
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(hwnd))
	if dpi == 0 {
		return probeBaseDPI
	}
	return int(dpi)
}

func (a *probeApp) updateUIFont(hwnd windows.Handle) {
	dpi := probeWindowDPI(hwnd)
	if a.uiFont != 0 && a.uiFontDPI == dpi {
		return
	}
	font, _, _ := pCreateFontW.Call(
		uintptr(int32(-probeDIP(16, dpi))), 0, 0, 0, fwNormal,
		0, 0, 0, defaultCharset, 0, 0, clearTypeQuality, 0,
		uintptr(unsafe.Pointer(utf16("Segoe UI"))),
	)
	if font == 0 {
		return
	}
	old := a.uiFont
	a.uiFont = windows.Handle(font)
	a.uiFontDPI = dpi
	for _, control := range []windows.Handle{a.intro, a.list, a.status, a.recordDefaultControl, a.recordSelectedControl, a.stopControl, a.pickerControl, a.hideControl} {
		pSendMessageW.Call(uintptr(control), wmSetFont, font, 1)
	}
	if old != 0 {
		pDeleteObject.Call(uintptr(old))
	}
}

func (a *probeApp) layoutUI(hwnd windows.Handle) {
	var client windowRect
	if ok, _, _ := pGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client))); ok == 0 {
		return
	}
	layout := probeLayoutFor(probeWindowDPI(hwnd), int(client.right-client.left), int(client.bottom-client.top))
	move := func(control windows.Handle, rect probeRect) {
		pMoveWindow.Call(uintptr(control), uintptr(rect.X), uintptr(rect.Y), uintptr(rect.Width), uintptr(rect.Height), 1)
	}
	move(a.intro, layout.Intro)
	move(a.list, layout.Devices)
	move(a.recordDefaultControl, layout.RecordDefault)
	move(a.recordSelectedControl, layout.RecordSelected)
	move(a.stopControl, layout.Stop)
	move(a.pickerControl, layout.Picker)
	move(a.hideControl, layout.Hide)
	move(a.status, layout.Status)
}

func hiddenWindowProc(hwnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	a := currentApp
	if a != nil {
		if result, suppressed := (confirmedShutdownMessageGate{shutdown: &a.shutdown}).enter(message, nil); suppressed {
			return result
		}
	}
	switch message {
	case wmAppTray:
		if a != nil {
			if lParam == wmRButtonUp {
				a.showTrayMenu()
			}
			if lParam == wmLButtonUp {
				a.showMainWindow()
			}
		}
		return 0
	case wmHotkey:
		if a != nil && wParam == hotkeyID {
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultAttempt, Action: "hotkey_received", SelectedAPIPath: "WM_HOTKEY"})
			a.mu.Lock()
			active := a.captureOp != 0
			mode := a.pendingMode
			a.mu.Unlock()
			if !a.lifecycle.workAllowed() {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultBlocked, Action: "hotkey_action", SelectedAPIPath: "WM_HOTKEY", FailureCause: "a lifecycle start gate is closed"})
			} else if active {
				if !a.enqueue(waiterCommand{kind: "stop", reason: winprobe.ReasonUserStop, source: winprobe.ScenarioHotkey}) {
					a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultFail, Action: "hotkey_stop_queue", SelectedAPIPath: "WM_HOTKEY+command-event", FailureCause: "stop command could not be queued"})
				}
			} else {
				result := winprobe.ResultBlocked
				if a.requestRecord(mode) {
					result = winprobe.ResultPass
				}
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: result, Action: "hotkey_record_accepted", SelectedAPIPath: "WM_HOTKEY+explicit-Record"})
			}
		}
		return 0
	case wmAppDevicesReady:
		if a != nil {
			a.populateDevices()
		}
		return 0
	case wmAppPermissionReady:
		if a != nil {
			a.handlePermissionChecked(uint64(wParam), winprobe.PermissionStatus(lParam))
		}
		return 0
	case wmAppCaptureReady:
		if a != nil {
			a.activateCapture(uint64(wParam), uint32(lParam))
		}
		return 0
	case wmAppCaptureStarted:
		if a != nil {
			setText(a.status, "Recording continues independently when this window is hidden.")
		}
		return 0
	case wmAppCaptureTerminal:
		if a != nil {
			setText(a.status, "Capture terminal result recorded in scenarios.jsonl.")
		}
		return 0
	case wmAppPickerTerminal:
		if a != nil && lParam != 0 {
			a.hideMainWindow("picker_restore_window_hide")
		}
		return 0
	case wmAppCleanupReady:
		if a != nil {
			a.tryDestroyOnce()
		}
		return 0
	case wmAppLifecycleIdleCleanup:
		if a != nil {
			if _, current := a.uiTransitions.consume(uiTransitionIdleCleanup, uint64(wParam)); current {
				a.completeIdleLifecycleCleanups()
			}
		}
		return 0
	case wmAppLifecycleRearm:
		if a != nil {
			if transition, current := a.uiTransitions.consume(uiTransitionLifecycleRearm, uint64(wParam)); current {
				a.applyLifecycleRearm(transition.Generation, transition.Status, "permission_access_changed")
			}
		}
		return 0
	case wmTimer:
		if a != nil && wParam == destroyRetryTimer {
			pKillTimer.Call(uintptr(hwnd), destroyRetryTimer)
			a.tryDestroyOnce()
			return 0
		}
		if a != nil && wParam == lifecycleRetryTimer {
			pKillTimer.Call(uintptr(hwnd), lifecycleRetryTimer)
			a.completeIdleLifecycleCleanups()
			return 0
		}
	case wmQueryEndSession, wmEndSession, wmPowerBroadcast, wmWTSSessionChange:
		if a == nil {
			if message == wmQueryEndSession || message == wmPowerBroadcast {
				return 1
			}
			return 0
		}
		plan, observed := planLifecycleMessage(message, wParam)
		if !observed {
			r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
			return r
		}
		switch plan.Action {
		case lifecycleMessageStop:
			a.requestLifecycleStop(plan.Edge, plan.Signal, plan.Reason, plan.Mode)
		case lifecycleMessageResume:
			a.lifecycle.resume(plan.Edge)
			a.observeLifecycleResume(plan.Edge, plan.Signal)
		case lifecycleMessageShutdownCancelled:
			progress, err := a.lifecycle.cancelShutdown(plan.Signal)
			if err != nil {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultFail, Action: "lifecycle_shutdown_cancelled_state", SelectedAPIPath: plan.Signal, FailureCause: err.Error()})
			} else {
				a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultAttempt, Action: "lifecycle_shutdown_cancelled", SelectedAPIPath: plan.Signal, Fields: a.lifecycleFields(progress, map[string]any{"autoRestartCapture": false, "repeatedSignal": progress.RepeatedSignal, "repeatedSignalCount": progress.RepeatedSignalCount})})
				a.postIdleLifecycleCleanup()
			}
		case lifecycleMessageShutdownConfirmed:
			adapter := confirmedShutdownAdapter{shutdown: &a.shutdown, owners: &a.captureOwners}
			adapter.confirm(func(capture uint32) winprobe.HResult {
				return a.helper.CaptureStop(capture, plan.Reason)
			}, func() {
				_ = windows.SetEvent(a.shutdownEvent)
			})
		}
		if message == wmQueryEndSession || message == wmPowerBroadcast {
			return 1
		}
		return 0
	case wmClose:
		if a != nil {
			a.requestGracefulQuit("WM_CLOSE")
		}
		return 0
	case wmDestroy:
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

func (a *probeApp) registerHotkey(source string) bool {
	if a.evidence == nil || !a.evidence.healthy() {
		return false
	}
	a.mu.Lock()
	if a.hotkeyRegistered {
		a.mu.Unlock()
		return a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultPass, Action: "register_hotkey_idempotent", SelectedAPIPath: "RegisterHotKey(hidden-top-level-HWND)", Fields: map[string]any{"source": source, "apiCalled": false}})
	}
	a.mu.Unlock()
	result, _, callErr := pRegisterHotKey.Call(uintptr(a.hidden), hotkeyID, modControl|modShift|modNoRepeat, vkR)
	registered := result != 0
	if registered {
		a.mu.Lock()
		a.hotkeyRegistered = true
		a.mu.Unlock()
	}
	event := winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Action: "register_hotkey", SelectedAPIPath: "RegisterHotKey(hidden-top-level-HWND)", HotkeyRegistered: &registered, Fields: map[string]any{"source": source}}
	if registered {
		event.Result = winprobe.ResultPass
		event.HResult = "0x00000000"
	} else {
		event.Result = winprobe.ResultBlocked
		event.FailureCause = callErr.Error()
		event.Fields["getLastError"] = fmt.Sprintf("0x%08x", win32ErrorCode(callErr))
		event.Fields["nextAction"] = "choose a free shortcut or record the AppContainer RegisterHotKey failure as a platform no-go in the signed hardware matrix"
	}
	written := a.log(event)
	return registered && written
}

func (a *probeApp) addTrayIcon() {
	icon, _, _ := pLoadIconW.Call(0, idiApplication)
	data := notifyIconData{cbSize: uint32(unsafe.Sizeof(notifyIconData{})), hWnd: a.hidden, uID: 1, uFlags: nifMessage | nifIcon | nifTip, uCallbackMessage: wmAppTray, hIcon: windows.Handle(icon)}
	copy(data.szTip[:], windows.StringToUTF16("Pulsar packaged probe"))
	added, _, callErr := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	event := winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Action: "tray_icon_add", SelectedAPIPath: "Shell_NotifyIconW(hidden-top-level-HWND)"}
	if added != 0 {
		a.trayIcon.setOwned(true)
		event.Result = winprobe.ResultPass
		event.HResult = "0x00000000"
	} else {
		event.Result = winprobe.ResultBlocked
		event.FailureCause = callErr.Error()
		event.Fields = map[string]any{"getLastError": fmt.Sprintf("0x%08x", win32ErrorCode(callErr))}
	}
	a.log(event)
}

func (a *probeApp) removeTrayIcon() (released bool, apiCalled bool, lastError uint32) {
	var callErr error
	released, apiCalled = a.trayIcon.release(func() bool {
		data := notifyIconData{cbSize: uint32(unsafe.Sizeof(notifyIconData{})), hWnd: a.hidden, uID: 1}
		removed, _, err := pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
		callErr = err
		lastError = win32ErrorCode(err)
		return removed != 0
	})
	result := winprobe.ResultPass
	failure := ""
	if !released {
		result = winprobe.ResultFail
		failure = fmt.Sprintf("Shell_NotifyIconW(NIM_DELETE) failed: %v", callErr)
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: result, Action: "tray_icon_remove", SelectedAPIPath: "Shell_NotifyIconW(NIM_DELETE)", FailureCause: failure, Fields: map[string]any{"apiCalled": apiCalled, "ownershipRetained": !released, "getLastError": fmt.Sprintf("0x%08x", lastError)}})
	return released, apiCalled, lastError
}

func (a *probeApp) unregisterHotkey(source string) (unregistered bool, apiCalled bool, lastError uint32) {
	return a.unregisterHotkeyWithMode(source, false)
}

func (a *probeApp) unregisterHotkeyWithMode(source string, nonblockingEvidence bool) (unregistered bool, apiCalled bool, lastError uint32) {
	a.mu.Lock()
	registered := a.hotkeyRegistered
	a.mu.Unlock()
	if !registered {
		return true, false, 0
	}
	result, _, callErr := pUnregisterHotKey.Call(uintptr(a.hidden), hotkeyID)
	if result == 0 {
		return false, true, win32ErrorCode(callErr)
	}
	a.mu.Lock()
	a.hotkeyRegistered = false
	a.mu.Unlock()
	value := false
	event := winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultPass, Action: "unregister_hotkey", SelectedAPIPath: "UnregisterHotKey(hidden-top-level-HWND)", HotkeyRegistered: &value, Fields: map[string]any{"source": source, "apiCalled": true}}
	if nonblockingEvidence {
		a.logNonblocking(event)
	} else {
		a.log(event)
	}
	return true, true, 0
}

func (a *probeApp) registerSessionNotifications() bool {
	a.mu.Lock()
	if a.wtsRegistered {
		a.mu.Unlock()
		return true
	}
	a.mu.Unlock()
	registered, _, callErr := pWTSRegisterSessionNotification.Call(uintptr(a.hidden), notifyForThisSession)
	if registered != 0 {
		a.mu.Lock()
		a.wtsRegistered = true
		a.mu.Unlock()
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "session_notification_register", SelectedAPIPath: "WTSRegisterSessionNotification(NOTIFY_FOR_THIS_SESSION)", Fields: map[string]any{"getLastError": "0x00000000"}})
		return true
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultBlocked, Action: "session_notification_register", SelectedAPIPath: "WTSRegisterSessionNotification(NOTIFY_FOR_THIS_SESSION)", FailureCause: callErr.Error(), Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", win32ErrorCode(callErr)), "nextAction": "run the signed AppContainer probe on Windows 10/11; if registration or WTS_SESSION_LOCK delivery fails, record the lifecycle requirement as blocked/no-go"}})
	return false
}

func (a *probeApp) unregisterSessionNotifications(source string) (unregistered bool, apiCalled bool, lastError uint32) {
	a.mu.Lock()
	registered := a.wtsRegistered
	a.mu.Unlock()
	if !registered {
		return true, false, 0
	}
	result, _, callErr := pWTSUnregisterSessionNotification.Call(uintptr(a.hidden))
	if result == 0 {
		return false, true, win32ErrorCode(callErr)
	}
	a.mu.Lock()
	a.wtsRegistered = false
	a.mu.Unlock()
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultPass, Action: "session_notification_unregister", SelectedAPIPath: "WTSUnRegisterSessionNotification", Fields: map[string]any{"source": source, "apiCalled": true}})
	return true, true, 0
}

func (a *probeApp) showTrayMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	appendMenu(menu, mfString, menuRecordDefault, "Record default input")
	appendMenu(menu, mfString, menuRecordSelected, "Record selected input")
	appendMenu(menu, mfString, menuStop, "Stop")
	appendMenu(menu, mfString, menuPicker, "Open brokered picker")
	appendMenu(menu, mfString, menuToggleWindow, "Show / hide probe window")
	appendMenu(menu, mfSeparator, 0, "")
	appendMenu(menu, mfString, menuQuit, "Quit")
	if started := a.quittingAt.Load(); started != 0 && time.Now().UnixMilli()-started >= 5000 {
		appendMenu(menu, mfString, menuForceQuit, "Force Quit")
	}
	var cursor point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	pSetForegroundWindow.Call(uintptr(a.hidden))
	command, _, _ := pTrackPopupMenu.Call(menu, tmpReturnCmd|tmpRightButton, uintptr(cursor.x), uintptr(cursor.y), 0, uintptr(a.hidden), 0)
	pDestroyMenu.Call(menu)
	switch command {
	case menuRecordDefault:
		a.requestRecord(recordDefault)
	case menuRecordSelected:
		a.requestRecord(recordSelected)
	case menuStop:
		a.enqueue(waiterCommand{kind: "stop", reason: winprobe.ReasonUserStop})
	case menuPicker:
		a.requestPicker()
	case menuToggleWindow:
		requested := !isWindowVisible(a.main)
		if requested {
			a.showMainWindow()
		} else {
			a.hideMainWindow("tray_window_hide")
			break
		}
		visible := isWindowVisible(a.main)
		result := winprobe.ResultPass
		if visible != requested {
			result = winprobe.ResultFail
		}
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: result, Action: "tray_window_toggle", SelectedAPIPath: "tray-menu+ShowWindow", WindowVisible: &visible})
	case menuQuit:
		a.requestGracefulQuit("tray_menu_quit")
	case menuForceQuit:
		a.forceQuit()
	}
}

func (a *probeApp) populateDevices() {
	pSendMessageW.Call(uintptr(a.list), lbResetContent, 0, 0)
	a.mu.Lock()
	devices := append([]winprobe.Device(nil), a.devices...)
	a.mu.Unlock()
	for _, device := range devices {
		label := device.Name + " — " + device.ID
		pSendMessageW.Call(uintptr(a.list), lbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
	}
	if len(devices) > 0 {
		pSendMessageW.Call(uintptr(a.list), lbSetCurSel, 0, 0)
		setText(a.status, fmt.Sprintf("%d capture input(s). Default and selected paths are separate controls.", len(devices)))
	} else {
		setText(a.status, "No capture inputs were returned; selected capture is blocked.")
	}
}

func (a *probeApp) tryDestroyOnce() {
	if a.exitEvidenceRecorded {
		if !a.evidence.sync() {
			a.retryEvidenceSyncOrExit("process_exit_evidence_sync")
			return
		}
		a.evidenceRetries.reset()
		a.exit.commitQuit(func() {
			pKillTimer.Call(uintptr(a.hidden), destroyRetryTimer)
			pPostQuitMessage.Call(0)
		})
		return
	}

	unregistered, apiCalled, lastError := a.unregisterHotkey("graceful_quit")
	if !unregistered {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioHotkey, Result: winprobe.ResultBlocked, Action: "graceful_quit_hotkey_unregister", SelectedAPIPath: "UnregisterHotKey", FailureCause: "hotkey registration is still owned; capture helper destruction remains blocked", Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", lastError)}})
		a.scheduleDestroyRetry("hotkey_unregister")
		return
	}
	if !a.advanceLifecycle(lifecycleQuit, lifecycleHotkeyUnregistered, winprobe.ResultPass, "UnregisterHotKey(hidden-top-level-HWND)", 0, "", map[string]any{"apiCalled": apiCalled}) {
		a.scheduleDestroyRetry("hotkey_cleanup_order")
		return
	}

	unregistered, apiCalled, lastError = a.unregisterSessionNotifications("graceful_quit")
	if !unregistered {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultBlocked, Action: "graceful_quit_session_notification_unregister", SelectedAPIPath: "WTSUnRegisterSessionNotification", FailureCause: "session notification registration is still owned; capture helper destruction remains blocked", Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", lastError)}})
		a.scheduleDestroyRetry("session_notification_unregister")
		return
	}
	if !a.advanceLifecycle(lifecycleQuit, lifecycleSessionNotificationUnregistered, winprobe.ResultPass, "WTSUnRegisterSessionNotification", 0, "", map[string]any{"apiCalled": apiCalled}) {
		a.scheduleDestroyRetry("session_notification_cleanup_order")
		return
	}

	if a.helperLifetime.isInitialized() {
		hr := a.helper.Destroy()
		if hr != 0 {
			result := winprobe.ResultFail
			failure := "CapDestroy returned an unexpected failure"
			if uint32(hr) == 0x8000000e { // E_ILLEGAL_METHOD_CALL: a callback/thread still owns the fence.
				result = winprobe.ResultAttempt
				failure = ""
			}
			a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: result, Action: "CapDestroy_retry", SelectedAPIPath: "WM_TIMER", HResult: hr.Hex(), FailureCause: failure})
			a.scheduleDestroyRetry("CapDestroy")
			return
		}
		a.helperLifetime.clear()
	}
	if !a.advanceLifecycle(lifecycleQuit, lifecycleHelperDestroyed, winprobe.ResultPass, "CapDestroy", 0, "", nil) {
		a.scheduleDestroyRetry("helper_cleanup_order")
		return
	}
	trayRemoved, apiCalled, lastError := a.removeTrayIcon()
	if !trayRemoved {
		a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: winprobe.ResultBlocked, Action: "graceful_quit_tray_remove", SelectedAPIPath: "Shell_NotifyIconW(NIM_DELETE)", FailureCause: "tray icon ownership is retained until deletion succeeds", Fields: map[string]any{"getLastError": fmt.Sprintf("0x%08x", lastError)}})
		a.scheduleDestroyRetry("tray_icon_remove")
		return
	}
	if !a.advanceLifecycle(lifecycleQuit, lifecycleTrayIconRemoved, winprobe.ResultPass, "Shell_NotifyIconW(NIM_DELETE)", 0, "", map[string]any{"apiCalled": apiCalled}) {
		a.scheduleDestroyRetry("tray_icon_cleanup_order")
		return
	}
	if !a.syncEvidenceBeforeExit() {
		a.retryEvidenceSyncOrExit("evidence_sync")
		return
	}
	a.evidenceRetries.reset()
	if !a.advanceLifecycle(lifecycleQuit, lifecycleProcessExit, winprobe.ResultPass, "PostQuitMessage(after-cleanup)", 0, "", map[string]any{"temporaryArtifactsClosed": true, "hotkeyRegistered": false, "sessionNotificationsRegistered": false}) {
		return
	}
	a.exitEvidenceRecorded = true
	// Flush the process-exit-ready record itself before posting WM_QUIT.
	a.tryDestroyOnce()
}

func (a *probeApp) retryEvidenceSyncOrExit(step string) {
	retry, _ := a.evidenceRetries.recordFailure()
	if retry {
		a.scheduleDestroyRetry(step)
		return
	}
	a.commitForcedExit("bounded evidence retry exhausted during " + step)
}

func (a *probeApp) scheduleDestroyRetry(step string) {
	timer, _, _ := pSetTimer.Call(uintptr(a.hidden), destroyRetryTimer, 100, 0)
	if decideRetryTimer(timer, true) == retryTimerForceExit {
		a.commitForcedExit("SetTimer failed during " + step)
	}
}

func pumpMessages() error {
	var message msg
	for {
		result, _, callErr := pGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessageW: %w", callErr)
		}
		if result == 0 {
			return nil
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func postMessage(hwnd windows.Handle, message uint32, wParam, lParam uintptr) (bool, error) {
	result, _, callErr := pPostMessageW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return result != 0, callErr
}

func showWindow(hwnd windows.Handle, visible bool) {
	command := uintptr(swHide)
	if visible {
		command = swRestore
		if !isWindowVisible(hwnd) {
			command = swShow
		}
	}
	pShowWindow.Call(uintptr(hwnd), command)
}

func isWindowVisible(hwnd windows.Handle) bool {
	result, _, _ := pIsWindowVisible.Call(uintptr(hwnd))
	return result != 0
}
func setForegroundWindow(hwnd windows.Handle) bool {
	result, _, _ := pSetForegroundWindow.Call(uintptr(hwnd))
	return result != 0
}
func setText(hwnd windows.Handle, text string) {
	pSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(utf16(text))))
}
func destroyWindow(hwnd windows.Handle) bool {
	result, _, _ := pDestroyWindow.Call(uintptr(hwnd))
	return result != 0
}

func (a *probeApp) controlsCreated() bool {
	return a.intro != 0 && a.list != 0 && a.status != 0 &&
		a.recordDefaultControl != 0 && a.recordSelectedControl != 0 &&
		a.stopControl != 0 && a.pickerControl != 0 && a.hideControl != 0
}

func (a *probeApp) hideMainWindow(action string) bool {
	showWindow(a.main, false)
	visible := isWindowVisible(a.main)
	result := winprobe.ResultPass
	if visible {
		result = winprobe.ResultFail
	}
	a.mu.Lock()
	active := a.captureOp != 0
	a.mu.Unlock()
	if !visible {
		a.enqueue(waiterCommand{kind: "window_hidden"})
	}
	a.log(winprobe.LogEvent{Scenario: winprobe.ScenarioWindow, Result: result, Action: action, SelectedAPIPath: "ShowWindow(SW_HIDE)", WindowVisible: &visible, Fields: map[string]any{"captureActive": active}})
	return !visible
}

func (a *probeApp) showMainWindow() bool {
	showWindow(a.main, true)
	visible := isWindowVisible(a.main)
	if visible {
		a.enqueue(waiterCommand{kind: "window_shown"})
	}
	return visible
}
func utf16(value string) *uint16 { pointer, _ := windows.UTF16PtrFromString(value); return pointer }
func appendMenu(menu uintptr, flags, id uintptr, text string) {
	pAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(utf16(text))))
}

func messageBoxError(title, body string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16(body))), uintptr(unsafe.Pointer(utf16(title))), 0x10)
}

func win32ErrorCode(err error) uint32 {
	if errno, ok := err.(syscall.Errno); ok {
		return uint32(errno)
	}
	return uint32(windows.ERROR_GEN_FAILURE)
}
