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
is an abort. Exit 2 is a refusal before motion (usage, a DLL the wrapper
cannot resolve, DPI, or the driver). Exit 130 is Ctrl+C.

`-run` also requires the process to be in the active console session, an
amd64 DLL exporting `DD_btn`, `DD_key`, `DD_mov`, `DD_movR`, `DD_whl`, and
`DD_str`, and a readable version resource. Those checks let the wrapper
resolve the DLL. They do not prove that the installed driver accepts it.
Filename and version numbers are recorded as `token_relation` and do not
refuse the run. The DriverStore copy must not be ambiguous or differ from
the running image. After the idle check, SPI speed, thresholds, and enhanced
pointer precision are read again; a difference aborts before the DLL is
loaded. `SetCursorPos` is used only to place the cursor at home between
trials. Loop steps call only `DD_movR`.

The probe does not install, update, or replace a driver, and it does not
change pointer speed, enhanced pointer precision, or DPI.

## Commands

From the repository root. Inventory does not load a DLL and does not call
`SetCursorPos`. It may call `SetProcessDPIAware`, which affects only this
process. The log stores file names, hashes, and version strings. It does not
store an account name or a machine path.

```powershell
go run ./cmd/ddmovr -log docs/evidence/ddmovr-inventory.log
```

The log is created with `O_EXCL`. Repeat the inventory into a new path.

Motion still requires `-run`, `-confirm-idle`, and a DLL the operator has
accepted. The probe records filename and version numbers. A difference does
not refuse the run, and a match does not prove the installed driver accepts
the DLL:

```powershell
go run ./cmd/ddmovr -dll <candidate-dll> -log docs/evidence/ddmovr-run.log -run -confirm-idle -wait 5s
```

`-run` without `-confirm-idle` exits 2 before the wait. The probe no longer
refuses a DLL because its build number differs from the running driver. That
is not permission to load it. The pinned initialization file is
`ddhid.63340.dll`, and its full SHA-256 is recorded below. `cmd/ddinit` is
the one-shot loader. It does not run the mouse matrix. Desktop-idle
confirmation is still required before that load.

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

Build numbers above were read from version resources and file names. A shared
or different number is not a compatibility result. Earlier `63340` and
`63330` measurements are not evidence for these files.

## Candidate registration

This record is separate from the 2026-11-12 `MoveRelative(50,50)` result and
from the simple-mode `ERROR_GEN_FAILURE` result. Parent
`686939ea61038f85fdb72c1228289d049bdf1425` records filename and version
numbers and does not use them as a load gate. Parent
`5b7c7a84490178b43e413159cb17b7110d159b80` is the earlier inventory commit.
Neither commit registers `ddhid.63340.dll`. This commit does.

| Role | File | SHA-256 |
| --- | --- | --- |
| Candidate DLL | `hid.72000.dll` | `fbc90b106e726f5e5b5e465c6bb2484b7da381a6c3ea4b2d32162b6efdb324cf` |
| Running driver | `ddhid63340.sys` | `fbe510402b3822c63e94752051b7d5895b67875f22ec48593de19764a649f8b1` |
| Driver the new package would install | `ddhid72000.sys` | `7f682284952523ace6c2e55462807f4d3e3f30895fc96d297b5e920f38f0cfd5` |

`hid.72000.dll` is the HID DLL in `hid.7xxxx.7z`. It is amd64, 35688 bytes,
and its version strings are `ddhid72000`. Its export names are `DD_btn`,
`DD_key`, `DD_mov`, `DD_movR`, `DD_str`, `DD_todc`, and `DD_whl`. The file
contains `HidD_*` and `SetupDi*` names. It does not contain a `.sys` path, and
it does not mention `63340`. Those exports mean the wrapper could resolve the
DLL. They do not prove that the running `ddhid63340` driver accepts it.

The same archive's `ddhid72000.inf` installs service `ddhid72000`, copies
`ddhid72000.sys`, and uses hardware ID `ddhid72000\HID_DEVICE`. `install.bat`
runs `ddc.exe`. That program's image contains
`UpdateDriverForPlugAndPlayDevicesW` beside `newdev.dll`, and the SetupDi
driver-install names. The running driver is service `ddhid63340`, hardware ID
`ddhid63340\HID_DEVICE`, and a different hash. The DLL file does not say that
this installed driver must be replaced. The package's setup path does install
a different driver. That setup was not run.

`dd.71000.dll` is the other new DLL in the archive. It exports the same names
and is also amd64, but its image contains `dd71000.sys`, `\\.\dd71000`, and
`\Device\dd71000`. That names a driver other than `ddhid63340.sys`. The
archive does not contain `dd71000.sys`. This DLL was not loaded.

No driver, service, or system setting was changed. Neither DLL was loaded.
The mouse matrix was not started.

## Init review

`hid.72000.dll` was disassembled and was not loaded. Its delay-import
directory is empty. The import set is HID enumeration and report I/O
(`HidD_GetHidGuid`, `HidD_GetAttributes`, `HidD_GetPreparsedData`,
`HidP_GetCaps`, `HidD_FreePreparsedData`, `HidD_SetNumInputBuffers`,
`SetupDiGetClassDevsW`, `SetupDiEnumDeviceInterfaces`,
`SetupDiGetDeviceInterfaceDetailW`, `SetupDiDestroyDeviceInfoList`,
`CreateFileW`, `WriteFile`, `CloseHandle`, `CreateEventW`,
`WaitForSingleObject`, `GetSystemMetrics`), plus the C runtime. It does not
import `LoadLibrary`, `GetProcAddress`, a service API, or
`UpdateDriverForPlugAndPlayDevicesW`. The text section has no `syscall`
instruction. `DllMain` initializes the security cookie and the C runtime.
Opening a collection is reached from `DD_btn(0)`.

`DD_btn(0)` allocates a 0x58-byte block, then looks for a collection whose
attributes are vendor `0x9AC5` and product `0x9B4D`. On a usable handle it
calls `HidD_SetNumInputBuffers(handle, 10)`. The HID class driver default for
that queue is 32. The setting is documented as the collection's input-report
queue (`IOCTL_SET_NUM_DEVICE_INPUT_BUFFERS`). The documentation does not say
the value returns to 32 when the handle closes. The same path then writes two
65-byte reports, `40 09 88` and `40 09 89`, and waits up to `0x1444` ms. The
wrapper returns success only when that path returns 100. What those reports
change inside the running driver was not measured.

That call is enough to leave `hid.72000.dll` unloaded. The installer was not
run. One load of that file would not be retried, and a later success would
still not be compatibility acceptance.

## Pinned DLL

The initialization candidate is the local file `ddhid.63340.dll`. It is not
part of the git tree. Its full SHA-256 is
`01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e`.
The file is 2242088 bytes, last modified `2026-04-17T02:38:44+08:00`, and
amd64. `FileVersion`, `ProductVersion`, `OriginalFilename`,
`FileDescription`, `ProductName`, and `InternalName` are `ddhid63340`.
The company string is `www.ddxoft.com`. The export names are `DD_btn`,
`DD_key`, `DD_mov`, `DD_movR`, `DD_str`, `DD_todc`, and `DD_whl`.

The running driver is still `ddhid63340.sys`, SHA-256
`fbe510402b3822c63e94752051b7d5895b67875f22ec48593de19764a649f8b1`,
modified `2026-04-10T12:05:05+08:00`. The shared `ddhid63340` text is a
recorded token. It does not prove that this driver accepts this DLL. The
Windows 11 table does not include this DLL hash, so those results are not a
measurement of this file.

The image contains no `.sys` path and no service-install import. `DD_btn(0)`
opens a HID collection, calls `HidD_SetNumInputBuffers(handle, 10)`, writes
two 65-byte reports beginning `40 09 88` and `40 09 89`, and waits up to
`0x1444` ms. The wrapper treats return 100 as code 1. An internal status of 7
calls `MessageBoxA`. The disassembly that was read does not show
`RegSetValueEx`, `CopyFile`, `ShellExecute`, or `SystemParametersInfo` on
this path. Those names are still present elsewhere in the import table.

The operator accepted the input-buffer side effect for this DLL only: the
count may change and may remain after the handle closes. That acceptance
does not allow installing or replacing a driver, or changing any other system
configuration. The DLL has not been loaded.

`cmd/ddinit` checks the file name and the full SHA-256 before `LoadLibrary`.
Without `-confirm-idle` it records the hash and exits 2. With `-confirm-idle`
it makes one `GetDD` call in its own process, then `Close`. The default
timeout is 20s. A timeout or any other failure stops the process, and the
command does not retry. Exit 0 means this process observed code 1 and a
successful `Close`. It does not accept the DLL as compatible, and it does not
start the mouse matrix.

```powershell
go run ./cmd/ddinit -dll ddhid.63340.dll -log docs/evidence/ddinit.log -confirm-idle
```

## Phased plan

1. **Identity, done.** This commit registers the pinned DLL hash above.
   Parent `686939e` is the load-gate change. Parent `5b7c7a8` is the
   inventory. Neither is this registration.
2. **Driver setup, stopped.** Do not run an installer, INF, or service
   change. Do not replace `ddhid63340.sys`.
3. **Load and `DD_btn(0)`, waiting for desktop idle.** One attempt is
   authorized for `ddhid.63340.dll` and for the input-buffer side effect
   only. `cmd/ddinit` is the process. It has not been run. An explicit
   desktop-idle confirmation is still required. No retry.
4. **Mouse matrix, not authorized.** Only after the initialization log is
   delivered, with another idle confirmation, using the existing `ddmovr -run`
   limits. Not started.

## Inventory

Public log: [evidence/ddmovr-inventory.log](evidence/ddmovr-inventory.log).

The log records `probe_moved_cursor=false` and `loaded=false`. No `DD_movR`
call was made. Pointer speed, enhanced pointer precision, DPI, and the driver
service were read and not written. The log's git line and `[source]` hashes
belong to `5b7c7a8`. Parent `686939e` is the load-gate change. This commit
registers `ddhid.63340.dll`. The inventory log is not that registration.

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

- The class `DriverVersion` `8.8.8.8` is not the product token. The running
  driver's file-name token is `63340`. A shared or different number is recorded
  and is not a compatibility result.
- With both monitors at the system DPI, the placement rule selects the larger
  safe interior. Whether a later ±200 count still reaches the clamp edge
  depends on measured gain. This inventory did not measure gain.
- The Windows 11 table and the `(+50,+50)` sample in `real-device-testing.md`
  come from an earlier write-up. They are a different record from this
  inventory.

### Untested

- Signs, magnitudes, call intervals, units, acceleration, overshoot, and
  multi-step convergence of `DD_movR`. No motion log exists.
- No member of either archive has file-name or version token `63340`. The
  candidate `hid.72000.dll` was still not loaded. See the registration above.
- Buttons, wheel, keyboard, and `DD_str` were not repeated.
- The winput human trajectory was not run. The probe records
  `winput_human_band_abs_lt_3` only as a comparison field. It is not the pass
  rule, and this document does not accept a winput change.
