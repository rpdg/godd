# GoDD
This is a golang wrapper of the [DD](https://github.com/ddxoft/master) virtual mouse & keyboard driver.

Builds and runs on Windows 10 & 11. Actual input injection depends on the
installed DD driver and DLL pairing; see [Driver and DLL notes](#driver-and-dll-notes).

The wrapper uses Windows DLL loading directly and does **not** require cgo or a C compiler.

## How to Use

1. Download the current official DD package.
2. Install the driver by following the instructions in that package.
3. Put the DLL from the package into your project directory.

The DLL file name changes between official releases. Pass the actual path from
the downloaded package to `godd.GetDD`; the example below uses `./dd.dll`, but
`./dd.71000.dll` or any other official file name works just as well.

```go
package main

import (
    "time"

    "github.com/rpdg/godd"
)

// Use the actual DLL path from the official package. If the file is named
// dd.71000.dll, either rename it to dd.dll or change this constant.
const dllPath = "./dd.dll"

func main() {
    dd, err := godd.GetDD(dllPath)
    if err != nil {
        panic("Device initialization error, " + err.Error())
    }
    // No Close is needed: the DLL stays loaded until the process exits.

    // Mouse control functions
    dd.MoveTo(0, 0)
    dd.MoveRelative(50, 150)
    dd.SetMouseButtonState(godd.BtnLeftDown)
    time.Sleep(50 * time.Millisecond)
    dd.SetMouseButtonState(godd.BtnLeftUp)
    dd.ScrollWheel(godd.WheelBackward) // DD_whl(2): down/backward

    // Keyboard control functions
    dd.SendKeyEvent(godd.VK_F1, godd.KeyDown)
    time.Sleep(50 * time.Millisecond)
    dd.SendKeyEvent(godd.VK_F1, godd.KeyUp)
    dd.InputString("some text")

    // Shortcut helpers
    dd.Click() // Performs a single left mouse click
    dd.PressKey(godd.VK_SPACE, 100*time.Millisecond)
    dd.PressHotkey(godd.VK_LWIN, godd.VK_M) // WIN + M

    // Convert a standard Windows virtual-key code to a DD key code.
    if code, err := dd.ToDDCode(0x09); err == nil { // Windows VK_TAB
        dd.SendKeyEvent(code, godd.KeyDown)
        dd.SendKeyEvent(code, godd.KeyUp)
    }
}
```

## Notes

- `godd.GetDD` is the only constructor. It returns one process-wide instance:
  the first successful call loads the DLL, later calls ignore the path, and
  failed initialization is not cached. The DD driver controls global cursor and
  keyboard state, so a singleton avoids false independence and makes the lock
  cover the whole process. `Close` is deprecated and terminal: after it, methods
  and `GetDD` return `ErrClosed`; restart the process to use DD again. Normal
  applications can leave the DLL loaded until process exit and skip `Close`
  entirely.
- `VirtualKey` constants are DD key codes, not Windows `VK_*` values. Use
  `DD.ToDDCode` when you already have a Windows virtual-key code. On the tested
  DLLs, `DD_todc` converts generic modifiers (`VK_CONTROL`, `VK_SHIFT`) but
  returns an error for left/right-specific VKs (`VK_LCONTROL`, `VK_RCONTROL`,
  `VK_LSHIFT`, `VK_RSHIFT`); use the `VirtualKey` constants for those.
- `InputString` uses `DD_str`, which only supports printable ASCII
  (`0x20-0x7E`). The tested vendor DLL silently drops non-ASCII and control
  bytes but still returns success, so the wrapper rejects them instead of
  silently losing text. Use clipboard+`Ctrl+V` or `SendInput` with
  `KEYEVENTF_UNICODE` for Unicode; see
  [docs/dd-str-byte-behavior.md](docs/dd-str-byte-behavior.md).
- `ScrollWheel` follows the official `DD_whl` mapping: `1` is forward/up and
  `2` is backward/down. (`WheelForward`/`WheelUp` and
  `WheelBackward`/`WheelDown` are aliases.)
- Input methods are safe for concurrent use. Because `GetDD` returns a single
  process-wide instance, all calls share one lock: complete sequences such as
  `Click`, `PressKey`, `PressHotkey`, and `InputString` are serialized as a unit
  across the process, and `PressHotkey` releases already-pressed keys if a later
  key fails.
- `Click`, `PressKey`, and `PressHotkey` retry a failed button or key release
  once and report success if the retry succeeds. Errors from combined
  operations preserve their underlying cause for `errors.Is`/`errors.As`.
- The wrapper does not install or uninstall the DD driver; use the official
  package's installer for that.

## Diagnostics and real-device testing

The repository includes small diagnostic commands that are not part of the
public API. They take the vendor DLL path explicitly, so they work with whatever
file name the official package uses:

- `cmd/ddprobe` — wrapper-level mouse, keyboard, `ToDDCode`, and lifecycle probe.
- `cmd/dde2e` — GUI integration test that verifies focus and edit-control text.
- `cmd/ddutf8` — raw `DD_str` byte probe for UTF-8, GBK, control bytes, and NUL.
- `cmd/hidprobe` — HID collection enumeration, access/share checks, and raw
  report writes.

```powershell
go run ./cmd/ddprobe -dll .\ddhid.63340.dll -wait 5s
go run ./cmd/dde2e -dll .\ddhid.63340.dll
go run ./cmd/ddutf8 -dll .\ddhid.63340.dll
go run ./cmd/hidprobe
```

Use `dde2e -skip-mouse` for a keyboard-only run.

See [docs/real-device-testing.md](docs/real-device-testing.md) for the test
matrix, methodology, and observed results,
[docs/dd-str-byte-behavior.md](docs/dd-str-byte-behavior.md) for the `DD_str`
byte-handling experiment, and
[docs/hid-protocol-notes.md](docs/hid-protocol-notes.md) for the vendor HID
collection and report observations. `ddprobe` aborts when the cursor is moving;
run it while the mouse is idle, and do not run multiple device probes at once.

## Driver and DLL notes

The official DD package ships more than one driver/DLL variant, and the files
must be paired. The tested 63xxx package keeps them in separate folders:

- `1.simple\dd63330.dll` — simple-mode DLL with its own driver;
- `2.hid\ddhid.63340.dll` — HID-mode DLL, paired with
  `2.hid\drv\ddhid63340.sys` (plus the `.inf`, `.cat`, and installer).

The two folders are alternatives, not two halves of one installation. Use one
mode and install only the driver from that mode. Do not mix a DLL with a driver
from another DD release.

Newer packages may put the variants at the archive root instead of
`1.simple`/`2.hid`: for example `dd.71000.dll` is the simple-mode DLL and
`hid.72000.dll` is the HID-mode DLL paired with `hid.drv\ddhid72000.sys`. The
HID DLL uses the Windows `HidD_*` APIs. File names change between releases, so
choose the DLL by the driver you have installed, not by the file name alone.

Vendor-side constraints observed while testing the 2026 package on Windows 11:

- HID mode: `DD_key`, `DD_str`, `DD_todc`, the key helpers, mouse movement, and
  mouse buttons work on the tested build. `DD_whl` returns success, but the
  scroll effect was not measured.
- An earlier run appeared to show mouse reports being ignored. That run was not
  a clean environment: a stale `ddhid40616` device/service and an orphan
  `ROOT\HIDCLASS\0001` node were present, and the BOCOM certificate tool was
  holding the HID collections. After cleanup, the same DLL/driver pair passed
  the mouse checks.
- Some certificate/USBKey tools (for example BOCOM
  `certd_I3000_BOCOM.exe`) keep the DD HID vendor collections open for
  writing. Initialization then fails with
  `DD driver initialization failed (code 0)`. Exit the tool and retry.
- The tested HID DLL supports only one successful initialization per process.
  The wrapper therefore makes `Close` terminal: `GetDD` and all methods return
  `ErrClosed` afterward instead of attempting to reload the DLL. Create one
  instance and keep it for the process lifetime.
- Simple mode on the same machine initialized and converted key codes, but
  input calls returned `ERROR_GEN_FAILURE` (31).

These are driver/vendor behaviors. Validate your specific DLL and driver
pairing before relying on injection in production.

## License
The GoDD package is open-sourced software licensed under the Apache V2 license.

## Feedback & Contribute
Notify me of any issues, bugs, or improvements. Thanks 👍
