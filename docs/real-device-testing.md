# Real-device testing

This document records how GoDD was validated against the official DD package on
real Windows hardware, which checks are automated, and which observations are
vendor/driver limitations rather than wrapper bugs.

The goal is repeatable evidence: a `DD_*` call returning `1` is treated as
"the DLL accepted the request", not as "the input was injected". Every input
test therefore verifies a visible side effect (cursor position, button/key
state, window focus, or edit-control text).

## Test environment

The results in this repository were collected on:

| Item | Value |
| --- | --- |
| OS | Windows 11, x64, active console session |
| Privileges | Administrator |
| Go | 1.26.0 windows/amd64 |
| DD package | Official 2026 package |
| HID DLL | `ddhid.63340.dll` |
| HID driver | `ddhid63340.sys` (installed) |
| HID hardware ID | `VID_9AC5&PID_9B4D` |
| Simple-mode DLL | `dd63330.dll`; project copy `dd43390.dll` is byte-identical |
| Simple-mode DLL SHA-256 | `72FBA9C7721AC394A0803D0C97835B0DA21C9D6DDA8076FC0584A566CD7B2A9D` |

The simple-mode and HID-mode files are different products. The tested 63xxx
package keeps them in separate folders: `1.simple\dd63330.dll` and
`2.hid\ddhid.63340.dll` plus `2.hid\drv\ddhid63340.sys`. Always pair a DLL with
the driver from the same official package; do not mix releases.

## Diagnostic tools

All tools are separate `main` packages so they never become part of the public
library API. They accept the DLL path explicitly; no DLL name is hard-coded in
the repository documentation or tests.

| Tool | Purpose |
| --- | --- |
| `cmd/ddprobe` | Minimal wrapper-level probe: cursor control, relative/absolute move, buttons, wheel, key injection, `ToDDCode`, `Close`, and terminal singleton behavior. |
| `cmd/dde2e` | GUI integration test: creates a window and edit control, moves/clicks with DD, then verifies focus and text produced by `DD_str`, `PressHotkey`, `PressKey`, and `ToDDCode`. |
| `cmd/ddutf8` | Calls the vendor `DD_str` export with raw byte sequences and reads the edit text back, documenting ASCII, UTF-8, GBK, control-byte, and NUL handling. |
| `cmd/hidprobe` | Low-level HID inspection without loading the vendor DLL: enumerate collections, query caps, test access/share modes, and write raw vendor reports. |

Typical commands (run from the repository root):

```powershell
# Wrapper-level mouse/keyboard probe
go run ./cmd/ddprobe -dll .\ddhid.63340.dll -wait 5s

# Keyboard mappings and modifier hold/release
go run ./cmd/ddprobe -dll .\ddhid.63340.dll -mode keys

# GetDD lifecycle and terminal Close
go run ./cmd/ddprobe -dll .\ddhid.63340.dll -mode getdd

# Full GUI test, including mouse
go run ./cmd/dde2e -dll .\ddhid.63340.dll

# Keyboard-only run (when mouse reports are not injected)
go run ./cmd/dde2e -dll .\ddhid.63340.dll -skip-mouse

# Raw DD_str byte handling (UTF-8, GBK, UTF-16LE, control bytes, NUL)
go run ./cmd/ddutf8 -dll .\ddhid.63340.dll

# HID collections used by DD
go run ./cmd/hidprobe

# All HID interfaces on the machine
go run ./cmd/hidprobe -all
```

`-dll` can be omitted when `GODD_DLL` is set.

## Methodology

Global cursor and keyboard state is shared with the physical user and with every
other application. A test that ignores this produces false results, so the
probes enforce the following rules:

1. **Become DPI-aware first.** The process calls `SetProcessDPIAware` before any
   cursor API. Without it, coordinates are virtualized and comparisons are
   meaningless on scaled displays.
2. **Wait for an idle mouse.** `ddprobe -wait` sleeps first, then requires the
   cursor to remain at the same position for 500 ms. If it moves, the run aborts
   instead of recording unreliable numbers.
3. **Prove the measurement path.** Before interpreting DD results, the probe
   calls `SetCursorPos(300,300)` and aborts if the cursor does not land there.
   This separates "DD did nothing" from "cursor queries are broken".
4. **Verify side effects, not return codes.** Mouse moves compare before/after
   coordinates; button and key tests read `GetAsyncKeyState`; GUI tests check
   `GetForegroundWindow`, `GetFocus`, and edit-control text.
5. **Reject contaminated evidence.** If a delta is neither zero nor the requested
   value, the probe reports that another input source may be active rather than
   claiming success or failure.
6. **Do not run device probes concurrently.** `hidprobe -matrix` fails with
   sharing violations while another process holds the collections. Run one tool
   at a time, and stop certificate/USBKey tools that keep the collections open.
7. **Restore the user's cursor.** The original position is restored on exit.

Exit status is not the whole story: `ddprobe` prints `[ok]`, `[warn]`, `[fail]`,
`[abort]`, `[skip]`, and `[known-vendor-limitation]` markers. An `[abort]` means
the environment was not stable enough to draw a conclusion.

## Automated checks

### Unit tests

```powershell
go test -count=1 ./...
```

The unit tests do not load a vendor DLL. They cover result checking, input
validation, release retries, error chains, closed-instance behavior, and the
singleton `GetDD` lifecycle.

### Wrapper probe (`ddprobe`)

Checks, in order:

- initialization through `godd.GetDD` (`DD_btn(0)`);
- cursor control via `SetCursorPos`;
- `MoveTo` and `MoveRelative`, measured against actual cursor coordinates;
- left button down/up, measured with `GetAsyncKeyState(VK_LBUTTON)`;
- `ScrollWheel`, whose scroll offset is **not** measured by this probe;
- `SendKeyEvent(VK_A)` down/up, measured with `GetAsyncKeyState`;
- `ToDDCode(VK_TAB)`;
- `Close`, then a method call that must return `ErrClosed`;
- `GetDD` after `Close`, which must also return `ErrClosed`.

### GUI integration (`dde2e`)

The harness creates a top-level window with an edit control and forces it to the
foreground. If the window cannot be focused, input tests are skipped and the run
exits with status 1 (`INCOMPLETE`) instead of reporting success. With focus
established it checks:

- `MoveTo(edit center)` changes the cursor position (tolerance 2 px);
- `Click()` focuses the edit control;
- `DD_str` types `godd-e2e-123`;
- `PressHotkey(Shift+Home)` + `DD_str` replaces the text;
- `ToDDCode(VK_HOME)` + `PressKey` + `DD_str` inserts at the start;
- `ToDDCode(VK_END)` + `PressKey` + `DD_str` appends at the end;
- `Close` and post-close `ErrClosed`;
- `GetDD` after `Close` returns `ErrClosed`; the singleton is terminal and is
  not reinitialized.

`-skip-mouse` is intended for driver builds that accept mouse reports without
injecting them. Keyboard and lifecycle checks still run.

### HID probe (`hidprobe`)

`hidprobe` does not load the vendor DLL at all. It enumerates HID interfaces via
SetupAPI, queries `HidD_GetAttributes`/`HidP_GetCaps`, and opens collections with
the same access mask as DD (`GENERIC_READ|GENERIC_WRITE`, `share=read`). This
distinguishes three failure classes:

- the collection is absent (driver not installed or device not started);
- the collection exists but another process holds a conflicting handle;
- the collection opens, so the problem is in the vendor DLL/report path.

See [hid-protocol-notes.md](hid-protocol-notes.md) for the collection map and
the observed report format.

## Results

### HID mode (`ddhid.63340.dll` + `ddhid63340.sys`)

| Check | Result |
| --- | --- |
| `godd.GetDD` initialization | Pass |
| `DD_todc(VK_TAB)` | Pass, returns `300` |
| `DD_todc` on left/right-specific modifiers | Fails with code -1 (vendor mapping limitation; generic `VK_CONTROL`/`VK_SHIFT` work) |
| `DD_str("godd-e2e-123")` | Pass |
| `PressHotkey(Shift+Home)` selection/replace | Pass |
| `PressKey(Home)` / `PressKey(End)` | Pass |
| `Close` and post-close `ErrClosed` | Pass |
| `GetDD` after `Close` | Returns `ErrClosed` (terminal singleton; the vendor DLL also rejects a second init with code 0) |
| `DD_key`, `SendKeyEvent` | Pass (observed key state changes) |
| `DD_mov` / `DD_movR` | Pass (cursor landed exactly on the requested position) |
| `DD_btn` | Pass (`VK_LBUTTON` down/up observed) |
| `DD_whl` | DLL returns success; scroll offset not measured |

The clean mouse retest on 2026-11-12 passed: `SetCursorPos(300,300)` landed at
`(300,300)`, `MoveTo(600,400)` landed at `(600,400)`, `MoveRelative(50,50)`
produced exactly `(50,50)`, and `DD_btn` changed `VK_LBUTTON` state. The full
`dde2e` run also moved the cursor to the edit control and clicked it into focus.

An earlier run reported "DLL returns success but the cursor did not move". That
result was not reproducible in a clean environment. The earlier run had a stale
`ddhid40616` device/service and an orphan `ROOT\HIDCLASS\0001` node, and
`certd_I3000_BOCOM.exe` was holding the vendor collections. After removing the
stale device and stopping the certificate tool, the same DLL/driver pair
injected mouse input normally. Treat a mouse failure as an environment issue
first: check for stale devices/services and collection holders, then rerun.

### DD_str byte handling

`cmd/ddutf8` calls `DD_str` directly, bypassing `InputString`'s ASCII guard. On
the tested HID DLL, printable ASCII `0x20-0x7E` is typed literally, while UTF-8,
GBK, CP1252, and control bytes are silently skipped. Mixed input such as
`a<E4><B8><AD>b` becomes `ab`, and `DD_str` still returns `1`. Embedded NUL
truncates the C string. See [dd-str-byte-behavior.md](dd-str-byte-behavior.md)
for the full table and implications.

### Simple mode (`dd43390.dll` / official `dd63330.dll`)

| Check | Result |
| --- | --- |
| Initialization | Pass |
| `DD_todc` | Pass |
| `GetDD` → `Close` → `GetDD` | Pass: later `GetDD` returns `ErrClosed` |
| Mouse and keyboard injection | Fails with `ERROR_GEN_FAILURE (31)` |

This is a driver/system pairing issue, not a wrapper decision. The wrapper
reports the error returned by the vendor DLL.

### Sharing conflicts

`certd_I3000_BOCOM.exe` (BOCOM USBKey/certificate tool) holds the DD HID vendor
collections (`\Device\000001b7` and `\Device\000001b8` on the tested machine)
with write access. While it runs, DD initialization fails with code 0. Exit the
tool and retry. Use `handle64.exe` or `hidprobe -matrix` to confirm which process
holds a collection; `hidprobe -matrix` must be run while no other DD client is
active.

## Interpreting failures

| Symptom | Likely cause | Action |
| --- | --- | --- |
| Initialization returns code 0 | Another process holds the HID collections | Stop certificate/USBKey tools; check with `hidprobe` |
| Init succeeds, injection returns `ERROR_GEN_FAILURE (31)` | DLL/driver mismatch or unsupported simple-mode pairing | Use the matching official driver/DLL package |
| DLL returns `1`, cursor/keys unchanged | Stale HID device/service, a process holding the collections, or a DLL/driver mismatch | Remove stale devices, stop collection holders, re-test with idle input |
| Cursor does not land on `SetCursorPos` target | Process is not DPI-aware or session is not interactive | Run in the active console session; do not virtualize input |
| `hidprobe -matrix` shows sharing violations | Another probe or DD client is running | Run probes one at a time |
| `go test -race` fails with `0xc0000139` | Known environment-wide race-runtime load failure on this machine (also affects hello-world) | Use `go test -count=1 ./...`; do not attribute it to GoDD |

## Limitations of this evidence

- Results apply to the exact DLL/driver pair listed above. DD changes file names
  and behavior between releases.
- `ddprobe` measures cursor position and `GetAsyncKeyState`, not application
  semantics. `dde2e` adds one GUI-level check, not coverage of every target app.
- Wheel scrolling is accepted but not measured; a dedicated scrollable target
  would be required to prove it.
- The tested HID DLL cannot be initialized twice in one process. GoDD exposes a
  terminal singleton (`GetDD` after `Close` returns `ErrClosed`) instead of
  attempting to reload it.
- The `DD_str` byte-handling results were collected with system ANSI code page
  65001 and the tested HID DLL. Another code page or keyboard layout may map
  different single bytes, but the DLL still does not decode UTF-8.
- The direct HID report experiments are reverse-engineering notes, not a
  supported API. Do not depend on them in production code.
