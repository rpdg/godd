//go:build windows

// Command dde2e runs a small GUI integration test against a real DD DLL.
//
// It creates a test window with an edit control, exercises mouse and keyboard
// input, verifies the resulting focus/text/cursor state, and checks Close
// behavior. Use -skip-mouse for driver builds that accept mouse commands but
// do not inject them; keyboard and lifecycle checks still run.
//
// Examples:
//
//	go run ./cmd/dde2e -dll .\dd.dll
//	go run ./cmd/dde2e -dll .\ddhid.63340.dll -skip-mouse
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/rpdg/godd"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procUnregisterClassW         = user32.NewProc("UnregisterClassW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procPeekMessageW             = user32.NewProc("PeekMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procUpdateWindow             = user32.NewProc("UpdateWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procSetFocus                 = user32.NewProc("SetFocus")
	procGetFocus                 = user32.NewProc("GetFocus")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procSetCursorPos             = user32.NewProc("SetCursorPos")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsBorder           = 0x00800000
	wsTabStop          = 0x00010000
	wsExClientEdge     = 0x00000200
	esMultiline        = 0x0004
	esAutoHScroll      = 0x0080
	swShow             = 5
	swRestore          = 9
	colorWindow        = 5
	swpNoSize          = 0x0001
	swpNoMove          = 0x0002
	swpShowWindow      = 0x0040
	pmRemove           = 0x0001
	wmDestroy          = 0x0002
	wmClose            = 0x0010
	wmQuit             = 0x0012
)

const (
	hwndTopmost    = ^uintptr(0)
	hwndNotTopmost = ^uintptr(1)
)

type point struct {
	X, Y int32
}

type rect struct {
	Left, Top, Right, Bottom int32
}

type msg struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

func wndProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmClose:
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

func createTestWindow() (syscall.Handle, syscall.Handle, error) {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, err := syscall.UTF16PtrFromString("GoDDE2ETestWindow")
	if err != nil {
		return 0, 0, err
	}
	title, err := syscall.UTF16PtrFromString("GoDD E2E Test")
	if err != nil {
		return 0, 0, err
	}

	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:    syscall.NewCallback(wndProc),
		Instance:   syscall.Handle(hInstance),
		Background: colorWindow + 1,
		ClassName:  className,
	}

	atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return 0, 0, fmt.Errorf("RegisterClassExW: %v", callErr)
	}

	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow|wsVisible,
		300, 300, 560, 200,
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
		return 0, 0, fmt.Errorf("CreateWindowExW: %v", callErr)
	}

	editClass, err := syscall.UTF16PtrFromString("EDIT")
	if err != nil {
		procDestroyWindow.Call(hwnd)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
		return 0, 0, err
	}
	edit, _, callErr := procCreateWindowExW.Call(
		wsExClientEdge,
		uintptr(unsafe.Pointer(editClass)),
		0,
		wsChild|wsVisible|wsBorder|wsTabStop|esMultiline|esAutoHScroll,
		20, 20, 500, 50,
		hwnd, 1001, hInstance, 0,
	)
	if edit == 0 {
		procDestroyWindow.Call(hwnd)
		procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), hInstance)
		return 0, 0, fmt.Errorf("CreateWindowExW(EDIT): %v", callErr)
	}

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	return syscall.Handle(hwnd), syscall.Handle(edit), nil
}

func forceForeground(hwnd, focus syscall.Handle) bool {
	// Windows' foreground lock can reject SetForegroundWindow when another
	// application owns the foreground. Retry while pumping messages: the
	// attach-thread-input fallback often succeeds on the second attempt.
	for attempt := 0; attempt < 20; attempt++ {
		procShowWindow.Call(uintptr(hwnd), swRestore)
		procSetWindowPos.Call(uintptr(hwnd), hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
		procBringWindowToTop.Call(uintptr(hwnd))
		procSetForegroundWindow.Call(uintptr(hwnd))
		procSetFocus.Call(uintptr(focus))

		if foregroundAndFocus(hwnd, focus) {
			procSetWindowPos.Call(uintptr(hwnd), hwndNotTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
			return true
		}

		fg, _, _ := procGetForegroundWindow.Call()
		curTID, _, _ := procGetCurrentThreadID.Call()
		fgTID, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
		if fgTID != 0 && fgTID != curTID {
			procAttachThreadInput.Call(curTID, fgTID, 1)
			procBringWindowToTop.Call(uintptr(hwnd))
			procSetForegroundWindow.Call(uintptr(hwnd))
			procSetFocus.Call(uintptr(focus))
			procAttachThreadInput.Call(curTID, fgTID, 0)
		}
		pump(100 * time.Millisecond)

		if foregroundAndFocus(hwnd, focus) {
			procSetWindowPos.Call(uintptr(hwnd), hwndNotTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
			return true
		}
	}

	procSetWindowPos.Call(uintptr(hwnd), hwndNotTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	return false
}

func foregroundAndFocus(hwnd, focus syscall.Handle) bool {
	fg, _, _ := procGetForegroundWindow.Call()
	focusNow, _, _ := procGetFocus.Call()
	return syscall.Handle(fg) == hwnd && syscall.Handle(focusNow) == focus
}

func pump(d time.Duration) {
	deadline := time.Now().Add(d)
	var m msg
	for time.Now().Before(deadline) {
		for {
			ret, _, _ := procPeekMessageW.Call(
				uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove,
			)
			if ret == 0 {
				break
			}
			if m.Message == wmQuit {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func textOf(hwnd syscall.Handle) string {
	n, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	buf := make([]uint16, int(n)+1)
	procGetWindowTextW.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	return syscall.UTF16ToString(buf)
}

func centerOf(hwnd syscall.Handle) (int, int, error) {
	var r rect
	ret, _, callErr := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return 0, 0, fmt.Errorf("GetWindowRect: %v", callErr)
	}
	return int((int64(r.Left) + int64(r.Right)) / 2), int((int64(r.Top) + int64(r.Bottom)) / 2), nil
}

func cursorPos() (int, int, error) {
	var p point
	ret, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if ret == 0 {
		return 0, 0, fmt.Errorf("GetCursorPos: %v", callErr)
	}
	return int(p.X), int(p.Y), nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func must(err error, what string) {
	if err != nil {
		panic(fmt.Sprintf("%s: %v", what, err))
	}
}

func main() {
	runtime.LockOSThread()
	procSetProcessDPIAware.Call()

	dllPath := flag.String("dll", os.Getenv("GODD_DLL"), "path to the vendor DD DLL (or set GODD_DLL)")
	skipMouse := flag.Bool("skip-mouse", false, "skip mouse movement/click checks (keyboard-only run)")
	flag.Parse()

	if *dllPath == "" {
		fmt.Fprintln(os.Stderr, "dde2e: -dll is required (or set GODD_DLL)")
		flag.Usage()
		os.Exit(2)
	}
	if _, err := os.Stat(*dllPath); err != nil {
		fmt.Fprintf(os.Stderr, "dde2e: DLL not found: %v\n", err)
		os.Exit(2)
	}
	fmt.Println("DLL:", *dllPath)

	dd, err := godd.GetDD(*dllPath)
	must(err, "godd.GetDD")
	fmt.Println("[ok] godd.GetDD / DD_btn(0) initialization")

	ddClosed := false
	defer func() {
		if !ddClosed {
			_ = dd.Close()
		}
	}()

	origX, origY, err := cursorPos()
	must(err, "GetCursorPos")
	fmt.Printf("[info] original cursor position: (%d, %d)\n", origX, origY)
	defer func() {
		procSetCursorPos.Call(uintptr(uint32(origX)), uintptr(uint32(origY)))
	}()

	if *skipMouse {
		fmt.Println("[skip] mouse smoke tests (-skip-mouse)")
	} else {
		must(dd.MoveRelative(0, 0), "MoveRelative(0,0)")
		fmt.Println("[ok] DD_movR(0,0)")

		// DD_mov does not support the negative coordinates of the secondary
		// monitor on this machine, so use a positive primary-monitor position.
		must(dd.MoveTo(50, 50), "MoveTo(50,50)")
		fmt.Println("[ok] DD_mov(50,50)")
	}

	tabCode, err := dd.ToDDCode(0x09) // Windows VK_TAB
	if err != nil {
		fmt.Println("[skip] DD_todc:", err)
	} else {
		fmt.Printf("[ok] DD_todc(VK_TAB) = %d\n", tabCode)
	}

	hwnd, edit, err := createTestWindow()
	must(err, "createTestWindow")
	defer func() {
		procDestroyWindow.Call(uintptr(hwnd))
		pump(50 * time.Millisecond)
	}()

	fgOK := forceForeground(hwnd, edit)
	pump(150 * time.Millisecond)
	fg, _, _ := procGetForegroundWindow.Call()
	focus, _, _ := procGetFocus.Call()
	fmt.Printf("[info] foreground=%v focus-edit=%v\n",
		syscall.Handle(fg) == hwnd, syscall.Handle(focus) == edit)

	inputTested := false
	if !fgOK || syscall.Handle(fg) != hwnd || syscall.Handle(focus) != edit {
		fmt.Println("[skip] window is not foreground/focused; skipping mouse and keyboard input tests")
	} else {
		inputTested = true
		if *skipMouse {
			procSetFocus.Call(uintptr(edit))
			pump(50 * time.Millisecond)
			fmt.Println("[skip] mouse injection disabled by -skip-mouse")
		} else {
			// Mouse: move to the edit center and click. Focus should move to the edit.
			procSetFocus.Call(uintptr(hwnd))
			pump(50 * time.Millisecond)
			cx, cy, err := centerOf(edit)
			must(err, "GetWindowRect(edit)")
			fmt.Printf("[test] MoveTo(%d,%d) + Click\n", cx, cy)
			beforeX, beforeY, err := cursorPos()
			must(err, "GetCursorPos before MoveTo")
			must(dd.MoveTo(cx, cy), "MoveTo(edit center)")
			pump(300 * time.Millisecond)
			gotX, gotY, err := cursorPos()
			must(err, "GetCursorPos after MoveTo")
			fmt.Printf("[info] cursor (%d,%d) -> (%d,%d), target=(%d,%d)\n",
				beforeX, beforeY, gotX, gotY, cx, cy)
			if abs(gotX-cx) > 2 || abs(gotY-cy) > 2 {
				panic(fmt.Sprintf("MoveTo did not move the cursor: got (%d,%d), want (%d,%d)",
					gotX, gotY, cx, cy))
			}
			fmt.Println("[ok] MoveTo moved the cursor")
			must(dd.Click(), "Click")
			pump(200 * time.Millisecond)
			focus, _, _ = procGetFocus.Call()
			if syscall.Handle(focus) != edit {
				panic("click did not focus the edit control")
			}
			fmt.Println("[ok] mouse moved and click focused the edit control")
		}

		// Keyboard: DD_str input.
		procSetFocus.Call(uintptr(edit))
		pump(50 * time.Millisecond)
		const typed = "godd-e2e-123"
		must(dd.InputString(typed), "InputString")
		pump(300 * time.Millisecond)
		if got := textOf(edit); got != typed {
			panic(fmt.Sprintf("DD_str text mismatch: got %q, want %q", got, typed))
		}
		fmt.Println("[ok] DD_str typed:", typed)

		// Hotkey: Shift+Home selects to the start of the line, then typing
		// replaces the selection. Ctrl+A is not implemented by the standard
		// EDIT control on all Windows versions, so use a hotkey it supports.
		must(dd.PressHotkey(godd.VK_LSHIFT, godd.VK_HOME), "PressHotkey(Shift+Home)")
		pump(150 * time.Millisecond)
		const replaced = "godd-hotkey-456"
		must(dd.InputString(replaced), "InputString(replace)")
		pump(300 * time.Millisecond)
		if got := textOf(edit); got != replaced {
			panic(fmt.Sprintf("hotkey replacement mismatch: got %q, want %q", got, replaced))
		}
		fmt.Println("[ok] Shift+Home + DD_str replaced text:", replaced)

		// DD_todc + PressKey: Home + '>' and End + '<'.
		homeCode, err := dd.ToDDCode(0x24) // Windows VK_HOME
		must(err, "ToDDCode(VK_HOME)")
		must(dd.PressKey(homeCode, 20*time.Millisecond), "PressKey(Home)")
		pump(100 * time.Millisecond)
		must(dd.InputString(">"), "InputString(>)")
		pump(150 * time.Millisecond)
		wantAfterHome := ">" + replaced
		if got := textOf(edit); got != wantAfterHome {
			panic(fmt.Sprintf("Home test mismatch: got %q, want %q", got, wantAfterHome))
		}
		fmt.Println("[ok] ToDDCode(VK_HOME) + PressKey + DD_str:", wantAfterHome)

		endCode, err := dd.ToDDCode(0x23) // Windows VK_END
		must(err, "ToDDCode(VK_END)")
		must(dd.PressKey(endCode, 20*time.Millisecond), "PressKey(End)")
		pump(100 * time.Millisecond)
		must(dd.InputString("<"), "InputString(<)")
		pump(150 * time.Millisecond)
		wantFinal := wantAfterHome + "<"
		if got := textOf(edit); got != wantFinal {
			panic(fmt.Sprintf("End test mismatch: got %q, want %q", got, wantFinal))
		}
		fmt.Println("[ok] ToDDCode(VK_END) + PressKey + DD_str:", wantFinal)
	}

	// Lifecycle: Close and ensure methods reject further use.
	must(dd.Close(), "Close")
	ddClosed = true
	fmt.Println("[ok] DD.Close")
	if err := dd.MoveTo(origX, origY); !errors.Is(err, godd.ErrClosed) {
		panic(fmt.Sprintf("MoveTo after Close = %v, want ErrClosed", err))
	}
	fmt.Println("[ok] method after Close returns ErrClosed")

	// The singleton is terminal after Close: methods return ErrClosed and a
	// later GetDD returns ErrClosed without reloading the vendor DLL.
	finish := func() {
		if !inputTested {
			fmt.Println("INCOMPLETE: input checks were skipped because the test window could not be focused")
			os.Exit(1)
		}
		fmt.Println("ALL APPLICABLE INTEGRATION CHECKS PASSED")
	}

	if _, err := godd.GetDD(*dllPath); !errors.Is(err, godd.ErrClosed) {
		panic(fmt.Sprintf("GetDD after Close = %v, want ErrClosed", err))
	}
	fmt.Println("[ok] GetDD after Close returns ErrClosed")

	finish()
}
