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
| Unit sample | Wrapper return 1, sample stays in the safe rectangle, the 20 ms and 80 ms positions differ by at most 1 px per axis, cross-axis movement ≤ 1 px. `settled` is this comparison |
| Immediate | The 0 ms and 80 ms positions differ by at most 1 px per axis. A 0 ms sample still at the start does not by itself make `settled` false |
| Step cap | 40 px. This forces multiple steps on a long target. It is not a winput motion |
| Samples | 0, 20, and 80 ms after a single call. Bursts also sample about 50 ms and 150 ms after the last call |
| Bursts | `(+20,0)` five times at gaps 0, 8 ms, and 30 ms, then `(-15,+15)` five times at 8 ms |
| Ladder | Axis counts 1, 10, 50, 200 in both signs. Diagonals only at ±50 |
| Loops | Short `(30,0)`, `(0,-25)`, `(-20,15)`. Long `(240,0)`, `(0,180)`, `(-160,120)` |
| Edge margin | 80 px. Trials that do not fit are skipped. A cursor on the clamp edge aborts the run |
| Run abort | `api-error`, `edge`, `left-monitor`, `diverged`, `anomaly-delta`, `contaminated`, `no-displacement`, `interrupted`, `edge-risk`, `control-failed`, `step-cap` |
| Not a run abort | `converged`, `max-steps`, `timeout`, `skipped-clearance` |
| Exit priority | `converged`, then `timeout`, then `max-steps` |
| Retry | None. A trial runs at most once. An anomaly stops the process |
| Interrupt | Ctrl+C is observed between DLL calls. A call already inside the DLL cannot be cancelled. Killing the process skips cursor restore |

Exit 0 means the plan finished. It does not mean every loop converged. Exit 1
is an abort. Exit 2 is a refusal before motion (usage, a DLL the wrapper
cannot resolve, DPI, or the driver). Exit 130 is Ctrl+C. `step-cap` aborts
only a `-part small` call that would exceed 40 px, and it does so before
`DD_movR`. The 1 px tolerance, 12-step limit, 3 s trial limit, and 40 px
closed-loop cap are unchanged. A matrix single shot still passes its listed
request, including 200.

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

## Round one correction

The raw log [evidence/ddmovr-run.log](evidence/ddmovr-run.log) stays as
recorded. Its git line is `d3d057abc6e4c02ead1eb9c946758fced50ace4d`. The log
commit is `aa1c66aaf92084834bc690804d5822fd46b15d6d`. This section does not
rewrite that file.

The operating-system exit code was not obtained. `Start-Process -NoNewWindow`
left `ExitCode` empty. The probe's own return for `[done] exit=diverged` is 1,
from the `exitAbort` path in `cmd/ddmovr`. Those are separate facts.

`SignConflict` treated any pure-axis observation below -1 px as a conflict.
The four ±10 samples at 80 ms were `10,0`, `-10,0`, `0,10`, and `0,-10`, each
with return 1. The negative samples met the old predicate, and the run stopped
at `stage=axis10` before gain, the later shots, the bursts, and the loops.
`classify` calls `SameDirection`, which already uses the request through the
dot product and stops one call only past `AnomalySlackPx`. That call was not
changed. The ladder call in `early` remains the only `SignConflict` caller.
After this correction it compares each requested axis with that request's
sign. Movement on a zero-request axis is not a sign conflict. One pixel
against the request stays inside the boundary; two pixels is a conflict.

`settled` compared 0 ms with 80 ms. On the ±10 samples the cursor was still
at the start at 0 ms and had completed the request by 20 ms, so `settled` was
false while the 20 ms and 80 ms positions agreed. `SettleAgreePx` remains 1.
`settled` is now the 20 ms and 80 ms comparison. `immediate` is the 0 ms and
80 ms comparison and is logged separately. A late 0 ms sample does not by
itself make `settled` false. The unit-sample ratio still requires `settled`,
a safe sample, return 1, and cross-axis movement of at most 1 px.

## Next matrix round

This commit does not run the matrix. The previous desktop-idle confirmation
applied only to the round already logged. The next round waits for a new
explicit confirmation.

The run uses the probe in this commit. The DLL remains `ddhid.63340.dll`,
SHA-256 `01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e`.
The driver check remains SHA-256
`fbe510402b3822c63e94752051b7d5895b67875f22ec48593de19764a649f8b1`. A compile
failure or a hash mismatch before the process starts stops the round. The
process still calls `GetDD` once. The accepted side effect remains the HID
input-buffer count. No driver install and no pointer, DPI, or service change.

The matrix, admission, 1 px tolerance, 12 steps, 3 s trial limit, and 16 ms
loop interval stay as already fixed. The 40 px cap still applies only inside
`StepRequest`. Single shots and bursts use the listed request sizes. The new
log path is `docs/evidence/ddmovr-run-2.log`, created with `O_EXCL`, so
`docs/evidence/ddmovr-run.log` is not overwritten. One round, no retry. The
outer limit is 90 s and kills only that process.

The process is started without a shared console. After it leaves, the
recorded operating-system exit code is `Process.ExitCode`. If that value is
absent, the record says it was not obtained. The probe token in `[done]` is
written separately and is not copied into the operating-system field.

```powershell
go build -o $env:TEMP\ddmovr-matrix.exe ./cmd/ddmovr
& $env:TEMP\ddmovr-matrix.exe -dll <absolute path of the pinned DLL> -log docs/evidence/ddmovr-run-2.log -run -confirm-idle -wait 5s
```

A normal return tries to restore the cursor captured at the start of motion
and records the result. Ctrl+C is observed between DLL calls. A call already
inside the DLL cannot be cancelled. A 90 s kill skips that restore. Exit 0
means the plan finished. It does not mean every loop converged, and it does
not accept the winput trajectory. Items that do not run stay untested.

## Round two record

The raw log [evidence/ddmovr-run-2.log](evidence/ddmovr-run-2.log) stays as
recorded. Its git line is `24497e2247062bffe15e7cd83ff6195f952381b8`. The log
commit is `b43b9f956eaa64199a4f9291863296e0df359087`. This section does not
rewrite that file and does not run the matrix again.

The operating-system exit code was not obtained. `Process.ExitCode` was empty
after `WaitForExit` returned and `HasExited` was true. The record is
未取得. The probe token is `[done] exit=diverged`. The source return for
that token is 1, from the `exitAbort` path. Those three facts stay separate.
The missing operating-system code is not a reason to repeat the round.

`axis+x200` requested `200,0`. At 20 ms and 80 ms the displacement was
`-56,0`, the wrapper return was 1, and `settled` was true. The cursor was
inside DISPLAY2, not on the clamp edge. `SameDirection` then stopped the
process because the opposite movement was 56 px, past the 15 px slack. Gain
stayed `1.0000` from the four ±10 samples. The summary is
`plan_complete=false`, `loops=0`, `converged=0`, `skipped=0`. The cursor
restore in that log returned to the position saved at the start.

Requests of 1, 10, and 50 on both axes, and the four ±50 diagonals, matched
the request at 20 ms and 80 ms with return 1. `axis-x200`, `axis+y200`,
`axis-y200`, the four bursts, and the six loops did not run in this log.
Leaving them out is not a pass. The bursts and six loops were measured
later in the small-step result. The three remaining 200-count shots stay
untested, and the `axis+x200` failure stays here.

## Parameter width

This check is read-only. `godd.go` is unchanged. Nothing here asserts that
the kernel driver truncates a wider value.

`MoveRelative` calls `DD_movR` as `uintptr(uint32(x)), uintptr(uint32(y))`.
For a value inside `int32`, that conversion keeps the low 32 bits and
zero-extends them to `uintptr`. `200` stays `200` (`0xC8`). A negative `int`
keeps its two's-complement low 32 bits. The Go call does not narrow either
argument to 8 bits.

The in-repository notes do not declare a `DD_movR` width. The absolute-move
experiment in [hid-protocol-notes.md](hid-protocol-notes.md) used a guessed
16-bit layout and did not move the cursor. Its command `0x04` note is an
unverified button layout, not a relative-move specification. The archive
readme already recorded above describes `DD_movR` as a relative move and
does not give a width. No header in this repository states one.

Static disassembly of `ddhid.63340.dll`, SHA-256
`01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e`, was done
without `LoadLibrary`. The image base is `0x180000000`. `DD_movR` is export
index 3, RVA `0x2480`. On the x64 calling convention, `ecx` is x and `edx`
is y. The function copies `ecx` to `eax`, then stores only `al` and `dl`.
The buffer passed onward begins `40 09 04 00`, then the low byte of x, the
low byte of y, and `00`. The callee at RVA `0x1920` walks bytes already in
that buffer and calls `WriteFile` with length 65. The `WriteFile` slot is
the KERNEL32 import at IAT RVA `0x16b8c0`. That callee does not receive the
original 32-bit registers, so it does not put the high 24 bits back. Both
the null-object path and the path after the call set the return to 1, so a
wrapper return of 1 does not show that the full 32-bit request was sent.

The low byte of `200` is `0xC8`. Read as a signed 8-bit value, `0xC8` is
`-56`, which matches the `axis+x200` displacement. That signed reading is
an inference from the byte store plus the cursor sample. The HID report was
not captured on the wire. No vendor document and no driver source states
the field width. Because this DLL stores and sends only the low byte, the
observation does not show the kernel driver truncating a wider value. The
driver-side width stays unknown.

## Large-increment boundary

This plan is independent of the small-step plan below. It keeps the
`axis+x200` failure in `docs/evidence/ddmovr-run-2.log`. It does not rerun
the matrix, the ±200 shots, or any trial that already has a sample. The
other three 200-count shots stay untested. Skipping them is not a full
matrix pass, and this document does not record one. The converged rule,
1 px tolerance, 12-step limit, 3 s trial limit, and 40 px cap are
unchanged. `step-cap` is only the small-part guard. No public API change
follows from the unknown driver width.

## Small-step closed loop

This commit does not run the small part. The desktop-idle confirmation that
authorizes the one run below is the request to commit this plan and then
start. It does not authorize another matrix round.

`-part small` runs the four bursts and then the six loops. It does not run
the early ladder, the 50-count axis shots, the diagonals, or any 200-count
shot. An empty `-part` remains the full matrix, so an older command does
not drop trials. Before `GetDD`, the process refuses with exit 2 if a burst
request, or a closed-loop `StepRequest`, would exceed 40 px.
`move` applies the same cap again and returns `step-cap` without calling
the DLL. `step-cap` aborts the run. The 16 ms loop interval and the burst
gaps of 0, 8, and 30 ms are unchanged.

This part does not repeat the ±10 ladder. Gain therefore starts at 0, and
`Admit` still floors a gain below 1 up to 1. That floor matches the
round-two ratio `1.0000`; it is not a new measurement. A later burst can
still raise gain. A loop that no longer fits is `skipped-clearance`, which
is not a pass and not a run abort.

The new log is `docs/evidence/ddmovr-small.log`, created with `O_EXCL`.
`docs/evidence/ddmovr-run.log` and `docs/evidence/ddmovr-run-2.log` are not
overwritten. The DLL and the running driver are hashed again immediately
before the process. The expected DLL SHA-256 is
`01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e`. The
expected driver and DriverStore SHA-256 is
`fbe510402b3822c63e94752051b7d5895b67875f22ec48593de19764a649f8b1`. A
mismatch or a compile failure stops the run. One process calls `GetDD`
once. The outer limit is 90 s and kills only that process. There is no
retry.

```powershell
go build -o $env:TEMP\ddmovr-small.exe ./cmd/ddmovr
& $env:TEMP\ddmovr-small.exe -part small -dll <absolute path of the pinned DLL> -log docs/evidence/ddmovr-small.log -run -confirm-idle -wait 5s
```

The operating-system exit code is `System.Diagnostics.Process.ExitCode`
with `UseShellExecute` false. If that value is absent, the record says
未取得. The `[done]` token and the source return are listed separately.
An empty stop returns 0, `interrupted` returns 130, and any other stop
returns 1. Exit 0 of this part does not pass the axis 200 trials or the
full matrix, and it does not accept the winput trajectory. The summary
field `full_matrix_pass` is false on both parts so it cannot be read as
that pass. A normal return tries to restore the cursor saved at the start
of motion. A 90 s kill skips that restore.

## Small-step result

The plan section above is the text committed before the run. The raw log
[evidence/ddmovr-small.log](evidence/ddmovr-small.log) stays as recorded.
Its git line is `8f5b5975ed2cb4f915aa579901b19e6069a1afba`, the probe
commit that ran. The log commit is
`b8e1dfdff8c8bbd5ecab27b0d4fbf7585ec373a7`. This section does not rewrite
that file and does not rerun the part.

The DLL is `ddhid.63340.dll`, size 2242088, SHA-256
`01e8db6893cf79e9e7aa3afbee76bea6c4220c4d1a2c63bc2e5b7c109fdb831e`.
The running driver `ddhid63340.sys` and its DriverStore copy are size
1190080, SHA-256
`fbe510402b3822c63e94752051b7d5895b67875f22ec48593de19764a649f8b1`,
with `store_agree=equal`. `godd.go` in the run commit is SHA-256
`2874c7d3d71036bc62638e8373635e278c5fea1fcc1cf2fc3e9ef1429aec4e59`.
The two integration limits below belong to this DLL hash and this driver
hash on this run. They are not a contract for every DLL version.

The operating-system exit code is 0, from
`System.Diagnostics.Process.ExitCode`, with `HasExited` true and the 90 s
limit not reached. The probe token is `[done] exit` empty. The source
return for an empty stop is 0. Those are separate records.

`part=small`, `plan_complete=true`, `full_matrix_pass=false`, `abort`
empty, `shots=38`, `loops=6`, `converged=6`, `skipped=0`, `gain=1.0000`.
Init ran once. Close had an empty error. Restore returned the cursor to
`(-1302,510)`. A finished small part is not a full matrix pass and does
not accept the winput trajectory.

`axis+x200` remains the recorded failure in
[evidence/ddmovr-run-2.log](evidence/ddmovr-run-2.log): request `200,0`
moved `-56,0` at 20 ms and 80 ms, the wrapper returned 1, and the run
then stopped with `diverged`. `axis-x200`, `axis+y200`, and `axis-y200`
did not run. `rem_before=200,0` on `loop-long+x` step 2 is the remaining
error. That call requested `40,0`.

The four bursts each returned 1. The three `(+20,0)` bursts, five calls
at gaps 0, 8 ms, and 30 ms, were at `+100,0` by 50 ms and still there at
150 ms. The `(-15,+15)` burst was at `-75,+75` on both late samples.
No burst displacement exceeded the sum of its requests.

The six loops converged with error `0,0` and wrapper return 1 on every
step. Short `(30,0)`, `(0,-25)`, and `(-20,15)` took one step. Long
`(240,0)` took six steps of 40 px in 116 ms. `(0,180)` took four steps of
40 px and one step of 20 px in 114 ms. `(-160,120)` took four steps in
76 ms. Every closed-loop request was at most 40 px on each axis.

### Integration limits for this hash

These two limits are what a later caller can take from this run. They do
not extend to other DLL versions, other driver hashes, or a request this
run did not make.

Each axis of a closed-loop `DD_movR` in this run was at most 40 px. The
bursts stayed inside that same per-axis cap, at 20 px and at 15 px. The
small-step pass does not cover a larger single request. The `+200` to
`-56` failure on this same DLL remains in force.

The cursor position read immediately after `DD_movR` returned was still
the pre-call position on all 38 calls, while the wrapper return was 1.
The requested quantum appeared on a later sample: the next burst read, the
loop sample after the 16 ms interval, or the burst sample at 50 ms. The
50 ms and 150 ms burst positions agreed. A caller must not treat that
immediate coordinate as the result and send the same increment again.
That would stack a second request on a move the first sample had not
shown yet. The lag is an observation of this DLL on this run, not a
timing contract for every build.
