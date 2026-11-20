package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMatrixCoversSignsMagnitudesAndLoops(t *testing.T) {
	axis := AxisTrials()
	if len(axis) != 16 {
		t.Fatalf("axis trials = %d, want 16", len(axis))
	}
	for _, n := range []int{1, 10, 50, 200} {
		for _, want := range []Move{
			{"", n, 0}, {"", -n, 0}, {"", 0, n}, {"", 0, -n},
		} {
			if !hasMove(axis, want.DX, want.DY) {
				t.Errorf("missing axis (%d,%d)", want.DX, want.DY)
			}
		}
	}
	diag := DiagonalTrials()
	for _, s := range [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		if !hasMove(diag, 50*s[0], 50*s[1]) {
			t.Errorf("missing diagonal (%d,%d)", 50*s[0], 50*s[1])
		}
	}
	var shortN, longN int
	var sawPos, sawNeg bool
	for _, m := range LoopTrials() {
		if IsLong(m.DX, m.DY) {
			longN++
		} else {
			shortN++
		}
		if m.DX > 0 || m.DY > 0 {
			sawPos = true
		}
		if m.DX < 0 || m.DY < 0 {
			sawNeg = true
		}
	}
	if shortN < 3 || longN < 3 || !sawPos || !sawNeg {
		t.Fatalf("loops short=%d long=%d pos=%v neg=%v", shortN, longN, sawPos, sawNeg)
	}
	gaps := map[time.Duration]bool{}
	for _, b := range BurstTrials() {
		gaps[b.Gap] = true
		if b.Count != BurstCount {
			t.Errorf("%s count = %d", b.Name, b.Count)
		}
	}
	for _, g := range []time.Duration{0, 8 * time.Millisecond, 30 * time.Millisecond} {
		if !gaps[g] {
			t.Errorf("missing burst gap %s", g)
		}
	}
}

func hasMove(ms []Move, dx, dy int) bool {
	for _, m := range ms {
		if m.DX == dx && m.DY == dy {
			return true
		}
	}
	return false
}

func TestPlaceAndChooseMonitor(t *testing.T) {
	left := Rect{Left: -1920, Top: 0, Right: 0, Bottom: 1080}
	primary := Rect{Left: 0, Top: 0, Right: 1366, Bottom: 768}
	leftP, ok := PlaceMonitor(left, EdgeMarginPx)
	if !ok {
		t.Fatal("left monitor did not place")
	}
	if leftP.Home != (Point{-960, 540}) {
		t.Fatalf("left home = %+v", leftP.Home)
	}
	if leftP.PhysClearX != 959 || leftP.PhysClearY != 539 {
		t.Fatalf("left physical = %d,%d", leftP.PhysClearX, leftP.PhysClearY)
	}
	if leftP.SafeClearX != 879 || leftP.SafeClearY != 459 {
		t.Fatalf("left safe = %d,%d", leftP.SafeClearX, leftP.SafeClearY)
	}
	priP, ok := PlaceMonitor(primary, EdgeMarginPx)
	if !ok {
		t.Fatal("primary did not place")
	}
	mons := []Monitor{
		{Name: `\\.\DISPLAY1`, Bounds: primary, DPI: 96, Primary: true},
		{Name: `\\.\DISPLAY2`, Bounds: left, DPI: 96, Primary: false},
	}
	got, ok := ChooseMonitor(mons, 96, EdgeMarginPx)
	if !ok || got.Monitor.Name != `\\.\DISPLAY2` {
		t.Fatalf("choose = %+v ok=%v", got.Monitor, ok)
	}
	if _, ok := ChooseMonitor(mons, 120, EdgeMarginPx); ok {
		t.Fatal("mismatched system DPI was accepted")
	}
	if _, ok := ChooseMonitor(mons, 0, EdgeMarginPx); ok {
		t.Fatal("unknown system DPI was accepted")
	}
	mixed := []Monitor{
		{Name: "primary", Bounds: primary, DPI: 96, Primary: true},
		{Name: "left", Bounds: left, DPI: 144, Primary: false},
	}
	got, ok = ChooseMonitor(mixed, 96, EdgeMarginPx)
	if !ok || got.Monitor.Name != "primary" {
		t.Fatalf("dpi filter = %+v ok=%v", got.Monitor, ok)
	}
	if priP.SafeClearY >= leftP.SafeClearY {
		t.Fatal("fixture no longer distinguishes the two monitors")
	}
	if !InsideSafe(leftP.Home, left, EdgeMarginPx) {
		t.Fatal("home is outside the safe rect")
	}
	if InsideSafe(Point{X: left.Left, Y: leftP.Home.Y}, left, EdgeMarginPx) {
		t.Fatal("clamp edge was treated as safe")
	}
}

func TestAdmission(t *testing.T) {
	if !AdmitEarly(10, 0, 959, 539) || !AdmitEarly(0, -1, 682, 383) {
		t.Fatal("early ladder should fit both fixtures")
	}
	if AdmitEarly(50, 0, 959, 539) {
		t.Fatal("early admission accepted a 50px request")
	}
	if AdmitEarly(10, 0, 70, 539) {
		t.Fatal("early admission can reach the clamp edge")
	}
	// 1080p safe clearance from PlaceMonitor, gain 1, headroom 2.
	if !Admit(200, 0, 1, 879, 459) || !Admit(0, -200, 1, 879, 459) {
		t.Fatal("200px axis should fit the larger monitor at gain 1")
	}
	if !Admit(240, 0, 1, 879, 459) || !Admit(0, 180, 1, 879, 459) || !Admit(-160, 120, 1, 879, 459) {
		t.Fatal("long loop targets should fit the larger monitor at gain 1")
	}
	if Admit(0, 200, 1, 602, 303) {
		t.Fatal("200px Y should not fit the 768-tall monitor")
	}
	if Admit(200, 0, 3, 879, 459) {
		t.Fatal("gain 3 should reject a 200px step on the larger monitor")
	}
	if Admit(10, 0, 1, 0, 459) {
		t.Fatal("zero clearance was admitted")
	}
}

func TestStepSafetyAndExit(t *testing.T) {
	if x, y := StepRequest(100, -5, MaxStepPx); x != MaxStepPx || y != -5 {
		t.Fatalf("step = %d,%d", x, y)
	}
	if x, y := StepRequest(30, 0, MaxStepPx); x != 30 || y != 0 {
		t.Fatalf("short step = %d,%d", x, y)
	}
	if UnsafeDelta(1, 0, FirstStepCeilPx, 0) || !UnsafeDelta(1, 0, FirstStepCeilPx+1, 0) {
		t.Fatal("1px ceiling")
	}
	if UnsafeDelta(10, 0, 80, 0) || !UnsafeDelta(10, 0, 81, 0) {
		t.Fatal("10px ceiling")
	}
	if !SameDirection(50, 0, 80, 2) {
		t.Fatal("overshoot should stay same-direction")
	}
	if SameDirection(50, 0, -16, 0) {
		t.Fatal("inversion beyond slack was accepted")
	}
	if !SameDirection(50, 0, -15, 0) {
		t.Fatal("inversion inside slack should not abort")
	}
	if LoopExit(0, 1, MaxSteps, TrialTimeout+time.Millisecond, TrialTimeout) != ExitConverged {
		t.Fatal("convergence must win over timeout and the step cap")
	}
	if LoopExit(2, 0, 3, TrialTimeout+time.Millisecond, TrialTimeout) != ExitTimeout {
		t.Fatal("timeout")
	}
	if LoopExit(2, 0, MaxSteps, 0, TrialTimeout) != ExitMaxSteps {
		t.Fatal("max steps")
	}
	if LoopExit(2, 0, 3, time.Millisecond, TrialTimeout) != "" {
		t.Fatal("loop should continue")
	}
}

func TestRatiosSignAndAbortClass(t *testing.T) {
	clean := []AxisObs{
		{ReqX: 10, ObsX: 10, Usable: true},
		{ReqX: -10, ObsX: -10, Usable: true},
		{ReqY: 10, ObsY: 20, Usable: true},
	}
	r, ok := CleanAlongRatio(clean[2])
	if !ok || r != 2 {
		t.Fatalf("along = %v %v", r, ok)
	}
	if _, ok := CleanAlongRatio(AxisObs{ReqX: 10, ObsX: 10, ObsY: 2, Usable: true}); ok {
		t.Fatal("crosstalk was aggregated")
	}
	if _, ok := CleanAlongRatio(AxisObs{ReqX: 10, ObsX: 10, Usable: false}); ok {
		t.Fatal("unusable sample was aggregated")
	}
	got, n := MaxInteriorRatio(append(clean, AxisObs{ReqX: 10, ObsX: 100, Usable: false}))
	if n != 3 || got != 2 {
		t.Fatalf("max ratio = %v n=%d", got, n)
	}
	if !SignConflict([]AxisObs{{ReqX: 10, ObsX: -2}}) {
		t.Fatal("expected sign conflict")
	}
	if SignConflict([]AxisObs{{ReqX: 10, ObsX: -1}}) {
		t.Fatal("one pixel against the request is not a ladder stop")
	}
	for _, reason := range []string{ExitAPI, ExitEdge, ExitLeftMonitor, ExitDiverged, ExitAnomaly, ExitContaminated, ExitNoDisplacement, ExitInterrupted, ExitEdgeRisk, ExitControl} {
		if !AbortsRun(reason) {
			t.Errorf("%s should abort", reason)
		}
	}
	for _, reason := range []string{ExitConverged, ExitMaxSteps, ExitTimeout, ExitSkipped} {
		if AbortsRun(reason) {
			t.Errorf("%s should not abort the run", reason)
		}
	}
}

func TestPairAndWrapperRet(t *testing.T) {
	ok, why := PairOK("ddhid63340.sys", DLLIdentity{
		Base: "ddhid.63340.dll", FileVersion: "ddhid63340",
		ProductVersion: "ddhid63340", OriginalFilename: "ddhid.63340.dll",
	})
	if !ok || why != "" {
		t.Fatalf("matching pair ok=%v why=%s", ok, why)
	}
	if ok, _ := PairOK("ddhid63340.sys", DLLIdentity{Base: "dd63330.dll", FileVersion: "DD63330"}); ok {
		t.Fatal("simple-mode DLL matched the HID driver")
	}
	if ok, _ := PairOK("ddhid63340.sys", DLLIdentity{Base: "hid.72000.dll", FileVersion: "hid.72000"}); ok {
		t.Fatal("72000 DLL matched the 63340 driver")
	}
	if ok, _ := PairOK("ddhid72000.sys", DLLIdentity{Base: "hid.72000.dll"}); !ok {
		t.Fatal("72000 pair should match on filename tokens")
	}
	renamed := DLLIdentity{Base: "ddhid.63340.dll", OriginalFilename: "dd.71000.dll"}
	if ok, _ := PairOK("ddhid63340.sys", renamed); ok {
		t.Fatal("renamed file hid a different original filename")
	}
	base := filepathBaseOnly(`D:\pkg\72000\ddhid.63340.dll`)
	if base != "ddhid.63340.dll" {
		t.Fatalf("base = %s", base)
	}
	if ok, why := PairOK("ddhid63340.sys", DLLIdentity{Base: base}); !ok {
		t.Fatal(why)
	}
	code, ok := WrapperRet(nil)
	if code != 1 || !ok {
		t.Fatalf("nil ret = %d %v", code, ok)
	}
	code, ok = WrapperRet(errors.New("godd: DD_movR failed (code -7): access is denied"))
	if code != -7 || !ok {
		t.Fatalf("parsed ret = %d %v", code, ok)
	}
	if _, ok := WrapperRet(errors.New("godd: load failed")); ok {
		t.Fatal("unrelated error looked like a DLL code")
	}
}

func filepathBaseOnly(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	return path[i+1:]
}

func TestProtocolIsFixedBeforeMotion(t *testing.T) {
	text := strings.Join(ProtocolLines(), "\n")
	for _, want := range []string{
		"tolerance_px=1",
		"max_steps=12",
		"trial_timeout=3s",
		"max_step_px=40",
		"Ctrl+C",
		"not the winput trajectory",
		"(+50,+50)",
		"no pointer, DPI, driver, or package change",
		"each trial runs at most once",
		"ret 1",
		"exit_priority=converged,timeout,max-steps",
		"reposition_settle=100ms",
		"control_settle=250ms",
		"idle=5x100ms",
		"pointer_guard=SPI speed and enhancement",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("protocol missing %q", want)
		}
	}
}

func TestSelectStoreAndPointerGuards(t *testing.T) {
	name, why := SelectService(nil, "")
	if name != "" || why != "no dd driver" {
		t.Fatalf("empty: %q %q", name, why)
	}
	name, why = SelectService([]string{"ddhid63340"}, "")
	if name != "ddhid63340" || why != "" {
		t.Fatalf("single: %q %q", name, why)
	}
	name, why = SelectService([]string{"ddhid63340", "ddhid72000"}, "")
	if name != "" || why == "" {
		t.Fatalf("multiple: %q %q", name, why)
	}
	name, why = SelectService([]string{"ddhid63340", "ddhid72000"}, "DDHID63340")
	if name != "ddhid63340" || why != "" {
		t.Fatalf("named: %q %q", name, why)
	}
	if _, why = SelectService([]string{"ddhid63340"}, "other"); why != "named driver not found" {
		t.Fatalf("missing name: %q", why)
	}

	if ok, why := StoreAgrees("abc", "", ""); !ok || why != "no store copy" {
		t.Fatalf("absent store: %v %q", ok, why)
	}
	if ok, _ := StoreAgrees("abc", "ambiguous", ""); ok {
		t.Fatal("ambiguous store accepted")
	}
	if ok, _ := StoreAgrees("abc", `C:\drv.sys`, ""); ok {
		t.Fatal("missing store hash accepted")
	}
	if ok, _ := StoreAgrees("abc", `C:\drv.sys`, "def"); ok {
		t.Fatal("different store hash accepted")
	}
	if ok, why := StoreAgrees("AbC", `C:\drv.sys`, "abc"); !ok || why != "" {
		t.Fatalf("same hash: %v %q", ok, why)
	}

	if ok, why := PointerUnchanged(10, 6, 10, 1, 10, 6, 10, 1, "", ""); !ok || why != "" {
		t.Fatalf("same pointer: %v %q", ok, why)
	}
	if ok, _ := PointerUnchanged(10, 6, 10, 1, 10, 6, 10, 0, "", ""); ok {
		t.Fatal("epp change accepted")
	}
	if ok, why := PointerUnchanged(10, 6, 10, 1, 10, 6, 10, 1, "read", ""); ok || why != "spi read failed" {
		t.Fatalf("spi error: %v %q", ok, why)
	}
}
