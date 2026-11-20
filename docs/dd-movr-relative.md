# DD_movR relative movement

`cmd/ddmovr` measures `DD_movR` through `godd.MoveRelative`. It is a separate
`main` package. The closed loop is not a godd API, and it is not the winput
trajectory: each step requests at most 40 px, waits 16 ms, and adds no jitter
or curve. A finished run does not accept a later winput change.

The 2026-11-12 `MoveRelative(50,50)` result in
[real-device-testing.md](real-device-testing.md) is one sample. It is not this
probe.

## Pass criteria

The probe prints these limits before it waits and before it moves the cursor.
The inventory log repeats them on `[protocol]` lines.

| Limit | Value |
| --- | --- |
| Converged | Absolute error ≤ 1 px on both axes, within 12 steps and 3 s, every sample inside the safe rectangle, wrapper return 1 |
| Unit sample | Wrapper return 1, sample stays in the safe rectangle, the 0 ms and 80 ms positions differ by at most 1 px per axis, cross-axis movement ≤ 1 px |
| Step cap | 40 px. This forces multiple steps on a long target. It is not a winput motion |
| Samples | 0, 20, and 80 ms after a single call. Bursts also sample about 50 ms and 150 ms after the last call |
| Bursts | `(+20,0)` five times at gaps 0, 8 ms, and 30 ms, then `(-15,+15)` five times at 8 ms |
| Ladder | Axis counts 1, 10, 50, 200 in both signs. Diagonals only at ±50 |
| Loops | Short `(30,0)`, `(0,-25)`, `(-20,15)`. Long `(240,0)`, `(0,180)`, `(-160,120)` |
| Edge margin | 80 px. Trials that do not fit are skipped. A cursor on the clamp edge aborts the run |
| Run abort | `api-error`, `edge`, `left-monitor`, `diverged`, `anomaly-delta`, `contaminated`, `no-displacement`, `interrupted`, `edge-risk`, `control-failed` |
| Not a run abort | `converged`, `max-steps`, `timeout`, `skipped-clearance` |
| Exit priority | `converged`, then `timeout`, then `max-steps` |
| Retry | None. A trial runs at most once. An anomaly stops the process |
| Interrupt | Ctrl+C is observed between DLL calls. A call already inside the DLL cannot be cancelled. Killing the process skips cursor restore |

Exit 0 means the plan finished. It does not mean every loop converged. Exit 1
is an abort. Exit 2 is a refusal before motion (usage, DLL/driver pair, DPI,
or driver). Exit 130 is Ctrl+C.

`-run` also requires the process to be in the active console session, an
amd64 DLL exporting `DD_btn`, `DD_key`, `DD_mov`, `DD_movR`, `DD_whl`, and
`DD_str`, and a readable version resource whose 4-digit tokens equal the
driver file-name token. The DriverStore copy must not be ambiguous or differ
from the running image. After the idle check, SPI speed, thresholds, and
enhanced pointer precision are read again; a difference aborts before the DLL
is loaded. `SetCursorPos` is used only to place the cursor at home between
trials. Loop steps call only `DD_movR`.

A file name that contains the driver token is not enough. `PairOK` also
requires every 4-digit run in `FileVersion`, `ProductVersion`, and
`OriginalFilename` to equal that same token. An empty version field is not a
token. A DLL from another build is not loaded.

## Commands

From the repository root. Inventory does not load a DLL and does not call
`SetCursorPos`. It may call `SetProcessDPIAware`, which affects only this
process. The log stores file names, hashes, and version strings. It does not
store an account name or a machine path.

```powershell
go run ./cmd/ddmovr -log docs/evidence/ddmovr-inventory.log
```

The log is created with `O_EXCL`. Repeat the inventory into a new path.

Motion requires a DLL whose file name and version tokens equal the running
driver, and an operator confirmation that the desktop is idle:

```powershell
go run ./cmd/ddmovr -dll <paired-dll> -log docs/evidence/ddmovr-run.log -run -confirm-idle -wait 5s
```

`-run` without `-confirm-idle` exits 2 before the wait. Do not point `-dll`
at `dd63330.dll` or at a 32695, 32715, 71000, or 72000 package while
`ddhid63340.sys` is the running driver.

## Archive listing

Two archives were listed. Their driver binaries were extracted only to read
size, SHA-256, and the version resource. Nothing was installed, and nothing
was passed to `LoadLibrary` or `DD_btn`.

`hid.7xxxx.7z`, archive SHA-256
`728632decc9339d780c708a92f6185e86e04de5e59538aa1aca9ac68563b1b1f`:

| Member | Size | SHA-256 | FileVersion | OriginalFilename |
| --- | --- | --- | --- | --- |
| `dd.71000.dll` | 3815272 | `ecf6df1111af9f8c0d1f55e5f0dc4924084643f6783765d22ca3faf87bffb298` | `dd71000` | `dd71000` |
| `hid.72000.dll` | 35688 | `fbc90b106e726f5e5b5e465c6bb2484b7da381a6c3ea4b2d32162b6efdb324cf` | `ddhid72000` | `ddhid72000` |
| `hid.drv\ddhid72000.sys` | 1180864 | `7f682284952523ace6c2e55462807f4d3e3f30895fc96d297b5e920f38f0cfd5` | `ddhid72000` | `ddhid72000` |

`ProductVersion` matches `FileVersion` on each of these three.
`hid.72000.dll` and `ddhid72000.sys` are one 72000 pair. `dd.71000.dll` is the
simple-mode DLL in that same archive. None of the tokens is `63340`.

A local RAR named `dd.最新官方EV版.rar` contains an older package. Its driver
binaries are:

| Member | Size | SHA-256 | FileVersion | OriginalFilename |
| --- | --- | --- | --- | --- |
| `dd.32695\dd32695.x64.dll` | 4188120 | `b436112f25a6a859b0512a44d19176e842e28e25dfde0b775eea33b5d2cb2ea5` | `dd32695` | `www.ddxoft.com` |
| `dd.32695\dd32695.x32.dll` | 3629016 | `f62a98380af99787fc4838986722d105ab9b4ad134149e57e9a741ec957f0090` | `dd32695` | `www.ddxoft.com` |
| `dd.hid.32715\hid.64.dll` | 2249688 | `40d302ec78cab59b31d8c3627bac57aaa9df2e2b1d5e5cef67bd257de5323214` | `202x.0.0.8` | `DD HID` |
| `dd.hid.32715\hid.32.dll` | 1650648 | `64723b97b8f843a79b754cdac39067a332d1c3b24bb65cfe5e10bb80e06c2aae` | `202x.0.0.8` | `DD HID` |
| `dd.hid.32715\drv\DDHID32715.sys` | 1599920 | `01976bbd6761c065c898439a4296f099568b42598fb81c5c4180f44f24737bf3` | `1.0.0.1` | `DDHID32715` |

`ProductVersion` matches `FileVersion` on each row. The RAR readme describes
`DD_movR` as a relative move and does not name build `63340`.

The repository copy `dd63330.dll` was not loaded. Its size is 3860008 and its
SHA-256 is `72fba9c7721ac394a0803d0c97835b0da21c9d6dda8076fc0584a566cd7b2a9d`.
`FileVersion`, `ProductVersion`, and `OriginalFilename` are `DD63330`.

### Pairing judgment

The running driver base name is `ddhid63340.sys`. Its version strings are
`ddhid63340`, so the only acceptable DLL token is `63340`.

- `dd.71000.dll`, `hid.72000.dll`, and both `dd32695` DLLs fail because the
  file-name token and the version token are different builds.
- `hid.64.dll` and `hid.32.dll` fail because the file name has no 4-digit
  token. `202x.0.0.8` and `DD HID` also contain no 4-digit token, and neither
  field contains `63340`. The directory name `dd.hid.32715` is package layout,
  not a version resource.
- `DDHID32715.sys` has `OriginalFilename` token `32715`. Its size and SHA-256
  differ from the running `ddhid63340.sys`.
- `dd63330.dll` fails because its token is `63330`.

No member is an original DLL for the running `63340` driver. The motion matrix
was not started.

## Inventory

Public log: [evidence/ddmovr-inventory.log](evidence/ddmovr-inventory.log).

The log records `probe_moved_cursor=false` and `loaded=false`. No `DD_movR`
call was made. Pointer speed, enhanced pointer precision, DPI, and the driver
service were read and not written. The log's git line names the parent commit
and the pre-commit status; this file's own commit is the citation point for
winput. Probe-file hashes are the `[source]` lines.

### Observed

- One `dd*.sys` driver: service `ddhid63340`, running, start type 3, image
  base name `ddhid63340.sys`, size 1190080, modified `2026-04-10T12:05:05+08:00`.
  SHA-256 `fbe510402b3822c63e94752051b7d5895b67875f22ec48593de19764a649f8b1`.
  `FileVersion`, `ProductVersion`, and `OriginalFilename` are `ddhid63340`.
  Hardware ID `ddhid63340\HID_DEVICE`. Class `DriverVersion` `8.8.8.8`.
  DriverStore copy hash is equal (`store_agree=equal`).
- OS, DPI, pointer speed, and monitor rectangles are in the log. Both monitors
  on that boot reported the system DPI. The placement line is a calculation
  from those rectangles, not a measured move.
- Repository siblings present and unused: `dd63330.dll` and `hid.7xxxx.7z`,
  with the hashes above.
- `GODD_DLL` was unset and `-dll` was not passed.

Rectangle fields in the log are `left,top,right,bottom`.

### Inference

- The class `DriverVersion` `8.8.8.8` is not the product token. Pairing uses
  `63340`.
- With both monitors at the system DPI, the placement rule selects the larger
  safe interior. Whether a later ±200 count still reaches the clamp edge
  depends on measured gain. This inventory did not measure gain.
- The Windows 11 table and the `(+50,+50)` sample in `real-device-testing.md`
  come from an earlier write-up. They are a different record from this
  inventory.

### Untested

- Signs, magnitudes, call intervals, units, acceleration, overshoot, and
  multi-step convergence of `DD_movR`. No motion log exists.
- A DLL whose file name and version tokens are `63340` was not found in either
  archive, so none was loaded.
- Buttons, wheel, keyboard, and `DD_str` were not repeated.
- The winput human trajectory was not run. The probe records
  `winput_human_band_abs_lt_3` only as a comparison field. It is not the pass
  rule, and this document does not accept a winput change.
