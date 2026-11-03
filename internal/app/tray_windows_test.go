//go:build windows

package app

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type winRect struct{ Left, Top, Right, Bottom int32 }
type iconIdentifier struct {
	Size   uint32
	Window uintptr
	ID     uint32
	GUID   [16]byte
}

type guiThreadInfo struct {
	Size, Flags                                        uint32
	Active, Focus, Capture, MenuOwner, MoveSize, Caret uintptr
	CaretRect                                          winRect
}
type mouseInput struct {
	X, Y              int32
	Data, Flags, Time uint32
	ExtraInfo         uintptr
}
type nativeInput struct {
	Type  uint32
	Mouse mouseInput
}

// Opt in on an interactive Windows desktop; all input targets this test's window.
func TestWindowsTrayLifecycle(t *testing.T) {
	if os.Getenv("HITBOX_TRAY_SMOKE") != "1" {
		t.Skip("set HITBOX_TRAY_SMOKE=1 for native desktop smoke test")
	}
	if runtime.GOARCH == "amd64" && (unsafe.Sizeof(notifyIconData{}) != 976 || unsafe.Sizeof(windowClass{}) != 80 || unsafe.Sizeof(windowMessage{}) != 48) {
		t.Fatal("Windows ABI layout mismatch")
	}
	opened, exiting := make(chan struct{}, 10), make(chan struct{}, 1)
	type readyResult struct {
		tray *windowsTray
		err  error
	}
	ready := make(chan readyResult, 1)
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tray, err := newWindowsTray(func() { opened <- struct{}{} }, func() { exiting <- struct{}{} })
		ready <- readyResult{tray, err}
		if err != nil {
			done <- err
			return
		}
		err = tray.loop()
		tray.dispose()
		done <- err
	}()
	var tray *windowsTray
	select {
	case r := <-ready:
		if r.err != nil {
			t.Fatal(r.err)
		}
		tray = r.tray
	case <-time.After(5 * time.Second):
		t.Fatal("tray startup timed out")
	}
	finished := false
	defer func() {
		if !finished {
			postMessage.Call(tray.window, 0x001f, 0, 0)
			postMessage.Call(tray.window, trayClose, 0, 0)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("tray cleanup timed out")
			}
		}
	}()
	send := trayUser32.NewProc("SendMessageW")
	identifier := iconIdentifier{Window: tray.window, ID: 1}
	identifier.Size = uint32(unsafe.Sizeof(identifier))
	rectForIcon := func() uintptr {
		var rect winRect
		result, _, _ := trayShell32.NewProc("Shell_NotifyIconGetRect").Call(uintptr(unsafe.Pointer(&identifier)), uintptr(unsafe.Pointer(&rect)))
		return result
	}
	if result := rectForIcon(); int32(result) != 0 {
		t.Fatalf("icon is not registered: HRESULT %x", result)
	}
	count, _, _ := trayUser32.NewProc("GetMenuItemCount").Call(tray.menu)
	if count != 2 {
		t.Fatalf("menu has %d items", count)
	}
	for i, want := range []string{nativeText("tray.open"), nativeText("tray.exit")} {
		var text [64]uint16
		trayUser32.NewProc("GetMenuStringW").Call(tray.menu, uintptr(i), uintptr(unsafe.Pointer(&text[0])), 64, 0x400)
		if got := syscall.UTF16ToString(text[:]); got != want {
			t.Fatalf("menu %d: %q", i, got)
		}
	}
	waitOpen := func() {
		t.Helper()
		select {
		case <-opened:
		case <-time.After(2 * time.Second):
			t.Fatal("open action not received")
		}
	}
	send.Call(tray.window, trayCallback, 0, (1<<16)|0x0202) // Left button up.
	send.Call(tray.window, trayCallback, 0, (1<<16)|0x0400) // NIN_SELECT.
	select {
	case <-opened:
		t.Fatal("single click opened the browser")
	case <-time.After(100 * time.Millisecond):
	}
	send.Call(tray.window, trayCallback, 0, (1<<16)|wmLeftDoubleClick)
	waitOpen()
	// Recreate just this icon, simulating Explorer's TaskbarCreated broadcast.
	data := tray.notification
	notifyIcon.Call(2, uintptr(unsafe.Pointer(&data)))
	send.Call(tray.window, uintptr(tray.taskbarCreated), 0, 0)
	if result := rectForIcon(); int32(result) != 0 {
		t.Fatal("tray icon was not restored")
	}
	var originalCursor winPoint
	getCursorPos.Call(uintptr(unsafe.Pointer(&originalCursor)))
	defer trayUser32.NewProc("SetCursorPos").Call(uintptr(originalCursor.X), uintptr(originalCursor.Y))
	thread, _, _ := trayUser32.NewProc("GetWindowThreadProcessId").Call(tray.window, 0)
	popup := func(index uintptr, anchor winPoint) {
		t.Helper()
		trayUser32.NewProc("SetCursorPos").Call(uintptr(anchor.X), uintptr(anchor.Y))
		// WM_CONTEXTMENU does not guarantee coordinates in the v4 callback's wParam.
		postMessage.Call(tray.window, trayCallback, 0, (1<<16)|wmContextMenu)
		visible := false
		var rect winRect
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			info := guiThreadInfo{}
			info.Size = uint32(unsafe.Sizeof(info))
			trayUser32.NewProc("GetGUIThreadInfo").Call(thread, uintptr(unsafe.Pointer(&info)))
			ok, _, _ := trayUser32.NewProc("GetMenuItemRect").Call(0, tray.menu, index, uintptr(unsafe.Pointer(&rect)))
			if ok != 0 && rect.Right > rect.Left && info.MenuOwner == tray.window && info.Flags&4 != 0 {
				visible = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !visible {
			t.Fatal("right-click popup did not appear")
		}
		if anchor.X < rect.Left-32 || anchor.X > rect.Right+32 || anchor.Y < rect.Top-64 || anchor.Y > rect.Bottom+64 {
			t.Fatalf("popup is not near cursor: cursor=%+v menu=%+v", anchor, rect)
		}
		t.Logf("popup near cursor: cursor=%+v menu=%+v", anchor, rect)
		// Click only after Windows confirms this test owns the active popup.
		trayUser32.NewProc("SetCursorPos").Call(uintptr((rect.Left+rect.Right)/2), uintptr((rect.Top+rect.Bottom)/2))
		inputs := [2]nativeInput{{Mouse: mouseInput{Flags: 2}}, {Mouse: mouseInput{Flags: 4}}}
		count, _, err := trayUser32.NewProc("SendInput").Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
		if count != 2 {
			t.Fatal("could not click test popup:", err)
		}
	}
	var anchors []winPoint
	monitorCallback := syscall.NewCallback(func(monitor, dc, rectPtr, data uintptr) uintptr {
		var info struct {
			Size          uint32
			Monitor, Work winRect
			Flags         uint32
		}
		info.Size = uint32(unsafe.Sizeof(info))
		ok, _, _ := trayUser32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
		if ok != 0 {
			t.Logf("monitor=%+v work=%+v", info.Monitor, info.Work)
			anchors = append(anchors,
				winPoint{(info.Work.Left + info.Work.Right) / 2, (info.Work.Top + info.Work.Bottom) / 2},
				winPoint{info.Work.Right - 30, info.Work.Bottom - 30})
		}
		return 1
	})
	trayUser32.NewProc("EnumDisplayMonitors").Call(0, 0, monitorCallback, 0)
	if len(anchors) == 0 {
		t.Fatal("no desktop monitors found")
	}
	for _, anchor := range anchors {
		popup(0, anchor)
		waitOpen()
	}
	popup(1, anchors[0])
	select {
	case <-exiting:
	case <-time.After(2 * time.Second):
		t.Fatal("menu exit action not received")
	}
	if result := rectForIcon(); int32(result) != 0 {
		t.Fatal("icon disappeared before graceful shutdown")
	}
	postMessage.Call(tray.window, trayClose, 0, 0)
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tray loop did not exit")
	}
	if result := rectForIcon(); int32(result) == 0 {
		t.Fatal("tray icon remained after exit")
	}
	select {
	case <-opened:
		t.Fatal("extra open action")
	default:
	}
	t.Log("Native icon, double click, Chinese popup actions, recreation and cleanup passed")
}
