# DD_str byte handling (UTF-8 experiment)

This note records what the vendor `DD_str` export does with non-ASCII bytes.
`godd.InputString` rejects them before the call, so `cmd/ddutf8` bypasses the
guard and calls `DD_str` directly.

## Environment

- Windows 11, active console session, elevated process.
- HID DLL: `ddhid.63340.dll`.
- Driver: `ddhid63340.sys` (service `ddhid63340`, running).
- System ANSI code page: `65001` (UTF-8).
- Test window: top-level window with a focused multiline `EDIT` control.
- `certd_I3000_BOCOM.exe` was stopped for the run; it holds the DD HID vendor
  collections with write access and blocks initialization.

## Method

For each case the probe:

1. clears the edit control with `SetWindowTextW`;
2. calls `DD_str` with a NUL-terminated heap buffer containing the raw bytes;
3. waits 400 ms and reads the edit text back with `GetWindowTextW`;
4. records the `DD_str` return value and the UTF-16 code units of the result.

No keyboard layout or IME changes were made. The process ANSI code page was the
system default.

## Scope

Only the HID DLL could be evaluated. The simple-mode `dd63330.dll` from the same
package initialized and converted key codes, but injection returned
`ERROR_GEN_FAILURE (31)` on this machine; only the HID driver was installed. Its
`DD_str` byte handling is therefore not covered by these results.

## Results

| Input | Bytes | `DD_str` | Edit text |
| --- | --- | --- | --- |
| `abc` | `61 62 63` | `1` | `abc` |
| UTF-8 `中文` | `E4 B8 AD E6 96 87` | `1` | empty |
| UTF-8 `é` | `C3 A9` | `1` | empty |
| GBK `中文` | `D6 D0 CE C4` | `1` | empty |
| `a` + UTF-8 `中` + `b` | `61 E4 B8 AD 62` | `1` | `ab` |
| UTF-8 `中` + `a` | `E4 B8 AD 61` | `1` | `a` |
| `a` + `0x80` + `b` | `61 80 62` | `1` | `ab` |
| `a` + LF + `b` | `61 0A 62` | `1` | `ab` |
| `a` + TAB + `b` | `61 09 62` | `1` | `ab` |
| UTF-16LE bytes for `中文` | `2D 4E 87 65` | `1` | `-Ne` |
| CP1252 `é` | `E9` | `1` | empty |
| UTF-8 emoji | `F0 9F 98 80` | `1` | empty |
| single space | `20` | `1` | space |
| all printable ASCII | `20`-`7E` | `1` | all characters, unchanged |
| embedded NUL | `61 62 00 63 64` | `1` | `ab` |
| CR LF | `0D 0A` | `1` | empty |

## Interpretation

- `DD_str` is a single-byte API. It types bytes that map to keyboard keys and
  silently skips bytes that do not.
- It does **not** decode UTF-8. `中文` reaches the DLL as six bytes
  (`E4 B8 AD E6 96 87`), and every one of them is dropped.
- It does **not** decode GBK, UTF-16LE, or CP1252 either. The UTF-16LE byte
  sequence happened to contain three printable ASCII bytes (`2D`, `4E`, `65`),
  which were typed as `-Ne`; `87` was dropped.
- Skipping is per byte and non-fatal. `a` + UTF-8 `中` + `b` becomes `ab`, and
  the call still returns success.
- Control bytes such as LF, TAB, and CR are dropped. Embedded NUL truncates the
  C string, so only the prefix before it is typed.
- `DD_str` returns `1` even when every supplied byte is dropped. The return
  value therefore says only that the call was accepted, not that text was
  injected.

## Implication for GoDD

`InputString` must keep rejecting bytes outside printable ASCII. Removing the
guard would turn `InputString("中文")` into a silent no-op that reports success,
and `InputString("a中b")` into `ab`. An explicit error is safer than silent data
loss.

For Unicode text, use a Unicode-capable path instead of `DD_str`:

- clipboard + `Ctrl+V` through `PressHotkey`, or
- `SendInput` with `KEYEVENTF_UNICODE`.

## Reproduce

```powershell
# Stop certificate/USBKey tools that hold the DD HID collections first.
go run ./cmd/ddutf8 -dll .\ddhid.63340.dll
```

The probe requires the DLL and driver to match. With a simple-mode DLL on a
machine without the corresponding simple-mode driver, `DD_str` may return
success while injecting nothing at all; the ASCII control case then fails and
the probe exits with status 1.
