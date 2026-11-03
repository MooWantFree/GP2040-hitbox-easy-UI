//go:build windows

package app

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	trayCallback      = 0x8001
	trayClose         = 0x8002
	trayOpenCommand   = 1
	trayExitCommand   = 2
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmContextMenu     = 0x007b
	wmCommand         = 0x0111
	wmTimer           = 0x0113
	wmLeftDoubleClick = 0x0203
	ninKeySelect      = 0x0401
)

var (
	trayUser32            = syscall.NewLazyDLL("user32.dll")
	trayShell32           = syscall.NewLazyDLL("shell32.dll")
	registerClass         = trayUser32.NewProc("RegisterClassExW")
	unregisterClass       = trayUser32.NewProc("UnregisterClassW")
	createWindow          = trayUser32.NewProc("CreateWindowExW")
	destroyWindow         = trayUser32.NewProc("DestroyWindow")
	defaultWindowProc     = trayUser32.NewProc("DefWindowProcW")
	registerWindowMessage = trayUser32.NewProc("RegisterWindowMessageW")
	getMessage            = trayUser32.NewProc("GetMessageW")
	translateMessage      = trayUser32.NewProc("TranslateMessage")
	dispatchMessage       = trayUser32.NewProc("DispatchMessageW")
	postMessage           = trayUser32.NewProc("PostMessageW")
	postQuitMessage       = trayUser32.NewProc("PostQuitMessage")
	createPopupMenu       = trayUser32.NewProc("CreatePopupMenu")
	appendMenu            = trayUser32.NewProc("AppendMenuW")
	destroyMenu           = trayUser32.NewProc("DestroyMenu")
	enableMenuItem        = trayUser32.NewProc("EnableMenuItem")
	trackPopupMenu        = trayUser32.NewProc("TrackPopupMenu")
	setForegroundWindow   = trayUser32.NewProc("SetForegroundWindow")
	getCursorPos          = trayUser32.NewProc("GetCursorPos")
	setTimer              = trayUser32.NewProc("SetTimer")
	killTimer             = trayUser32.NewProc("KillTimer")
	notifyIcon            = trayShell32.NewProc("Shell_NotifyIconW")
	destroyIcon           = trayUser32.NewProc("DestroyIcon")
)

type windowClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type winPoint struct{ X, Y int32 }
type windowMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          winPoint
	Private        uint32
}
type notifyIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}

type windowsTray struct {
	window, menu, icon, instance uintptr
	className                    *uint16
	procedure                    uintptr
	taskbarCreated               uint32
	notification                 notifyIconData
	registered, added, exiting   bool
	onOpen, onExit               func()
}

func winError(operation string, err error) error {
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("%s：%w", operation, err)
	}
	return fmt.Errorf("%s", operation)
}

// All creation, notification and menu operations belong to the window's OS thread.
func newWindowsTray(onOpen, onExit func()) (*windowsTray, error) {
	t := &windowsTray{onOpen: onOpen, onExit: onExit}
	fail := func(err error) (*windowsTray, error) { t.dispose(); return nil, err }
	var err error
	t.icon, err = createTrayIcon()
	if err != nil {
		return fail(err)
	}
	t.instance, _, err = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	if t.instance == 0 {
		return fail(winError(nativeText("tray.instance"), err))
	}
	t.className, _ = syscall.UTF16PtrFromString("HitboxMap.Tray")
	t.procedure = syscall.NewCallback(t.windowProc)
	class := windowClass{Procedure: t.procedure, Instance: t.instance, ClassName: t.className, Icon: t.icon, SmallIcon: t.icon}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, err := registerClass.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return fail(winError(nativeText("tray.registerWindow"), err))
	}
	t.registered = true
	taskbar, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	message, _, err := registerWindowMessage.Call(uintptr(unsafe.Pointer(taskbar)))
	if message == 0 {
		return fail(winError(nativeText("tray.registerTaskbar"), err))
	}
	t.taskbarCreated = uint32(message)
	title, _ := syscall.UTF16PtrFromString("Hitbox Map")
	// Hidden top-level windows receive TaskbarCreated after Explorer restarts.
	t.window, _, err = createWindow.Call(0, uintptr(unsafe.Pointer(t.className)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, t.instance, 0)
	if t.window == 0 {
		return fail(winError(nativeText("tray.window"), err))
	}
	t.menu, _, err = createPopupMenu.Call()
	if t.menu == 0 {
		return fail(winError(nativeText("tray.menu"), err))
	}
	for _, item := range []struct {
		id    uintptr
		label string
	}{{trayOpenCommand, nativeText("tray.open")}, {trayExitCommand, nativeText("tray.exit")}} {
		label, _ := syscall.UTF16PtrFromString(item.label)
		ok, _, err := appendMenu.Call(t.menu, 0, item.id, uintptr(unsafe.Pointer(label)))
		if ok == 0 {
			return fail(winError(nativeText("tray.menuItem"), err))
		}
	}
	trayUser32.NewProc("SetMenuDefaultItem").Call(t.menu, trayOpenCommand, 0)
	t.notification = notifyIconData{Window: t.window, ID: 1, Flags: 1 | 2 | 4 | 0x80, Callback: trayCallback, Icon: t.icon, Version: 4}
	t.notification.Size = uint32(unsafe.Sizeof(t.notification))
	tip, _ := syscall.UTF16FromString("Hitbox Map")
	copy(t.notification.Tip[:], tip)
	if err := t.addIcon(); err != nil {
		return fail(err)
	}
	return t, nil
}

func (t *windowsTray) addIcon() error {
	ok, _, err := notifyIcon.Call(0, uintptr(unsafe.Pointer(&t.notification)))
	if ok == 0 {
		return winError(nativeText("tray.desktopRequired"), err)
	}
	t.added = true
	ok, _, err = notifyIcon.Call(4, uintptr(unsafe.Pointer(&t.notification)))
	if ok == 0 {
		notifyIcon.Call(2, uintptr(unsafe.Pointer(&t.notification)))
		t.added = false
		return winError(nativeText("tray.interaction"), err)
	}
	return nil
}

func (t *windowsTray) command(id uintptr) {
	if t.exiting {
		return
	}
	switch id {
	case trayOpenCommand:
		go t.onOpen()
	case trayExitCommand:
		t.exiting = true
		enableMenuItem.Call(t.menu, trayOpenCommand, 1)
		enableMenuItem.Call(t.menu, trayExitCommand, 1)
		t.notification.Tip = [128]uint16{}
		tip, _ := syscall.UTF16FromString(nativeText("tray.exiting"))
		copy(t.notification.Tip[:], tip)
		notifyIcon.Call(1, uintptr(unsafe.Pointer(&t.notification)))
		go t.onExit()
	}
}

func (t *windowsTray) popup() {
	// NOTIFYICON_VERSION_4 does not define wParam coordinates for WM_CONTEXTMENU.
	// Read screen coordinates directly, including negative multi-monitor positions.
	var point winPoint
	if ok, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return
	}
	setForegroundWindow.Call(t.window)
	command, _, _ := trackPopupMenu.Call(t.menu, 0x0102, uintptr(point.X), uintptr(point.Y), 0, t.window, 0)
	// Required so dismissing the menu works on subsequent invocations too.
	postMessage.Call(t.window, 0, 0, 0)
	if command != 0 {
		t.command(command)
	} else {
		notifyIcon.Call(3, uintptr(unsafe.Pointer(&t.notification)))
	}
}

func (t *windowsTray) windowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if message == t.taskbarCreated && t.taskbarCreated != 0 {
		t.added = false
		if err := t.addIcon(); err != nil {
			setTimer.Call(window, 1, 1000, 0)
		}
		return 0
	}
	switch message {
	case trayCallback:
		if (lParam>>16)&0xffff != 1 {
			return 0
		}
		switch lParam & 0xffff {
		case wmLeftDoubleClick, ninKeySelect:
			t.command(trayOpenCommand)
		case wmContextMenu:
			t.popup()
		}
		return 0
	case wmCommand:
		t.command(wParam & 0xffff)
		return 0
	case wmClose:
		t.command(trayExitCommand)
		return 0
	case 0x0011:
		return 1 // WM_QUERYENDSESSION
	case 0x0016:
		if wParam != 0 {
			t.command(trayExitCommand)
		}
		return 0 // WM_ENDSESSION
	case wmTimer:
		if wParam == 1 {
			if err := t.addIcon(); err == nil {
				killTimer.Call(window, 1)
			}
		}
		return 0
	case trayClose:
		// The HTTP server has drained; remove the icon before destroying its window.
		if t.added {
			notifyIcon.Call(2, uintptr(unsafe.Pointer(&t.notification)))
			t.added = false
		}
		destroyWindow.Call(window)
		return 0
	case wmDestroy:
		postQuitMessage.Call(0)
		return 0
	}
	result, _, _ := defaultWindowProc.Call(window, uintptr(message), wParam, lParam)
	return result
}

func (t *windowsTray) loop() error {
	var message windowMessage
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return winError(nativeText("tray.messages"), err)
		}
		if result == 0 {
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func (t *windowsTray) dispose() {
	if t.added {
		notifyIcon.Call(2, uintptr(unsafe.Pointer(&t.notification)))
		t.added = false
	}
	if t.window != 0 {
		killTimer.Call(t.window, 1)
		destroyWindow.Call(t.window)
	}
	if t.menu != 0 {
		destroyMenu.Call(t.menu)
	}
	if t.registered {
		unregisterClass.Call(uintptr(unsafe.Pointer(t.className)), t.instance)
	}
	if t.icon != 0 {
		destroyIcon.Call(t.icon)
	}
	runtime.KeepAlive(t)
}

func runDesktop(url string, openOnStart bool, shutdown func(), stopped <-chan struct{}) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	open := func() {
		if err := openBrowser(url); err != nil {
			showError(nativeText("tray.browserFailed", url))
		}
	}
	tray, err := newWindowsTray(open, shutdown)
	if err != nil {
		return err
	}
	defer tray.dispose()
	loopDone := make(chan struct{})
	defer close(loopDone)
	go func() {
		select {
		case <-stopped:
			postMessage.Call(tray.window, trayClose, 0, 0)
		case <-loopDone:
		}
	}()
	if openOnStart {
		go open()
	}
	return tray.loop()
}
