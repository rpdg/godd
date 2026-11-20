//go:build windows

// Command ddmovr records the installed DD driver and, only when -run and
// -confirm-idle are both set, measures DD_movR.
//
// The closed loop in this command is a fixed-cap instrument. It is not part of
// the godd public API, it is not the winput trajectory, and a finished run does
// not accept a later winput change.
//
// Inventory does not load a DLL and does not call SetCursorPos:
//
//	go run ./cmd/ddmovr -log docs/evidence/ddmovr-inventory.log
//
// Motion requires a DLL whose version token matches the running driver, and an
// operator who has confirmed that the desktop is idle:
//
//	go run ./cmd/ddmovr -dll C:\abs\ddhid.63340.dll -log docs\evidence\ddmovr-run.log -run -confirm-idle -wait 5s
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rpdg/godd"
)

const (
	exitOK        = 0
	exitAbort     = 1
	exitRefuse    = 2
	exitInterrupt = 130
)

// requiredExports are the symbols godd resolves before DD_btn(0). Refusing
// here keeps a partial DLL from being loaded.
var requiredExports = []string{"DD_btn", "DD_key", "DD_mov", "DD_movR", "DD_whl", "DD_str"}

func main() {
	os.Exit(realMain(os.Args[1:]))
}

func realMain(args []string) int {
	fs := flag.NewFlagSet("ddmovr", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dllFlag := fs.String("dll", os.Getenv("GODD_DLL"), "vendor DD DLL path; required with -run")
	driverFlag := fs.String("driver", "", "DD service name; required when more than one dd*.sys is installed")
	runFlag := fs.Bool("run", false, "measure DD_movR; moves the cursor and requires -confirm-idle")
	confirmFlag := fs.Bool("confirm-idle", false, "operator confirms the desktop is idle")
	waitFlag := fs.Duration("wait", 5*time.Second, "pause after the pre-motion checks, before the idle samples")
	logFlag := fs.String("log", "", "exclusive evidence log; required with -run")
	if err := fs.Parse(args); err != nil {
		return exitRefuse
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "ddmovr: unexpected arguments")
		return exitRefuse
	}
	if *runFlag && *logFlag == "" {
		fmt.Fprintln(os.Stderr, "ddmovr: -run requires -log")
		return exitRefuse
	}

	lg, err := newLineLog(*logFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ddmovr: log: %v\n", err)
		return exitRefuse
	}
	defer lg.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logProtocol(lg)
	root, err := moduleRoot()
	if err != nil {
		lg.Line("git", kv("err", err.Error()))
	} else {
		logGit(lg, root)
		logSources(lg, root)
		logSiblings(lg, root, absPath(*dllFlag))
	}
	lg.Line("env", kv("godd_dll_set", os.Getenv("GODD_DLL") != ""), kv("dll_name", baseName(*dllFlag)), kv("run", *runFlag), kv("confirm_idle", *confirmFlag))

	facts := collectSystem()
	logFacts(lg, facts)
	names := driverNames(facts.Drivers)
	selectedName, selectWhy := SelectService(names, *driverFlag)
	drv, _ := findDriver(facts.Drivers, selectedName)
	lg.Line("driver_selected", kv("service", selectedName), kv("reason", selectWhy), kv("count", len(facts.Drivers)))

	place, placed := ChooseMonitor(facts.Monitors, facts.SystemDPI, EdgeMarginPx)
	lg.Line("placement",
		kv("ok", placed),
		kv("monitor", place.Monitor.Name),
		kv("home", coord(place.Home)),
		kv("bounds", rectText(place.Monitor.Bounds)),
		kv("dpi", place.Monitor.DPI),
		kv("safe_clear", coord(Point{place.SafeClearX, place.SafeClearY})),
		kv("phys_clear", coord(Point{place.PhysClearX, place.PhysClearY})),
	)

	var inspected dllInspect
	if *dllFlag != "" {
		inspected = inspectDLL(lg, *dllFlag, filepath.Base(drv.Image))
	} else {
		lg.Line("dll", kv("name", ""), kv("loaded", false), kv("note", "unset"))
	}

	if !*runFlag {
		lg.Line("done", kv("mode", "inventory"), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitOK
	}
	if !*confirmFlag {
		lg.Line("refuse", kv("exit", exitRefuse), kv("reason", "-run requires -confirm-idle"), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitRefuse
	}
	if why := motionRefused(facts, drv, selectWhy, inspected); why != "" {
		lg.Line("refuse", kv("exit", exitRefuse), kv("reason", why), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitRefuse
	}

	lg.Line("wait", kv("duration", waitFlag.String()), kv("note", "desktop must stay idle; Ctrl+C stops before the DLL is loaded"))
	if !pause(ctx, *waitFlag) {
		lg.Line("done", kv("exit", ExitInterrupted), kv("stage", "wait"), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitInterrupt
	}
	if !idleStable(ctx, lg) {
		if ctx.Err() != nil {
			lg.Line("done", kv("exit", ExitInterrupted), kv("stage", "idle"), kv("probe_moved_cursor", false), kv("loaded", false))
			return exitInterrupt
		}
		lg.Line("done", kv("exit", ExitContaminated), kv("stage", "idle"), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitAbort
	}
	again := readPointer()
	logPointer(lg, "pointer_after", again)
	ok, why := PointerUnchanged(
		facts.Pointer.SPISpeed, facts.Pointer.SPIThreshold1, facts.Pointer.SPIThreshold2, facts.Pointer.SPIEnhanced,
		again.SPISpeed, again.SPIThreshold1, again.SPIThreshold2, again.SPIEnhanced,
		facts.Pointer.SPIErr, again.SPIErr,
	)
	if !ok {
		lg.Line("done", kv("exit", ExitControl), kv("stage", "pointer_guard"), kv("reason", why), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitAbort
	}

	dd, err := godd.GetDD(inspected.Abs)
	if err != nil {
		lg.Line("init", kv("ok", false), kv("err", err.Error()), kv("loaded", false), kv("attempt", 1))
		lg.Line("done", kv("exit", ExitAPI), kv("stage", "init"), kv("probe_moved_cursor", false), kv("loaded", false))
		return exitAbort
	}
	lg.Line("init", kv("ok", true), kv("loaded", true), kv("attempt", 1), kv("name", baseName(inspected.Abs)))
	reason := execute(ctx, lg, dd, place)
	closeErr := dd.Close()
	lg.Line("close", kv("err", errText(closeErr)))
	lg.Line("done", kv("exit", reason), kv("probe_moved_cursor", true), kv("loaded", true))
	if reason == "" {
		return exitOK
	}
	if reason == ExitInterrupted {
		return exitInterrupt
	}
	return exitAbort
}

func logProtocol(lg *lineLog) {
	for _, line := range ProtocolLines() {
		lg.Line("protocol", kv("item", line))
	}
}

func logGit(lg *lineLog, root string) {
	g := readGit(root)
	lg.Line("git", kv("commit", g.Commit), kv("branch", g.Branch), kv("status", g.Status), kv("err", g.Err))
}

func logSources(lg *lineLog, root string) {
	dir := filepath.Join(root, "cmd", "ddmovr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		lg.Line("source", kv("err", err.Error()))
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".go") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		rel := filepath.ToSlash(filepath.Join("cmd", "ddmovr", name))
		sum, err := fileSHA256(filepath.Join(dir, name))
		lg.Line("source", kv("path", rel), kv("sha256", sum), kv("err", errText(err)))
	}
	sum, err := fileSHA256(filepath.Join(root, "godd.go"))
	lg.Line("source", kv("path", "godd.go"), kv("sha256", sum), kv("err", errText(err)))
}

func logSiblings(lg *lineLog, root, dllAbs string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		lg.Line("sibling", kv("err", err.Error()))
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".dll", ".sys", ".7z":
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(root, name)
		sum, err := fileSHA256(path)
		used := dllAbs != "" && strings.EqualFold(path, dllAbs)
		lg.Line("sibling", kv("name", name), kv("sha256", sum), kv("used", used), kv("err", errText(err)))
	}
}

func logFacts(lg *lineLog, s systemFacts) {
	lg.Line("os", kv("name", s.OSName), kv("version", s.OSVersion), kv("go", s.GoVersion))
	lg.Line("privilege",
		kv("elevated", s.Elevated), kv("elevation_err", s.ElevationErr),
		kv("integrity", s.Integrity), kv("integrity_rid", fmt.Sprintf("%#x", s.IntegrityRID)), kv("integrity_err", s.IntegrityErr),
		kv("session", s.Session), kv("console_session", s.ConsoleSession), kv("console_match", s.ConsoleMatch), kv("session_err", s.SessionErr),
	)
	lg.Line("dpi",
		kv("process_before", s.DPIBefore), kv("process_after", s.DPIAfter), kv("aware_ok", s.DPIAwareOK),
		kv("system", s.SystemDPI), kv("system_err", s.SystemDPIErr), kv("query_err", s.DPIQueryErr),
	)
	lg.Line("screen", kv("virtual", rectText(s.Virtual)), kv("cursor", coord(s.Cursor)), kv("cursor_err", s.CursorErr))
	for i, m := range s.Monitors {
		lg.Line("monitor",
			kv("i", i), kv("name", m.Name), kv("primary", m.Primary),
			kv("bounds", rectText(m.Bounds)), kv("work", rectText(m.Work)),
			kv("dpi", m.DPI), kv("note", m.DPINote),
		)
	}
	if s.MonitorErr != "" {
		lg.Line("monitor", kv("err", s.MonitorErr))
	}
	logPointer(lg, "pointer", s.Pointer)
	if s.DriverErr != "" {
		lg.Line("driver", kv("err", s.DriverErr))
	}
	for _, d := range s.Drivers {
		logDriver(lg, d)
	}
}

func logPointer(lg *lineLog, tag string, p PointerFacts) {
	lg.Line(tag,
		kv("spi_speed", p.SPISpeed), kv("spi_t1", p.SPIThreshold1), kv("spi_t2", p.SPIThreshold2), kv("spi_epp", p.SPIEnhanced), kv("spi_err", p.SPIErr),
		kv("reg_sensitivity", p.RegSensitivity), kv("reg_speed", p.RegSpeed), kv("reg_t1", p.RegT1), kv("reg_t2", p.RegT2), kv("reg_err", p.RegErr),
	)
}

func logDriver(lg *lineLog, d DriverFacts) {
	lg.Line("driver",
		kv("service", d.Service), kv("state", d.State), kv("state_err", d.StateErr), kv("start", d.Start),
		kv("image", baseName(d.Image)), kv("size", d.Size), kv("modified", d.Modified),
		kv("sha256", d.SHA256), kv("hash_err", d.HashErr),
		kv("file_version", d.FileVersion), kv("product_version", d.ProductVersion), kv("original_filename", d.OriginalFilename),
		kv("hardware_id", d.HardwareID), kv("inf_driver_ver", d.InfDriverVer),
		kv("store_agree", storeAgree(d)), kv("store_sha256", d.StoreSHA256),
	)
}

type dllInspect struct {
	Abs        string
	PE         PEInfo
	PEErr      string
	VersionErr string
	PairOK     bool
	PairWhy    string
	Err        string
}

func inspectDLL(lg *lineLog, path, driverBase string) dllInspect {
	var out dllInspect
	abs, err := filepath.Abs(path)
	if err != nil {
		out.Err = err.Error()
		lg.Line("dll", kv("name", baseName(path)), kv("err", out.Err), kv("loaded", false))
		return out
	}
	out.Abs = abs
	st, err := os.Stat(abs)
	if err != nil {
		out.Err = err.Error()
		lg.Line("dll", kv("name", baseName(abs)), kv("err", out.Err), kv("loaded", false))
		return out
	}
	sum, sumErr := fileSHA256(abs)
	pe, peErr := readPE(abs)
	out.PE = pe
	if peErr != nil {
		out.PEErr = peErr.Error()
	}
	fv, pv, orig, verErr := fileVersion(abs)
	if verErr != nil {
		out.VersionErr = verErr.Error()
	}
	id := DLLIdentity{Base: filepath.Base(abs), FileVersion: fv, ProductVersion: pv, OriginalFilename: orig}
	out.PairOK, out.PairWhy = PairOK(driverBase, id)
	lg.Line("dll",
		kv("name", baseName(abs)), kv("size", st.Size()), kv("sha256", sum), kv("hash_err", errText(sumErr)),
		kv("machine", pe.MachineName()), kv("exports", strings.Join(pe.Exports, ",")), kv("pe_err", out.PEErr),
		kv("file_version", fv), kv("product_version", pv), kv("original_filename", orig), kv("version_err", out.VersionErr),
		kv("driver_base", driverBase), kv("pair_ok", out.PairOK), kv("pair_reason", out.PairWhy), kv("loaded", false),
	)
	if sumErr != nil && out.Err == "" {
		out.Err = sumErr.Error()
	}
	return out
}

func motionRefused(s systemFacts, drv DriverFacts, selectWhy string, dll dllInspect) string {
	if selectWhy != "" {
		return selectWhy
	}
	if !s.ConsoleMatch || s.SessionErr != "" {
		return "process is not in the active console session"
	}
	if s.SystemDPI == 0 || s.SystemDPIErr != "" {
		return "system DPI unknown"
	}
	if _, ok := ChooseMonitor(s.Monitors, s.SystemDPI, EdgeMarginPx); !ok {
		return "no monitor matches system DPI"
	}
	if drv.State != "running" {
		return "driver is not running"
	}
	if ok, why := StoreAgrees(drv.SHA256, drv.StorePath, drv.StoreSHA256); !ok {
		return why
	}
	if dll.Err != "" {
		return dll.Err
	}
	if dll.Abs == "" {
		return "dll path required"
	}
	if dll.PEErr != "" || dll.PE.Machine != peMachineAMD64 {
		return "dll is not an amd64 PE"
	}
	for _, name := range requiredExports {
		if !dll.PE.HasExport(name) {
			return "dll missing export " + name
		}
	}
	if dll.VersionErr != "" {
		return "dll version resource unreadable"
	}
	if !dll.PairOK {
		if dll.PairWhy == "" {
			return "dll/driver token mismatch"
		}
		return dll.PairWhy
	}
	if s.Pointer.SPIErr != "" {
		return "spi unreadable before motion"
	}
	return ""
}

func driverNames(ds []DriverFacts) []string {
	names := make([]string, 0, len(ds))
	for _, d := range ds {
		names = append(names, d.Service)
	}
	return names
}

func findDriver(ds []DriverFacts, service string) (DriverFacts, bool) {
	for _, d := range ds {
		if strings.EqualFold(d.Service, service) {
			return d, true
		}
	}
	return DriverFacts{}, false
}

func idleStable(ctx context.Context, lg *lineLog) bool {
	start, err := cursorPos()
	if err != nil {
		lg.Line("idle", kv("ok", false), kv("err", err.Error()))
		return false
	}
	for i := 0; i < IdleSamples; i++ {
		if !pause(ctx, IdleGap) {
			lg.Line("idle", kv("ok", false), kv("exit", ExitInterrupted), kv("i", i+1))
			return false
		}
		got, err := cursorPos()
		if err != nil || got != start {
			lg.Line("idle", kv("ok", false), kv("i", i+1), kv("start", coord(start)), kv("got", coord(got)), kv("err", errText(err)))
			return false
		}
	}
	lg.Line("idle", kv("ok", true), kv("at", coord(start)), kv("samples", IdleSamples), kv("gap", IdleGap.String()))
	return true
}

func pause(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func storeAgree(d DriverFacts) string {
	ok, why := StoreAgrees(d.SHA256, d.StorePath, d.StoreSHA256)
	if ok && why == "" {
		return "equal"
	}
	if ok && why == "no store copy" {
		return "absent"
	}
	if why == "" {
		return "differ"
	}
	return why
}

func absPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func rectText(r Rect) string {
	return fmt.Sprintf("%d,%d,%d,%d", r.Left, r.Top, r.Right, r.Bottom)
}
