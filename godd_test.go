//go:build windows

package godd

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

type fakeCall struct {
	args []uintptr
}

type fakeProc struct {
	calls []fakeCall
	fn    func(callIndex int, args ...uintptr) (uintptr, uintptr, error)
}

func (f *fakeProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	index := len(f.calls)
	f.calls = append(f.calls, fakeCall{args: append([]uintptr(nil), args...)})
	if f.fn != nil {
		return f.fn(index, args...)
	}
	return 1, 0, syscall.Errno(0)
}

func (f *fakeProc) recorded() [][]uintptr {
	result := make([][]uintptr, len(f.calls))
	for i, call := range f.calls {
		result[i] = call.args
	}
	return result
}

func TestCheckResult(t *testing.T) {
	negativeSeven := int32(-7)
	tests := []struct {
		name      string
		ret       uintptr
		lastErr   error
		wantError bool
		contains  string
	}{
		{
			name:    "success with zero last error",
			ret:     1,
			lastErr: syscall.Errno(0),
		},
		{
			name:    "success ignores stale last error",
			ret:     1,
			lastErr: syscall.Errno(31),
		},
		{
			name:      "failure does not mask device error",
			ret:       0,
			lastErr:   syscall.Errno(31),
			wantError: true,
			contains:  "code 0",
		},
		{
			name:      "negative failure code",
			ret:       uintptr(negativeSeven),
			lastErr:   syscall.Errno(0),
			wantError: true,
			contains:  "-7",
		},
		{
			name:      "failure wraps errno",
			ret:       0,
			lastErr:   syscall.Errno(5),
			wantError: true,
			contains:  "code 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkResult("DD_test", tt.ret, tt.lastErr)
			if tt.wantError && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
				t.Fatalf("error %q does not contain %q", err, tt.contains)
			}
		})
	}
}

func TestValidateASCII(t *testing.T) {
	valid := []string{"", "abc", "ABC 123 !@#$%^&*()", "~"}
	for _, value := range valid {
		if err := validateASCII(value); err != nil {
			t.Errorf("validateASCII(%q) returned %v", value, err)
		}
	}

	invalid := []string{
		"中文",
		"café",
		"a\x00b",
		"line\nbreak",
		"\t",
	}
	for _, value := range invalid {
		if err := validateASCII(value); err == nil {
			t.Errorf("validateASCII(%q) should fail", value)
		}
	}
}

func TestClickSequence(t *testing.T) {
	btn := &fakeProc{}
	d := &DD{btn: btn}

	if err := d.Click(); err != nil {
		t.Fatalf("Click returned %v", err)
	}

	want := [][]uintptr{{uintptr(BtnLeftDown)}, {uintptr(BtnLeftUp)}}
	if got := btn.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestClickRetriesRelease(t *testing.T) {
	upCalls := 0
	btn := &fakeProc{fn: func(_ int, args ...uintptr) (uintptr, uintptr, error) {
		if args[0] == uintptr(BtnLeftDown) {
			return 1, 0, syscall.Errno(0)
		}
		upCalls++
		if upCalls == 1 {
			return 0, 0, syscall.Errno(0)
		}
		return 1, 0, syscall.Errno(0)
	}}
	d := &DD{btn: btn}

	if err := d.Click(); err != nil {
		t.Fatalf("Click should succeed when the release retry succeeds, got %v", err)
	}

	want := [][]uintptr{
		{uintptr(BtnLeftDown)},
		{uintptr(BtnLeftUp)},
		{uintptr(BtnLeftUp)},
	}
	if got := btn.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestClickReportsErrorWhenReleaseRetriesFail(t *testing.T) {
	btn := &fakeProc{fn: func(_ int, args ...uintptr) (uintptr, uintptr, error) {
		if args[0] == uintptr(BtnLeftDown) {
			return 1, 0, syscall.Errno(0)
		}
		return 0, 0, syscall.Errno(5)
	}}
	d := &DD{btn: btn}

	err := d.Click()
	if err == nil {
		t.Fatal("expected both release attempts to fail")
	}
	if !errors.Is(err, syscall.Errno(5)) {
		t.Fatalf("error %v does not preserve the underlying errno", err)
	}

	want := [][]uintptr{
		{uintptr(BtnLeftDown)},
		{uintptr(BtnLeftUp)},
		{uintptr(BtnLeftUp)},
	}
	if got := btn.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestClickDoesNotReleaseWhenPressFails(t *testing.T) {
	btn := &fakeProc{fn: func(_ int, _ ...uintptr) (uintptr, uintptr, error) {
		return 0, 0, syscall.Errno(0)
	}}
	d := &DD{btn: btn}

	if err := d.Click(); err == nil {
		t.Fatal("expected press failure")
	}

	want := [][]uintptr{{uintptr(BtnLeftDown)}}
	if got := btn.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestPressKeyRetriesRelease(t *testing.T) {
	key := VirtualKey(600)
	upCalls := 0
	proc := &fakeProc{fn: func(_ int, args ...uintptr) (uintptr, uintptr, error) {
		if args[1] == uintptr(KeyDown) {
			return 1, 0, syscall.Errno(0)
		}
		upCalls++
		if upCalls == 1 {
			return 0, 0, syscall.Errno(0)
		}
		return 1, 0, syscall.Errno(0)
	}}
	d := &DD{key: proc}

	if err := d.PressKey(key, 0); err != nil {
		t.Fatalf("PressKey should succeed when the release retry succeeds, got %v", err)
	}

	want := [][]uintptr{
		{uintptr(key), uintptr(KeyDown)},
		{uintptr(key), uintptr(KeyUp)},
		{uintptr(key), uintptr(KeyUp)},
	}
	if got := proc.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestPressHotkeyReleasesPressedKeysOnFailure(t *testing.T) {
	const (
		ctrl = VirtualKey(600)
		alt  = VirtualKey(602)
		del  = VirtualKey(706)
	)

	key := &fakeProc{fn: func(index int, args ...uintptr) (uintptr, uintptr, error) {
		// Ctrl down, Alt down, Del down(fail), Alt up, Ctrl up.
		if index == 2 {
			return 0, 0, syscall.Errno(0)
		}
		return 1, 0, syscall.Errno(0)
	}}
	d := &DD{key: key}

	if err := d.PressHotkey(ctrl, alt, del); err == nil {
		t.Fatal("expected press failure")
	}

	want := [][]uintptr{
		{uintptr(ctrl), uintptr(KeyDown)},
		{uintptr(alt), uintptr(KeyDown)},
		{uintptr(del), uintptr(KeyDown)},
		{uintptr(alt), uintptr(KeyUp)},
		{uintptr(ctrl), uintptr(KeyUp)},
	}
	if got := key.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestPressHotkeyReleaseRetrySucceeds(t *testing.T) {
	const (
		ctrl = VirtualKey(600)
		alt  = VirtualKey(602)
	)

	key := &fakeProc{fn: func(index int, _ ...uintptr) (uintptr, uintptr, error) {
		if index == 2 { // first Alt up fails
			return 0, 0, syscall.Errno(5)
		}
		return 1, 0, syscall.Errno(0)
	}}
	d := &DD{key: key}

	if err := d.PressHotkey(ctrl, alt); err != nil {
		t.Fatalf("PressHotkey should succeed when the release retry succeeds, got %v", err)
	}

	want := [][]uintptr{
		{uintptr(ctrl), uintptr(KeyDown)},
		{uintptr(alt), uintptr(KeyDown)},
		{uintptr(alt), uintptr(KeyUp)},
		{uintptr(alt), uintptr(KeyUp)},
		{uintptr(ctrl), uintptr(KeyUp)},
	}
	if got := key.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestPressHotkeyContinuesReleasingAfterReleaseError(t *testing.T) {
	const (
		ctrl = VirtualKey(600)
		alt  = VirtualKey(602)
	)

	key := &fakeProc{fn: func(index int, _ ...uintptr) (uintptr, uintptr, error) {
		if index == 2 || index == 3 { // both Alt up attempts fail
			return 0, 0, syscall.Errno(5)
		}
		return 1, 0, syscall.Errno(0)
	}}
	d := &DD{key: key}

	err := d.PressHotkey(ctrl, alt)
	if err == nil {
		t.Fatal("expected release failure")
	}
	if !errors.Is(err, syscall.Errno(5)) {
		t.Fatalf("error %v does not preserve the underlying errno", err)
	}

	want := [][]uintptr{
		{uintptr(ctrl), uintptr(KeyDown)},
		{uintptr(alt), uintptr(KeyDown)},
		{uintptr(alt), uintptr(KeyUp)},
		{uintptr(alt), uintptr(KeyUp)},
		{uintptr(ctrl), uintptr(KeyUp)},
	}
	if got := key.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected calls: got %v, want %v", got, want)
	}
}

func TestInputStringRejectsNonASCII(t *testing.T) {
	str := &fakeProc{}
	d := &DD{str: str}

	if err := d.InputString("中文"); err == nil {
		t.Fatal("expected non-ASCII input to fail")
	}
	if len(str.calls) != 0 {
		t.Fatalf("DD_str should not be called, got %v", str.recorded())
	}

	if err := d.InputString("hello"); err != nil {
		t.Fatalf("ASCII input failed: %v", err)
	}
	if len(str.calls) != 1 {
		t.Fatalf("expected one DD_str call, got %d", len(str.calls))
	}
}

func TestToDDCode(t *testing.T) {
	todc := &fakeProc{fn: func(_ int, _ ...uintptr) (uintptr, uintptr, error) {
		return 300, 0, syscall.Errno(0)
	}}
	d := &DD{todc: todc}

	code, err := d.ToDDCode(0x09)
	if err != nil {
		t.Fatalf("ToDDCode returned %v", err)
	}
	if code != VirtualKey(300) {
		t.Fatalf("got %d, want 300", code)
	}

	missing := &DD{}
	if _, err := missing.ToDDCode(0x09); err == nil {
		t.Fatal("expected an error when DD_todc is unavailable")
	}

	failing := &fakeProc{fn: func(_ int, _ ...uintptr) (uintptr, uintptr, error) {
		negativeOne := int32(-1)
		return uintptr(negativeOne), 0, syscall.Errno(0)
	}}
	d = &DD{todc: failing}
	if _, err := d.ToDDCode(0x09); err == nil {
		t.Fatal("expected DD_todc failure")
	}
}

func TestCloseIsIdempotentAndMethodsReturnErrClosed(t *testing.T) {
	d := &DD{}

	if err := d.Close(); err != nil {
		t.Fatalf("first Close returned %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("second Close returned %v", err)
	}
	if err := d.MoveTo(1, 2); !errors.Is(err, ErrClosed) {
		t.Fatalf("MoveTo returned %v, want ErrClosed", err)
	}
	if err := d.SendKeyEvent(VK_A, KeyDown); !errors.Is(err, ErrClosed) {
		t.Fatalf("SendKeyEvent returned %v, want ErrClosed", err)
	}
}

func TestNewDDRejectsEmptyPath(t *testing.T) {
	if _, err := newDD(""); err == nil {
		t.Fatal("expected an error for an empty DLL path")
	}
}

func TestGetDDFailedInitializationIsNotCached(t *testing.T) {
	defaultMu.Lock()
	old := defaultDD
	defaultDD = nil
	defaultMu.Unlock()
	defer func() {
		defaultMu.Lock()
		defaultDD = old
		defaultMu.Unlock()
	}()

	if _, err := GetDD(""); err == nil {
		t.Fatal("expected an error for an empty DLL path")
	}

	defaultMu.Lock()
	cached := defaultDD
	defaultMu.Unlock()
	if cached != nil {
		t.Fatal("failed initialization must not be cached")
	}
}

func TestGetDDAfterCloseReturnsErrClosed(t *testing.T) {
	defaultMu.Lock()
	old := defaultDD
	d := &DD{closed: true}
	defaultDD = d
	defaultMu.Unlock()
	defer func() {
		defaultMu.Lock()
		defaultDD = old
		defaultMu.Unlock()
	}()

	got, err := GetDD("")
	if got != nil {
		t.Fatalf("GetDD returned closed instance %p", got)
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("GetDD after Close returned %v, want ErrClosed", err)
	}
}

type testWrappedError struct {
	message string
}

func (e *testWrappedError) Error() string {
	return e.message
}

func TestCombineErrorsPreservesErrorIdentity(t *testing.T) {
	sentinel := errors.New("sentinel")
	errno := syscall.Errno(5)
	wrapped := fmt.Errorf("wrapped: %w", &testWrappedError{message: "custom"})

	err := combineErrors(errno, []error{sentinel, wrapped})
	for _, target := range []error{errno, sentinel} {
		if !errors.Is(err, target) {
			t.Fatalf("errors.Is(%v, %v) = false", err, target)
		}
	}

	var custom *testWrappedError
	if !errors.As(err, &custom) {
		t.Fatalf("errors.As(%v, *testWrappedError) = false", err)
	}
	if custom.message != "custom" {
		t.Fatalf("got wrapped error %q", custom.message)
	}
	if message := err.Error(); !strings.Contains(message, "sentinel") || !strings.Contains(message, "custom") {
		t.Fatalf("combined message %q is missing an underlying error", message)
	}
}

func TestCloseKeepsSharedInstanceClosed(t *testing.T) {
	defaultMu.Lock()
	old := defaultDD
	d := &DD{}
	defaultDD = d
	defaultMu.Unlock()
	defer func() {
		defaultMu.Lock()
		defaultDD = old
		defaultMu.Unlock()
	}()

	if err := d.Close(); err != nil {
		t.Fatalf("Close returned %v", err)
	}

	defaultMu.Lock()
	shared := defaultDD
	defaultMu.Unlock()
	if shared != d {
		t.Fatal("Close must retain the shared instance in a closed state")
	}

	if _, err := GetDD(""); !errors.Is(err, ErrClosed) {
		t.Fatalf("GetDD after Close returned %v, want ErrClosed", err)
	}
}
