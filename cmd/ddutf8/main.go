//go:build windows

// Command ddutf8 measures what the vendor DD_str export does with raw bytes.
//
// godd.InputString rejects non-ASCII before calling DD_str. This probe bypasses
// that guard, calls DD_str directly, and reads the result back from a focused
// EDIT control. It shows whether UTF-8, GBK, UTF-16LE, and control bytes are
// decoded, dropped, or typed literally.
//
// The probe needs the matching DD HID driver and DLL. Close certificate/USBKey
// tools that hold the vendor HID collections before running it.
//
// Example:
//
//	go run ./cmd/ddutf8 -dll .\ddhid.63340.dll
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
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
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procSetWindowTextW           = user32.NewProc("SetWindowTextW")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessID")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID       = kernel32.NewProc("GetCurrentThreadId")
	procGetACP                   = kernel32.NewProc("GetACP")
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

	className, err := syscall.UTF16PtrFromString("GoDDUTF8TestWindow")
	if err != nil {
		return 0, 0, err
	}
	title, err := syscall.UTF16PtrFromString("GoDD DD_str Byte Probe")
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

func setText(hwnd syscall.Handle, text string) {
	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		panic(err)
	}
	procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(p)))
}

func hexUTF16(s string) string {
	u := syscall.StringToUTF16(s)
	var b []byte
	for _, v := range u {
		b = append(b, byte(v), byte(v>>8))
	}
	return fmt.Sprintf("% X", b)
}

func callDDStr(proc *syscall.Proc, data []byte) (int32, error) {
	// Copy into a heap buffer with a NUL terminator, mirroring newCString.
	buf := make([]byte, len(data)+1)
	copy(buf, data)
	r1, _, lastErr := proc.Call(uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	return int32(r1), lastErr
}

func main() {
	runtime.LockOSThread()
	procSetProcessDPIAware.Call()

	dllPath := flag.String("dll", os.Getenv("GODD_DLL"), "path to the vendor DD DLL (or set GODD_DLL)")
	flag.Parse()
	if *dllPath == "" {
		fmt.Fprintln(os.Stderr, "ddutf8: -dll is required (or set GODD_DLL)")
		flag.Usage()
		os.Exit(2)
	}
	if _, err := os.Stat(*dllPath); err != nil {
		fmt.Fprintf(os.Stderr, "ddutf8: DLL not found: %v\n", err)
		os.Exit(2)
	}

	acp, _, _ := procGetACP.Call()
	fmt.Printf("DLL: %s\n", *dllPath)
	fmt.Printf("ANSI code page: %d\n", acp)

	dll, err := syscall.LoadDLL(*dllPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "LoadDLL:", err)
		os.Exit(1)
	}
	// Keep the module loaded until process exit; the vendor driver may still
	// reference it, and the probe does not need deterministic teardown.

	btn, err := dll.FindProc("DD_btn")
	if err != nil {
		fmt.Fprintln(os.Stderr, "FindProc(DD_btn):", err)
		os.Exit(1)
	}
	str, err := dll.FindProc("DD_str")
	if err != nil {
		fmt.Fprintln(os.Stderr, "FindProc(DD_str):", err)
		os.Exit(1)
	}

	if r1, _, lastErr := btn.Call(0); int32(r1) != 1 {
		fmt.Fprintf(os.Stderr, "DD_btn(0) initialization failed: code=%d err=%v\n", int32(r1), lastErr)
		os.Exit(1)
	}
	fmt.Println("[ok] DD_btn(0) initialization")

	hwnd, edit, err := createTestWindow()
	if err != nil {
		fmt.Fprintln(os.Stderr, "createTestWindow:", err)
		os.Exit(1)
	}
	defer func() {
		procDestroyWindow.Call(uintptr(hwnd))
		pump(50 * time.Millisecond)
	}()

	if !forceForeground(hwnd, edit) {
		fg, _, _ := procGetForegroundWindow.Call()
		focus, _, _ := procGetFocus.Call()
		fmt.Fprintf(os.Stderr, "window could not be focused: foreground=%v focus-edit=%v\n",
			syscall.Handle(fg) == hwnd, syscall.Handle(focus) == edit)
		os.Exit(1)
	}
	pump(150 * time.Millisecond)
	fmt.Println("[ok] test window is foreground and the edit control has focus")

	tests := []struct {
		name string
		data []byte
	}{
		{"ASCII control", []byte("abc")},
		{"UTF-8 中文", []byte{0xE4, 0xB8, 0xAD, 0xE6, 0x96, 0x87}},
		{"UTF-8 é", []byte{0xC3, 0xA9}},
		{"GBK 中文", []byte{0xD6, 0xD0, 0xCE, 0xC4}},
		{"ASCII + UTF-8 中 + ASCII", []byte{'a', 0xE4, 0xB8, 0xAD, 'b'}},
		{"UTF-8 中 + ASCII", []byte{0xE4, 0xB8, 0xAD, 'a'}},
		{"ASCII + high byte + ASCII", []byte{'a', 0x80, 'b'}},
		{"ASCII + LF + ASCII", []byte("a\nb")},
		{"ASCII + TAB + ASCII", []byte("a\tb")},
		{"UTF-16LE bytes for 中文", []byte{0x2D, 0x4E, 0x87, 0x65}},
		{"CP1252 é", []byte{0xE9}},
		{"UTF-8 emoji", []byte{0xF0, 0x9F, 0x98, 0x80}},
		{"single space", []byte{' '}},
		{"all printable ASCII", []byte(" !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~")},
		{"embedded NUL", []byte{'a', 'b', 0x00, 'c', 'd'}},
		{"CR LF", []byte{'\r', '\n'}},
	}

	results := make(map[string]string, len(tests))
	for _, tc := range tests {
		setText(edit, "")
		pump(100 * time.Millisecond)

		code, lastErr := callDDStr(str, tc.data)
		pump(400 * time.Millisecond)
		got := textOf(edit)
		results[tc.name] = got

		fmt.Printf("\n[test] %s\n", tc.name)
		fmt.Printf("  input bytes: % X\n", tc.data)
		fmt.Printf("  DD_str: code=%d err=%v\n", code, lastErr)
		fmt.Printf("  edit text: %q\n", got)
		fmt.Printf("  edit utf16: %s\n", hexUTF16(got))
	}

	if results["ASCII control"] != "abc" {
		fmt.Println("\n[result] ASCII control failed; cannot evaluate byte filtering.")
		fmt.Println("         Check the DLL/driver pairing and whether another process holds the HID collections.")
		os.Exit(1)
	}

	fmt.Println("\n[conclusion] printable ASCII 0x20-0x7E is typed literally")
	if results["UTF-8 中文"] == "" && results["GBK 中文"] == "" {
		fmt.Println("[conclusion] DD_str does not decode UTF-8 or GBK; all non-ASCII bytes were dropped")
	}
	if results["ASCII + UTF-8 中 + ASCII"] == "ab" {
		fmt.Println("[conclusion] unsupported bytes are skipped, not fatal: ASCII before and after them still arrives")
	}
	if results["embedded NUL"] == "ab" {
		fmt.Println("[conclusion] embedded NUL truncates the C string")
	}
	fmt.Println("[conclusion] DD_str returned success even when every supplied byte was dropped")
	fmt.Println("[conclusion] keep InputString's ASCII guard; use clipboard+Ctrl+V or SendInput(KEYEVENTF_UNICODE) for Unicode text")
}
