package godd

import "C"
import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// DD represents the DD driver wrapper
// It encapsulates DLL operations for mouse and keyboard simulation.

type DD struct {
	dll *syscall.DLL
	mu  sync.RWMutex
}

// ButtonState represents the state of a mouse button action.
// It is used to simulate pressing or releasing a mouse button.
type ButtonState int

// MouseButton constants
const (
	BtnLeftDown   ButtonState = 1  // Left mouse button pressed
	BtnLeftUp     ButtonState = 2  // Left mouse button released
	BtnRightDown  ButtonState = 4  // Right mouse button pressed
	BtnRightUp    ButtonState = 8  // Right mouse button released
	BtnMiddleDown ButtonState = 16 // Middle mouse button pressed
	BtnMiddleUp   ButtonState = 32 // Middle mouse button released
)

// WheelDirection represents the scrolling direction of the mouse wheel.
// It is used to simulate forward or backward wheel movements.
type WheelDirection int

// MouseWheel constants
const (
	WheelForward  WheelDirection = 1 // Scroll forward (typically upwards)
	WheelBackward WheelDirection = 2 // Scroll backward (typically downwards)
)

// KeyAction represents the state of a keyboard key action.
// It is used to simulate pressing or releasing a key.
type KeyAction int

// KeyState constants
const (
	KeyDown KeyAction = 1 // Key is pressed down
	KeyUp   KeyAction = 2 // Key is released
)

var (
	instance *DD
	once     sync.Once
	initErr  error
)

// GetDD initializes and returns a singleton instance of the DD driver.
// It loads the specified DLL and performs initialization. Subsequent calls
// will return the same instance. Thread-safe via sync.Once.
func GetDD(dllPath string) (*DD, error) {
	once.Do(func() {
		dll, err := syscall.LoadDLL(dllPath)
		if err != nil {
			initErr = fmt.Errorf("failed to load DD DLL: %w", err)
			return
		}

		instance = &DD{dll: dll}

		// initialize
		if err := instance.initialize(); err != nil {
			initErr = fmt.Errorf("initial failed: %w", err)
			instance = nil
			return
		}
	})
	return instance, initErr
}

// initializes the DD driver. Not intended for external use.
func (d *DD) initialize() error {
	proc, err := d.dll.FindProc("DD_btn")
	if err != nil {
		return fmt.Errorf("failed to find DD_btn procedure: %w", err)
	}

	ret, _, err := proc.Call(uintptr(0))
	if ret != 1 {
		return fmt.Errorf("failed to initialize DD driver")
	}

	return nil
}

// MoveTo moves the cursor to absolute screen coordinates (x, y).
// Returns an error if the DLL call fails or returns non-zero status.
func (d *DD) MoveTo(x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, err := d.dll.FindProc("DD_mov")
	if err != nil {
		return fmt.Errorf("failed to find DD_mov procedure: %w", err)
	}

	ret, _, err := proc.Call(uintptr(uint32(x)), uintptr(uint32(y)))
	return checkError(ret, err)
}

// MoveRelative moves the cursor by relative coordinates (dx, dy).
// Useful for smooth cursor movements. Thread-safe via mutex lock.
func (d *DD) MoveRelative(x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, err := d.dll.FindProc("DD_movR")
	if err != nil {
		return fmt.Errorf("failed to find DD_movR procedure: %w", err)
	}

	ret, _, err := proc.Call(uintptr(uint32(x)), uintptr(uint32(y)))
	return checkError(ret, err)
}

// SetMouseButtonState simulates a mouse button press/release action.
// Accepts ButtonState constants (e.g., BtnLeftDown, BtnRightUp).
// Returns error on API failure.
func (d *DD) SetMouseButtonState(buttonState ButtonState) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, err := d.dll.FindProc("DD_btn")
	if err != nil {
		return fmt.Errorf("failed to find DD_btn procedure: %w", err)
	}

	ret, _, err := proc.Call(uintptr(buttonState))
	return checkError(ret, err)
}

// Click simulates a left mouse button click with default timing.
// Internally calls BtnLeftDown, waits 50ms, then BtnLeftUp.
// Suitable for basic click operations.
func (d *DD) Click() error {
	err := d.SetMouseButtonState(BtnLeftDown)
	if err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	err = d.SetMouseButtonState(BtnLeftUp)
	if err != nil {
		return err
	}
	return nil
}

// ScrollWheel simulates mouse wheel movement.
// Accepts WheelDirection (WheelForward or WheelBackward).
// May not work with applications expecting precise wheel delta values.
func (d *DD) ScrollWheel(direction WheelDirection) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, err := d.dll.FindProc("DD_whl")
	if err != nil {
		return fmt.Errorf("failed to find DD_btn procedure: %w", err)
	}

	ret, _, err := proc.Call(uintptr(direction))
	return checkError(ret, err)
}

// SendKeyEvent sends a key down/up event for a specific virtual key.
// Parameters:
//   - key:  Virtual key code (e.g., VK_A, VK_ENTER)
//   - state: KeyAction (KeyDown or KeyUp)
//
// Returns error if DLL call fails.
func (d *DD) SendKeyEvent(key VirtualKey, state KeyAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, err := d.dll.FindProc("DD_key")
	if err != nil {
		return fmt.Errorf("failed to find DD_key procedure: %w", err)
	}

	ret, _, err := proc.Call(uintptr(key), uintptr(state))
	return checkError(ret, err)
}

// PressKey simulates pressing and holding a key for a duration.
// Example: Hold VK_SHIFT for 100ms before releasing.
// The duration must be ≥10ms. Returns error on key state failure.
func (d *DD) PressKey(key VirtualKey, duration time.Duration) error {
	if err := d.SendKeyEvent(key, KeyDown); err != nil {
		return err
	}
	time.Sleep(duration)
	return d.SendKeyEvent(key, KeyUp)
}

// PressHotkey simulates pressing and releasing a combination of keys.
// Keys are pressed in order and released in reverse order.
// Example: PressHotkey(VK_CTRL, VK_C) sends Ctrl+C.
// Returns error if any key operation fails.
func (d *DD) PressHotkey(keys ...VirtualKey) error {
	for _, key := range keys {
		if e := d.SendKeyEvent(key, KeyDown); e != nil {
			return e
		}
	}
	time.Sleep(time.Millisecond * 50)

	for i := len(keys) - 1; i >= 0; i-- {
		if e := d.SendKeyEvent(keys[i], KeyUp); e != nil {
			return e
		}
	}
	return nil
}

// InputString simulates typing a string via the DD driver.
// Returns error on encoding failure or DLL error.
func (d *DD) InputString(s string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, err := d.dll.FindProc("DD_str")
	if err != nil {
		return fmt.Errorf("failed to find DD_str procedure: %w", err)
	}

	// encode to ASC-II
	b := []byte(s)
	cString := append(b, 0)

	ret, _, err := proc.Call(uintptr(unsafe.Pointer(&cString[0])))
	return checkError(ret, err)
}

// Close releases the DD driver
func (d *DD) Close() error {
	d.dll.Handle = 0
	return d.dll.Release()
}

func checkError(ret uintptr, err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) && errno != 0 && err.Error() != "A device attached to the system is not functioning." {
		return fmt.Errorf("API error: %w", err)
	}

	if ret != 0 {
		return fmt.Errorf("DLL error code: %d", ret)
	}

	return nil
}
