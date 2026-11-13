//go:build windows

// Command ddprobe exercises a DD DLL through the GoDD wrapper and reports the
// observed cursor, mouse-button, and keyboard state.
//
// The probe is a diagnostic aid for real-device testing. It never installs or
// uninstalls the vendor driver.
//
// Examples:
//
//	go run ./cmd/ddprobe -dll .\dd.dll
//	go run ./cmd/ddprobe -dll .\dd.dll -mode keys
//	go run ./cmd/ddprobe -dll .\dd.dll -mode getdd
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/rpdg/godd"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procSetCursorPos       = user32.NewProc("SetCursorPos")
	procGetAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
)

const (
	vkLeftButton = 0x01
	vkControl    = 0x11
	vkA          = 0x41
)

type point struct {
	X, Y int32
}

func cursorPos() (int, int) {
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y)
}

func setCursorPos(x, y int) {
	procSetCursorPos.Call(uintptr(uint32(x)), uintptr(uint32(y)))
}

func keyDown(vk int) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

// cursorStable reports whether the cursor stays at (x,y) for 500ms.
func cursorStable(x, y int) bool {
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		gotX, gotY := cursorPos()
		if gotX != x || gotY != y {
			return false
		}
	}
	return true
}

func main() {
	dllPath := flag.String("dll", os.Getenv("GODD_DLL"), "path to the vendor DD DLL (or set GODD_DLL)")
	mode := flag.String("mode", "basic", "probe mode: basic, keys, or getdd")
	x := flag.Int("x", 600, "absolute X coordinate for the basic mouse test")
	y := flag.Int("y", 400, "absolute Y coordinate for the basic mouse test")
	wait := flag.Duration("wait", 3*time.Second, "wait before the mouse test so the user can stop moving the mouse")
	flag.Parse()

	procSetProcessDPIAware.Call()

	if *dllPath == "" {
		fmt.Fprintln(os.Stderr, "ddprobe: -dll is required (or set GODD_DLL)")
		flag.Usage()
		os.Exit(2)
	}

	switch *mode {
	case "basic":
		runBasic(*dllPath, *x, *y, *wait)
	case "keys":
		runKeys(*dllPath)
	case "getdd", "lifecycle":
		runGetDD(*dllPath)
	default:
		fmt.Fprintf(os.Stderr, "ddprobe: unknown mode %q (want basic, keys, or getdd)\n", *mode)
		os.Exit(2)
	}
}

func runBasic(dllPath string, targetX, targetY int, wait time.Duration) {
	fmt.Println("DLL:", dllPath)
	if wait > 0 {
		fmt.Printf("[info] waiting %v; do not move the mouse or click\n", wait)
		time.Sleep(wait)
	}

	startX, startY := cursorPos()
	if !cursorStable(startX, startY) {
		fmt.Println("[abort] cursor moved during the stability check; rerun when the mouse is idle")
		return
	}
	fmt.Printf("[info] cursor before test: (%d,%d)\n", startX, startY)

	dd, err := godd.GetDD(dllPath)
	if err != nil {
		failInit(err)
	}
	defer func() {
		if err := dd.Close(); err != nil {
			fmt.Println("[warn] Close:", err)
		}
	}()
	defer setCursorPos(startX, startY)

	// Control: prove that cursor queries and SetCursorPos work in this
	// session before interpreting DD results.
	setCursorPos(300, 300)
	time.Sleep(250 * time.Millisecond)
	ctrlX, ctrlY := cursorPos()
	fmt.Printf("[control] SetCursorPos(300,300) -> (%d,%d)\n", ctrlX, ctrlY)
	if ctrlX != 300 || ctrlY != 300 {
		fmt.Println("[abort] SetCursorPos control failed; check DPI awareness or input isolation")
		return
	}

	fmt.Printf("[test] MoveTo(%d,%d)\n", targetX, targetY)
	if err := dd.MoveTo(targetX, targetY); err != nil {
		fmt.Println("[fail] MoveTo:", err)
	} else {
		time.Sleep(800 * time.Millisecond)
		gotX, gotY := cursorPos()
		fmt.Printf("       cursor after MoveTo: (%d,%d)\n", gotX, gotY)
		if abs(gotX-targetX) <= 2 && abs(gotY-targetY) <= 2 {
			fmt.Println("[ok] MoveTo changed the cursor position")
		} else if gotX == ctrlX && gotY == ctrlY {
			fmt.Println("[warn] MoveTo returned success but the cursor did not move")
		} else {
			fmt.Println("[warn] cursor moved to an unexpected position; another input source may be active")
		}
	}

	beforeX, beforeY := cursorPos()
	fmt.Println("[test] MoveRelative(50,50)")
	if err := dd.MoveRelative(50, 50); err != nil {
		fmt.Println("[fail] MoveRelative:", err)
	} else {
		time.Sleep(800 * time.Millisecond)
		gotX, gotY := cursorPos()
		deltaX, deltaY := gotX-beforeX, gotY-beforeY
		fmt.Printf("       cursor after MoveRelative: (%d,%d), delta=(%d,%d)\n",
			gotX, gotY, deltaX, deltaY)
		switch {
		case abs(deltaX-50) <= 3 && abs(deltaY-50) <= 3:
			fmt.Println("[ok] MoveRelative changed the cursor position")
		case deltaX == 0 && deltaY == 0:
			fmt.Println("[warn] MoveRelative returned success but the cursor did not move")
		default:
			fmt.Println("[warn] cursor delta is not the requested (50,50); another input source may be active")
		}
	}

	fmt.Println("[test] mouse button left down/up")
	if keyDown(vkLeftButton) {
		fmt.Println("[abort] left mouse button is already down; release it and rerun")
		return
	}
	if err := dd.SetMouseButtonState(godd.BtnLeftDown); err != nil {
		fmt.Println("[fail] BtnLeftDown:", err)
	} else {
		time.Sleep(150 * time.Millisecond)
		fmt.Printf("       VK_LBUTTON down after BtnLeftDown: %v\n", keyDown(vkLeftButton))
	}
	if err := dd.SetMouseButtonState(godd.BtnLeftUp); err != nil {
		fmt.Println("[fail] BtnLeftUp:", err)
	} else {
		time.Sleep(150 * time.Millisecond)
		fmt.Printf("       VK_LBUTTON down after BtnLeftUp: %v\n", keyDown(vkLeftButton))
	}

	fmt.Println("[test] ScrollWheel(WheelForward)")
	if err := dd.ScrollWheel(godd.WheelForward); err != nil {
		fmt.Println("[fail] ScrollWheel:", err)
	} else {
		fmt.Println("[info] ScrollWheel returned success; scroll offset is not measured by this probe")
	}

	fmt.Println("[test] SendKeyEvent(VK_A)")
	if err := dd.SendKeyEvent(godd.VK_A, godd.KeyDown); err != nil {
		fmt.Println("[fail] VK_A down:", err)
	} else {
		time.Sleep(100 * time.Millisecond)
		fmt.Printf("       VK_A down: %v\n", keyDown(vkA))
	}
	if err := dd.SendKeyEvent(godd.VK_A, godd.KeyUp); err != nil {
		fmt.Println("[fail] VK_A up:", err)
	} else {
		time.Sleep(100 * time.Millisecond)
		fmt.Printf("       VK_A down after release: %v\n", keyDown(vkA))
	}

	if code, err := dd.ToDDCode(0x09); err != nil {
		fmt.Println("[skip] ToDDCode(VK_TAB):", err)
	} else {
		fmt.Printf("[ok] ToDDCode(VK_TAB) = %d\n", code)
	}

	if err := dd.Close(); err != nil {
		fmt.Println("[fail] Close:", err)
	} else {
		fmt.Println("[ok] Close")
	}
	if err := dd.MoveRelative(0, 0); !errors.Is(err, godd.ErrClosed) {
		fmt.Printf("[warn] MoveRelative after Close = %v, want ErrClosed\n", err)
	} else {
		fmt.Println("[ok] method after Close returns ErrClosed")
	}
}

func runKeys(dllPath string) {
	fmt.Println("DLL:", dllPath)

	dd, err := godd.GetDD(dllPath)
	if err != nil {
		failInit(err)
	}
	defer dd.Close()

	fmt.Println("[info] DD_todc mappings for common Windows VK codes:")
	for _, vk := range []int{0x11, 0xA2, 0xA3, 0x10, 0xA0, 0xA1, 0x41, 0x09, 0x24, 0x23, 0x0D, 0x20} {
		code, err := dd.ToDDCode(vk)
		if err != nil {
			fmt.Printf("  VK=0x%02X -> error: %v\n", vk, err)
			continue
		}
		fmt.Printf("  VK=0x%02X -> %d\n", vk, code)
	}

	fmt.Println("[test] hold VK_LCTRL and VK_A")
	if err := dd.SendKeyEvent(godd.VK_LCTRL, godd.KeyDown); err != nil {
		fmt.Println("[fail] VK_LCTRL down:", err)
		return
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("       VK_CONTROL=%v VK_LCONTROL=%v\n", keyDown(vkControl), keyDown(0xA2))

	if err := dd.SendKeyEvent(godd.VK_A, godd.KeyDown); err != nil {
		fmt.Println("[fail] VK_A down:", err)
	} else {
		time.Sleep(100 * time.Millisecond)
		fmt.Printf("       VK_CONTROL=%v VK_A=%v\n", keyDown(vkControl), keyDown(vkA))
	}

	_ = dd.SendKeyEvent(godd.VK_A, godd.KeyUp)
	time.Sleep(100 * time.Millisecond)
	_ = dd.SendKeyEvent(godd.VK_LCTRL, godd.KeyUp)
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("       after release: VK_CONTROL=%v VK_A=%v\n", keyDown(vkControl), keyDown(vkA))
}

func runGetDD(dllPath string) {
	fmt.Println("DLL:", dllPath)

	dd, err := godd.GetDD(dllPath)
	if err != nil {
		failInit(err)
	}
	fmt.Println("[ok] GetDD")
	if err := dd.MoveRelative(0, 0); err != nil {
		fmt.Println("[warn] MoveRelative(0,0):", err)
	}
	if err := dd.Close(); err != nil {
		fmt.Println("[fail] Close:", err)
		return
	}
	fmt.Println("[ok] Close")

	if _, err := godd.GetDD(dllPath); !errors.Is(err, godd.ErrClosed) {
		fmt.Printf("[fail] GetDD after Close = %v, want ErrClosed\n", err)
		os.Exit(1)
	}
	fmt.Println("[ok] GetDD after Close returns ErrClosed (singleton is terminal)")
}

func failInit(err error) {
	fmt.Fprintln(os.Stderr, "[fail] DD initialization:", err)
	fmt.Fprintln(os.Stderr, "If the code is 0, check whether another process holds the DD HID collections.")
	fmt.Fprintln(os.Stderr, "See docs/real-device-testing.md for the sharing-conflict checklist.")
	os.Exit(1)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
