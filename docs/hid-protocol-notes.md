# DD HID protocol notes

These notes come from probing the official 2026 DD package on real hardware.
They are **observations, not an official specification**. DD does not publish a
stable HID ABI, file names and report layouts change between releases, and the
vendor may ignore reports that look valid. Do not build production code on this
layout; use the exported `DD_*` functions through GoDD.

The purpose of this document is diagnostic: when injection fails, these notes
help distinguish a missing collection, a sharing conflict, and a driver that
silently discards accepted reports.

## Device identity

`hidprobe -all` on the tested machine reported one DD HID device:

```text
VID=9AC5 PID=9B4D
```

| Collection | Usage page | Usage | Input bytes | Output bytes | Feature bytes |
| --- | --- | --- | --- | --- | --- |
| `col01` | `0x000D` | `0x0004` | 22 | 0 | 2 |
| `col02` | `0x000D` | `0x000E` | 0 | 0 | 3 |
| `col03` | `0x0001` | `0x0002` | 7 | 0 | 0 |
| `col04` | `0x0001` | `0x0002` | 5 | 0 | 0 |
| `col05` | `0x000D` | `0x0002` | 6 | 0 | 0 |
| `col06` | `0x0001` | `0x0004` | 9 | 0 | 0 |
| `col08` | `0xFF00` | `0x0001` | 65 | 65 | 0 |
| `col09` | `0xFF00` | `0x0002` | 33 | 65 | 0 |

`col08` and `col09` are the vendor-defined collections. They are the ones DD
opens for injection. The other collections are keyboard/mouse/system-control
inputs from the device and are not used for output reports.

The interface path changes with the device instance and driver version. Query it
at runtime with `hidprobe`; do not hard-code the path.

## Collection access

DD opens the vendor collections with:

```text
CreateFileW(
    path,
    GENERIC_READ | GENERIC_WRITE,   // 0xC0000000
    FILE_SHARE_READ,                // 0x00000001
    ...
)
```

`hidprobe` uses the same flags in its "exact DD open" section. Consequences:

- A process that holds a collection with write access and does not share write
  blocks DD. On the tested machine this is what
  `certd_I3000_BOCOM.exe` does.
- `hidprobe` itself holds the collections while it runs, so `-matrix` and other
  probes must be run sequentially, never concurrently.
- Enumeration opens with access `0` and `share=read|write` just long enough to
  query attributes and caps; this does not conflict with normal operation.

Sharing violation is Windows error 32 (`ERROR_SHARING_VIOLATION`). DD surfaces
the same condition as initialization code `0`, which is why the error text alone
is not enough to diagnose it.

## Output report framing

The tested vendor collections accept a **65-byte output report**:

```text
offset  value        meaning
0       0x40         report ID
1       0x09         vendor prefix
2       command      operation code
3..64   payload      command-specific, zero-padded
```

The first three bytes were identical for every command tested. The payload is
little-endian where multi-byte values appear.

### Command `0x07`: keyboard

The payload after the command looks like a boot-keyboard report:

```text
offset  meaning
3       modifier mask
4       reserved (0)
5..10   up to six HID keyboard usages
```

Example: press `A` (HID usage `0x04`):

```text
40 09 07 00 00 04 00 00 00 00 00 ...   (65 bytes total)
```

Observed result: `GetAsyncKeyState(VK_A)` reported the key down, and writing the
same report with an all-zero payload released it. This command works on the
tested build.

### Command `0x03`: absolute mouse move

The tested layout was:

```text
offset  meaning
3       button state (written as 0)
4..5    X, little-endian, normalized to 0..0x7FFF
6..7    Y, little-endian, normalized to 0..0x7FFF
```

The normalization used by the probe was:

```text
x_norm = (x + 1) * 0x7FFF / screen_width
y_norm = (y + 1) * 0x7FFF / screen_height
```

where `screen_width`/`screen_height` come from `GetSystemMetrics(SM_CXSCREEN)`
and `SM_CYSCREEN`. This is the common absolute-pointer convention, but it was
not confirmed against the vendor DLL's own calculations.

Observed result of this raw-layout experiment: `WriteFile` accepted all 65 bytes,
and the cursor did not move. The wrapper's `DD_mov`/`DD_movR` **did** move the
cursor after the environment was cleaned up (see
[real-device-testing.md](real-device-testing.md)). This raw layout is therefore
incomplete or not the path the vendor DLL uses; do not conclude that the driver
ignores mouse reports.

### Command `0x04`: mouse buttons

The tested layout was:

```text
offset  meaning
3       button mask: 0x01 = left, 0x00 = release
```

Observed result of this raw-layout experiment: `WriteFile` accepted the report,
but `GetAsyncKeyState(VK_LBUTTON)` did not change. The wrapper's `DD_btn` path
did change button state after cleanup, so the raw layout above is unverified and
must not be used as a specification.

### Wheel

`DD_whl` returned success through the wrapper, but the scroll offset was never
measured, and the raw command byte was not identified. Treat wheel support on
this build as unverified rather than working or broken.

## Reproducing a report

`hidprobe` writes a raw report to the first matching vendor collection. Build
the 65-byte buffer explicitly. For example, to press `A` on PowerShell:

```powershell
$r = [byte[]]::new(65)
$r[0] = 0x40; $r[1] = 0x09; $r[2] = 0x07; $r[5] = 0x04
$hex = ($r | ForEach-Object { $_.ToString('x2') }) -join ' '
go run ./cmd/hidprobe -report $hex
```

The tool warns when the supplied length differs from the collection's
`OutputReportByteLength`. A successful `WriteFile` only means the driver
accepted the buffer; it does not prove injection. Always measure a side effect.

## What the evidence does and does not show

Shown:

- the DD HID device exposes two vendor collections with 65-byte output reports;
- the `0x40 0x09 <command>` framing is accepted on both wrapper and raw paths;
- keyboard reports (`0x07`) produced real key state changes;
- raw `0x03`/`0x04` reports built with the guessed layout were accepted but
  produced no observable changes, while the vendor DLL path did work after the
  environment was cleaned up — the raw layout is incomplete or unverified;
- a second process holding the collections with write access prevents DD from
  initializing.

Not shown:

- that the mouse layout above is complete or correct for other DD releases;
- that the driver requires additional initialization or feature reports before
  it acts on mouse commands;
- that wheel reports work;
- that the behavior is caused by GoDD rather than the vendor DLL/driver pairing.

Before treating a specific build as broken, rerun `ddprobe` with an idle mouse
and `dde2e` without `-skip-mouse`, and record the exact DLL/driver versions.
The methodology and result tables are in
[real-device-testing.md](real-device-testing.md).
