//go:build windows

package godd

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// ErrClosed is returned by DD methods after Close has been called, and by
// GetDD once the process-wide instance has been closed.
var ErrClosed = errors.New("godd: DD driver is closed")

// procCaller is the subset of syscall.Proc used by DD. It is an interface so
// the driver wrapper can be tested without loading the vendor DLL.
type procCaller interface {
	Call(a ...uintptr) (r1, r2 uintptr, lastErr error)
}

// DD represents the DD driver wrapper.
// It encapsulates DLL operations for mouse and keyboard simulation.
//
// A DD must be created with GetDD; the zero value is not usable.
type DD struct {
	dll *syscall.DLL

	btn  procCaller
	key  procCaller
	mov  procCaller
	movR procCaller
	whl  procCaller
	str  procCaller
	todc procCaller // optional: not present in every DD DLL version

	// mu serializes DLL calls and complete input sequences (Click, PressKey,
	// PressHotkey, InputString). It also guards closed.
	mu     sync.Mutex
	closed bool
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

// MouseWheel constants.
//
// DD_whl(1) scrolls forward/up and DD_whl(2) scrolls backward/down.
const (
	WheelForward  WheelDirection = 1 // Scroll forward (up, away from the user)
	WheelBackward WheelDirection = 2 // Scroll backward (down, toward the user)

	WheelUp   WheelDirection = WheelForward
	WheelDown WheelDirection = WheelBackward
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
	defaultMu sync.Mutex
	defaultDD *DD
)

// newDD loads the DD DLL at dllPath and initializes the driver.
//
// The DLL file name changes between official DD releases, so callers pass the
// actual path from the downloaded package rather than depending on a
// particular version.
//
// The DLL is loaded with syscall.LoadDLL and all exports are resolved up
// front. syscall.NewLazyDLL is not used because it has no unload API, while
// Close must be able to release the module.
func newDD(dllPath string) (*DD, error) {
	if dllPath == "" {
		return nil, errors.New("godd: DLL path is empty")
	}

	dll, err := syscall.LoadDLL(dllPath)
	if err != nil {
		return nil, fmt.Errorf("godd: load DD DLL %q: %w", dllPath, err)
	}

	d := &DD{dll: dll}
	if err := d.initialize(); err != nil {
		_ = dll.Release()
		return nil, err
	}
	return d, nil
}

// GetDD returns the process-wide DD instance, loading the DLL on first use.
//
// GetDD is the only public constructor. The DD driver controls the session's
// global cursor and keyboard state, and some official DLL builds allow only one
// successful initialization per process, so multiple instances are neither
// independent nor reliably possible. All input methods share one lock, which
// makes compound operations atomic across the whole process.
//
// The first successful call loads dllPath and initializes the driver. Later
// calls return the same instance and ignore dllPath. Failed initialization is
// not cached: a later call can retry with a corrected path.
//
// Close is terminal. After Close, GetDD returns ErrClosed and does not reload
// the vendor DLL; restart the process to use DD again.
func GetDD(dllPath string) (*DD, error) {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	if defaultDD != nil {
		if defaultDD.isClosed() {
			return nil, ErrClosed
		}
		return defaultDD, nil
	}

	d, err := newDD(dllPath)
	if err != nil {
		return nil, err
	}
	defaultDD = d
	return d, nil
}

func (d *DD) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// initialize resolves every required export and performs DD_btn(0).
func (d *DD) initialize() error {
	required := []struct {
		name string
		dst  *procCaller
	}{
		{"DD_btn", &d.btn},
		{"DD_key", &d.key},
		{"DD_mov", &d.mov},
		{"DD_movR", &d.movR},
		{"DD_whl", &d.whl},
		{"DD_str", &d.str},
	}

	for _, item := range required {
		proc, err := d.dll.FindProc(item.name)
		if err != nil {
			return fmt.Errorf("godd: find %s: %w", item.name, err)
		}
		*item.dst = proc
	}

	// DD_todc is optional. Older DLL versions may not export it; ToDDCode
	// returns a clear error in that case without breaking initialization.
	if proc, err := d.dll.FindProc("DD_todc"); err == nil {
		d.todc = proc
	}

	// DD_btn(0) initializes the driver. The vendor contract is 1 on success;
	// failures are negative codes (for example -7).
	ret, _, _ := d.btn.Call(0)
	if code := int32(ret); code != 1 {
		return fmt.Errorf("godd: DD driver initialization failed (code %d)", code)
	}
	return nil
}

// checkClosedLocked reports whether Close has been called.
// d.mu must be held by the caller.
func (d *DD) checkClosedLocked() error {
	if d.closed {
		return ErrClosed
	}
	return nil
}

// checkResult validates a DD action result.
//
// DD action functions return 1 on success. The third return value of
// syscall.Proc.Call is GetLastError; it is always non-nil and may contain a
// stale value after a successful call, so it must only be consulted when the
// primary return value indicates failure. The error text must never be
// inspected (it is localized by Windows).
func checkResult(name string, ret uintptr, lastErr error) error {
	code := int32(ret)
	if code == 1 {
		return nil
	}
	if errno, ok := lastErr.(syscall.Errno); ok && errno != 0 {
		return fmt.Errorf("godd: %s failed (code %d): %w", name, code, errno)
	}
	return fmt.Errorf("godd: %s failed (code %d)", name, code)
}

// MoveTo moves the cursor to absolute screen coordinates (x, y).
// It returns an error if the DLL call fails.
func (d *DD) MoveTo(x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}

	ret, _, lastErr := d.mov.Call(uintptr(uint32(x)), uintptr(uint32(y)))
	return checkResult("DD_mov", ret, lastErr)
}

// MoveRelative moves the cursor by relative coordinates (dx, dy).
// Negative values are supported. Thread-safe via mutex lock.
func (d *DD) MoveRelative(x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}

	ret, _, lastErr := d.movR.Call(uintptr(uint32(x)), uintptr(uint32(y)))
	return checkResult("DD_movR", ret, lastErr)
}

// SetMouseButtonState simulates a mouse button press/release action.
// Accepts ButtonState constants (e.g., BtnLeftDown, BtnRightUp).
// Returns an error on API failure.
func (d *DD) SetMouseButtonState(buttonState ButtonState) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}
	return d.setMouseButtonStateLocked(buttonState)
}

func (d *DD) setMouseButtonStateLocked(buttonState ButtonState) error {
	ret, _, lastErr := d.btn.Call(uintptr(buttonState))
	return checkResult("DD_btn", ret, lastErr)
}

// releaseMouseButtonLocked releases buttonState and retries once on failure.
// It reports success if either attempt succeeds, so callers do not retry the
// whole input sequence after the button has already been released.
func (d *DD) releaseMouseButtonLocked(buttonState ButtonState) error {
	firstErr := d.setMouseButtonStateLocked(buttonState)
	if firstErr == nil {
		return nil
	}

	retryErr := d.setMouseButtonStateLocked(buttonState)
	if retryErr == nil {
		return nil
	}
	return combineErrors(firstErr, []error{retryErr})
}

// Click simulates a left mouse button click with default timing.
// Internally calls BtnLeftDown, waits 50ms, then BtnLeftUp.
// A failed release is retried once; the click is reported as successful if
// either release attempt succeeds.
func (d *DD) Click() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}

	if err := d.setMouseButtonStateLocked(BtnLeftDown); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)

	return d.releaseMouseButtonLocked(BtnLeftUp)
}

// ScrollWheel simulates mouse wheel movement.
// Accepts WheelDirection (WheelForward/WheelUp or WheelBackward/WheelDown).
func (d *DD) ScrollWheel(direction WheelDirection) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}

	ret, _, lastErr := d.whl.Call(uintptr(direction))
	return checkResult("DD_whl", ret, lastErr)
}

// SendKeyEvent sends a key down/up event for a specific DD virtual key.
// Parameters:
//   - key:   DD key code (e.g., VK_A, VK_ENTER); not a Windows VK_* value
//   - state: KeyAction (KeyDown or KeyUp)
//
// Returns an error if the DLL call fails.
func (d *DD) SendKeyEvent(key VirtualKey, state KeyAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}
	return d.sendKeyEventLocked(key, state)
}

func (d *DD) sendKeyEventLocked(key VirtualKey, state KeyAction) error {
	ret, _, lastErr := d.key.Call(uintptr(key), uintptr(state))
	return checkResult("DD_key", ret, lastErr)
}

// releaseKeyLocked sends KeyUp for key and retries once on failure. It reports
// success if either attempt succeeds.
func (d *DD) releaseKeyLocked(key VirtualKey) error {
	firstErr := d.sendKeyEventLocked(key, KeyUp)
	if firstErr == nil {
		return nil
	}

	retryErr := d.sendKeyEventLocked(key, KeyUp)
	if retryErr == nil {
		return nil
	}
	return combineErrors(firstErr, []error{retryErr})
}

// PressKey simulates pressing and holding a key for a duration.
// Example: Hold VK_SHIFT for 100ms before releasing.
// A failed release is retried once; the press is reported as successful if
// either release attempt succeeds.
func (d *DD) PressKey(key VirtualKey, duration time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}

	if err := d.sendKeyEventLocked(key, KeyDown); err != nil {
		return err
	}
	time.Sleep(duration)

	return d.releaseKeyLocked(key)
}

// PressHotkey simulates pressing and releasing a combination of keys.
// Keys are pressed in order and released in reverse order.
// Example: PressHotkey(VK_CTRL, VK_C) sends Ctrl+C.
//
// If any key press fails, all keys already pressed are released before the
// error is returned. Each failed release is retried once. Remaining keys are
// still released even if a release ultimately fails, though that key may
// remain down.
func (d *DD) PressHotkey(keys ...VirtualKey) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	pressed := make([]VirtualKey, 0, len(keys))
	var pressErr error
	for _, key := range keys {
		if err := d.sendKeyEventLocked(key, KeyDown); err != nil {
			pressErr = err
			break
		}
		pressed = append(pressed, key)
	}

	if pressErr == nil {
		time.Sleep(50 * time.Millisecond)
	}

	var releaseErrs []error
	for i := len(pressed) - 1; i >= 0; i-- {
		if err := d.releaseKeyLocked(pressed[i]); err != nil {
			releaseErrs = append(releaseErrs, fmt.Errorf("godd: release key %d: %w", pressed[i], err))
		}
	}

	return combineErrors(pressErr, releaseErrs)
}

// InputString simulates typing a string via DD_str.
//
// DD_str is a single-byte API for visible characters and spaces. Only
// printable ASCII (0x20-0x7E) is supported; non-ASCII text must use a
// Unicode-capable input path instead. NUL bytes are rejected because DD_str
// takes a C string.
func (d *DD) InputString(s string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return err
	}
	if err := validateASCII(s); err != nil {
		return err
	}

	cString := newCString(s)
	ret, _, lastErr := d.str.Call(uintptr(unsafe.Pointer(cString)))
	// procCaller is an interface, so the //go:uintptrescapes directive on
	// (*syscall.Proc).Call is not visible at this call site. The buffer is
	// heap-allocated by newCString, and KeepAlive keeps it reachable until
	// DD_str has finished reading it.
	runtime.KeepAlive(cString)
	return checkResult("DD_str", ret, lastErr)
}

// newCString returns a heap-allocated, NUL-terminated copy of s.
//
// The call site in InputString converts the pointer to uintptr and passes it
// through an interface, so the compiler cannot keep the buffer alive or
// guarantee that a stack-allocated buffer would not move. noinline ensures the
// returned pointer forces the backing array onto the heap.
//
//go:noinline
func newCString(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

func validateASCII(s string) error {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0 {
			return fmt.Errorf("godd: InputString does not support NUL bytes (index %d)", i)
		}
		if c < 0x20 || c > 0x7e {
			return fmt.Errorf("godd: InputString only supports printable ASCII (0x20-0x7E); byte 0x%02X at index %d", c, i)
		}
	}
	return nil
}

// ToDDCode converts a standard Windows virtual-key code (VK_*) to a DD key
// code using the DLL's DD_todc export.
//
// Use this for keys that are not in the static VirtualKey table or when the
// caller already has a Windows VK_* value. The returned code can be passed to
// SendKeyEvent, PressKey, or PressHotkey.
func (d *DD) ToDDCode(vk int) (VirtualKey, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkClosedLocked(); err != nil {
		return 0, err
	}
	if d.todc == nil {
		return 0, errors.New("godd: DD_todc is not exported by the loaded DLL")
	}

	ret, _, lastErr := d.todc.Call(uintptr(uint32(vk)))
	code := int32(ret)
	if code < 0 {
		if errno, ok := lastErr.(syscall.Errno); ok && errno != 0 {
			return 0, fmt.Errorf("godd: DD_todc(%d) failed (code %d): %w", vk, code, errno)
		}
		return 0, fmt.Errorf("godd: DD_todc(%d) failed (code %d)", vk, code)
	}
	return VirtualKey(code), nil
}

// Close terminates the DD instance and releases the DLL. It is safe to call
// more than once.
//
// Deprecated: Close is normally unnecessary; the DLL remains loaded until
// process exit. It is retained for compatibility and for tests that need
// deterministic teardown.
//
// Close is terminal for the process-wide singleton returned by GetDD: the
// shared reference is kept in a closed state, so later GetDD calls return
// ErrClosed instead of reloading the vendor DLL. This matches official DLL
// builds that allow only one successful initialization per process.
//
// Close and GetDD both take defaultMu before d.mu. The shared reference is
// retained while the DLL is released, so GetDD cannot return an open instance
// after Close.
func (d *DD) Close() error {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true
	// Keep defaultDD pointing at this instance. GetDD sees that it is closed
	// and returns ErrClosed instead of silently reinitializing the singleton.

	if d.dll == nil {
		return nil
	}

	// Release before clearing Handle: syscall.DLL.Release calls
	// FreeLibrary(d.Handle), so clearing it first would leak the module.
	err := d.dll.Release()
	d.dll.Handle = 0
	return err
}

// multiError combines several errors while preserving their identity for
// errors.Is and errors.As.
type multiError struct {
	errs []error
}

func (m multiError) Error() string {
	parts := make([]string, 0, len(m.errs))
	for _, err := range m.errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	return strings.Join(parts, "; ")
}

func (m multiError) Is(target error) bool {
	for _, err := range m.errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (m multiError) As(target interface{}) bool {
	for _, err := range m.errs {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}

// Unwrap is used by Go 1.20+ errors.Is/errors.As; Is/As above cover older
// versions, including the module's declared minimum of Go 1.18.
func (m multiError) Unwrap() []error {
	return m.errs
}

func combineErrors(primary error, extra []error) error {
	errs := make([]error, 0, len(extra)+1)
	if primary != nil {
		errs = append(errs, primary)
	}
	for _, err := range extra {
		if err != nil {
			errs = append(errs, err)
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return multiError{errs: errs}
	}
}
