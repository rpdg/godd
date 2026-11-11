# GoDD
This is a golang wrapper of the [DD](https://github.com/ddxoft/master) virtual mouse & keyboard driver.

Tested and working on Windows (10 & 11).

The wrapper uses Windows DLL loading directly and does **not** require cgo or a C compiler.

## How to Use

1. Download the current official DD package.
2. Install the driver by following the instructions in that package.
3. Put the DLL from the package into your project directory.

The DLL file name changes between official releases. Pass the actual path from
the downloaded package to `godd.New`; the example below uses `./dd.dll`, but
`./dd.71000.dll` or any other official file name works just as well.

```go
package main

import (
    "fmt"
    "time"

    "github.com/rpdg/godd"
)

// Use the actual DLL path from the official package. If the file is named
// dd.71000.dll, either rename it to dd.dll or change this constant.
const dllPath = "./dd.dll"

func main() {
    dd, err := godd.New(dllPath)
    if err != nil {
        panic("Device initialization error, " + err.Error())
    }
    defer func() {
        if err := dd.Close(); err != nil {
            fmt.Println("close DD:", err)
        }
    }()

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

- `godd.New` creates an independent instance and is the recommended API.
  `godd.GetDD` remains available as a process-wide shared instance; failed
  initialization is not cached, and `Close` allows a later call to reinitialize.
- `VirtualKey` constants are DD key codes, not Windows `VK_*` values. Use
  `DD.ToDDCode` when you already have a Windows virtual-key code.
- `InputString` uses `DD_str`, which only supports printable ASCII
  (`0x20-0x7E`). Non-ASCII text needs a Unicode-capable input method.
- `ScrollWheel` follows the official `DD_whl` mapping: `1` is forward/up and
  `2` is backward/down. (`WheelForward`/`WheelUp` and
  `WheelBackward`/`WheelDown` are aliases.)
- Input methods are safe for concurrent use. Complete sequences such as
  `Click`, `PressKey`, `PressHotkey`, and `InputString` are serialized as a
  unit, and `PressHotkey` releases already-pressed keys if a later key fails.
- `Click`, `PressKey`, and `PressHotkey` retry a failed button or key release
  once and report success if the retry succeeds. Errors from combined
  operations preserve their underlying cause for `errors.Is`/`errors.As`.
- The wrapper does not install or uninstall the DD driver; use the official
  package's installer for that.

## License
The GoDD package is open-sourced software licensed under the Apache V2 license.

## Feedback & Contribute
Notify me of any issues, bugs, or improvements. Thanks 👍
