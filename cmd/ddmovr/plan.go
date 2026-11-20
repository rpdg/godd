package main

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Probe limits are the predeclared measurement contract. They are printed
// before any cursor motion. They are not an acceptance test for winput.
const (
	EdgeMarginPx      = 80
	TolerancePx       = 1
	MaxSteps          = 12
	MaxStepPx         = 40
	TrialTimeout      = 3 * time.Second
	InterStep         = 16 * time.Millisecond
	AnomalySlackPx    = 15
	HardGain          = 8
	AdmissionHeadroom = 2
	FirstStepCeilPx   = 30
	EarlyMaxRequestPx = 10
	BurstCount        = 5
	SettleAgreePx     = 1
	CrosstalkLimitPx  = 1
)

// Exit tokens recorded by the probe. AbortsRun is the set that stops the
// whole process. A trial is never repeated.
const (
	ExitConverged      = "converged"
	ExitMaxSteps       = "max-steps"
	ExitTimeout        = "timeout"
	ExitAPI            = "api-error"
	ExitEdge           = "edge"
	ExitLeftMonitor    = "left-monitor"
	ExitDiverged       = "diverged"
	ExitAnomaly        = "anomaly-delta"
	ExitContaminated   = "contaminated"
	ExitSkipped        = "skipped-clearance"
	ExitInterrupted    = "interrupted"
	ExitNoDisplacement = "no-displacement"
	ExitEdgeRisk       = "edge-risk"
	ExitControl        = "control-failed"
)

const (
	RepositionSettle = 100 * time.Millisecond
	ControlSettle    = 250 * time.Millisecond
	IdleSamples      = 5
	IdleGap          = 100 * time.Millisecond
	Sample20         = 20 * time.Millisecond
	Sample80         = 80 * time.Millisecond
	Late50           = 50 * time.Millisecond
	Late150          = 150 * time.Millisecond
)

// AbortsRun reports whether a trial outcome stops every later trial.
// max-steps, timeout, skipped-clearance, and converged are results, not retries.
func AbortsRun(reason string) bool {
	switch reason {
	case ExitAPI, ExitEdge, ExitLeftMonitor, ExitDiverged, ExitAnomaly,
		ExitContaminated, ExitNoDisplacement, ExitInterrupted, ExitEdgeRisk, ExitControl:
		return true
	default:
		return false
	}
}

// Rect is a half-open pixel rectangle [Left, Right) x [Top, Bottom).
type Rect struct {
	Left, Top, Right, Bottom int
}

func (r Rect) Width() int  { return r.Right - r.Left }
func (r Rect) Height() int { return r.Bottom - r.Top }

func (r Rect) Contains(x, y int) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// InsideSafe reports whether p is at least margin pixels away from every
// clamp edge of b.
func InsideSafe(p Point, b Rect, margin int) bool {
	if margin < 0 || b.Width() <= 2*margin || b.Height() <= 2*margin {
		return false
	}
	inner := Rect{
		Left: b.Left + margin, Top: b.Top + margin,
		Right: b.Right - margin, Bottom: b.Bottom - margin,
	}
	return inner.Contains(p.X, p.Y)
}

// OnEdge is true on the outer pixel row, where Windows clamps the cursor.
func (r Rect) OnEdge(x, y int) bool {
	if r.Width() <= 0 || r.Height() <= 0 {
		return true
	}
	return x <= r.Left || y <= r.Top || x >= r.Right-1 || y >= r.Bottom-1
}

// Point is a cursor position in the probe process's coordinate space.
type Point struct {
	X, Y int
}

func (p Point) Delta(q Point) (int, int) { return q.X - p.X, q.Y - p.Y }

type Monitor struct {
	Name    string
	Bounds  Rect
	Work    Rect
	DPI     uint32
	DPINote string
	Primary bool
}

// Placement is the interior home used for every trial.
type Placement struct {
	Monitor                Monitor
	Home                   Point
	SafeClearX, SafeClearY int
	PhysClearX, PhysClearY int
}

// PlaceMonitor picks the home point at the center of b.
// Safe clearance is the distance from home to the margin inset.
// Physical clearance is the distance from home to the clamp edge.
func PlaceMonitor(b Rect, margin int) (Placement, bool) {
	if b.Width() <= 2*margin || b.Height() <= 2*margin || margin < 0 {
		return Placement{}, false
	}
	home := Point{X: b.Left + b.Width()/2, Y: b.Top + b.Height()/2}
	physX := smaller(home.X-b.Left, b.Right-1-home.X)
	physY := smaller(home.Y-b.Top, b.Bottom-1-home.Y)
	safeX := physX - margin
	safeY := physY - margin
	if physX <= 0 || physY <= 0 || safeX <= 0 || safeY <= 0 {
		return Placement{}, false
	}
	return Placement{
		Monitor:    Monitor{Bounds: b},
		Home:       home,
		SafeClearX: safeX, SafeClearY: safeY,
		PhysClearX: physX, PhysClearY: physY,
	}, true
}

// ChooseMonitor selects the display whose system-DPI coordinates are physical
// pixels and whose safe interior is the largest. A DPI mismatch is refused
// rather than measured in a mixed coordinate space. systemDPI 0 means the
// system DPI could not be read; matching monitors cannot be chosen.
func ChooseMonitor(mons []Monitor, systemDPI uint32, margin int) (Placement, bool) {
	if systemDPI == 0 {
		return Placement{}, false
	}
	var best Placement
	ok := false
	bestScore := -1
	for _, m := range mons {
		if m.DPI != systemDPI {
			continue
		}
		p, placed := PlaceMonitor(m.Bounds, margin)
		if !placed {
			continue
		}
		p.Monitor = m
		score := smaller(p.SafeClearX, p.SafeClearY)
		if score > bestScore {
			best = p
			bestScore = score
			ok = true
		}
	}
	return best, ok
}

type Move struct {
	Name   string
	DX, DY int
}

type Burst struct {
	Name   string
	DX, DY int
	Count  int
	Gap    time.Duration
}

func (b Burst) Travel() (int, int) { return b.DX * b.Count, b.DY * b.Count }

func AxisTrials() []Move {
	var out []Move
	for _, n := range []int{1, 10, 50, 200} {
		out = append(out,
			Move{"axis+x" + strconv.Itoa(n), n, 0},
			Move{"axis-x" + strconv.Itoa(n), -n, 0},
			Move{"axis+y" + strconv.Itoa(n), 0, n},
			Move{"axis-y" + strconv.Itoa(n), 0, -n},
		)
	}
	return out
}

func DiagonalTrials() []Move {
	const n = 50
	return []Move{
		{"diag+x+y50", n, n},
		{"diag+x-y50", n, -n},
		{"diag-x+y50", -n, n},
		{"diag-x-y50", -n, -n},
	}
}

func BurstTrials() []Burst {
	return []Burst{
		{"burst+x-gap0", 20, 0, BurstCount, 0},
		{"burst+x-gap8", 20, 0, BurstCount, 8 * time.Millisecond},
		{"burst+x-gap30", 20, 0, BurstCount, 30 * time.Millisecond},
		{"burst-x+y-gap8", -15, 15, BurstCount, 8 * time.Millisecond},
	}
}

func LoopTrials() []Move {
	return []Move{
		{"loop-short+x", 30, 0},
		{"loop-short-y", 0, -25},
		{"loop-short-x+y", -20, 15},
		{"loop-long+x", 240, 0},
		{"loop-long+y", 0, 180},
		{"loop-long-x+y", -160, 120},
	}
}

func IsLong(dx, dy int) bool { return Chebyshev(dx, dy) >= 120 }

func Chebyshev(x, y int) int { return larger(abs(x), abs(y)) }

func smaller(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func larger(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StepRequest caps one closed-loop correction. The cap is what makes a long
// target take more than one DD_movR. It is not a pointer trajectory.
func StepRequest(remX, remY, maxStep int) (int, int) {
	if maxStep < 0 {
		maxStep = 0
	}
	return Clamp(remX, -maxStep, maxStep), Clamp(remY, -maxStep, maxStep)
}

// AdmitEarly allows only the small ladder, and only when HardGain stays off
// the clamp edge. Measured gain does not exist yet.
func AdmitEarly(dx, dy, physX, physY int) bool {
	if Chebyshev(dx, dy) == 0 || Chebyshev(dx, dy) > EarlyMaxRequestPx {
		return false
	}
	return fitsStrict(dx, HardGain, physX) && fitsStrict(dy, HardGain, physY)
}

// fitsStrict is true when a zero request has no travel, and a nonzero request
// amplified by gain stops at least one pixel before the clearance limit.
func fitsStrict(v, gain, clear int) bool {
	if v == 0 {
		return true
	}
	return abs(v)*gain < clear
}

// Admit reports whether a requested displacement fits in the safe interior
// when amplified by max(gain, 1) and AdmissionHeadroom.
func Admit(dx, dy int, gain float64, clearX, clearY int) bool {
	if clearX <= 0 || clearY <= 0 {
		return false
	}
	g := gain
	if g < 1 {
		g = 1
	}
	return admitAxis(dx, g, clearX) && admitAxis(dy, g, clearY)
}

func admitAxis(v int, gain float64, clear int) bool {
	if v == 0 {
		return AnomalySlackPx <= clear
	}
	need := int(math.Ceil(math.Abs(float64(v))*gain*AdmissionHeadroom)) + AnomalySlackPx
	return need <= clear
}

// UnsafeDelta is the hard ceiling checked after a call returns.
// A 1-count request uses FirstStepCeilPx because quantization can exceed HardGain.
func UnsafeDelta(reqX, reqY, obsX, obsY int) bool {
	req := Chebyshev(reqX, reqY)
	obs := Chebyshev(obsX, obsY)
	if req <= 1 {
		return obs > FirstStepCeilPx
	}
	return obs > req*HardGain
}

// SameDirection is false when the cursor moves against the request by more
// than AnomalySlackPx. Overshoot in the requested direction stays true.
func SameDirection(reqX, reqY, obsX, obsY int) bool {
	if obsX == 0 && obsY == 0 {
		return true
	}
	dot := int64(reqX)*int64(obsX) + int64(reqY)*int64(obsY)
	if dot > 0 {
		return true
	}
	return Chebyshev(obsX, obsY) <= AnomalySlackPx
}

// LoopExit gives timeout and max-steps only when the cursor is still outside
// tolerance. Convergence wins when both would otherwise be true.
func LoopExit(errX, errY, steps int, elapsed, timeout time.Duration) string {
	if abs(errX) <= TolerancePx && abs(errY) <= TolerancePx {
		return ExitConverged
	}
	if timeout > 0 && elapsed > timeout {
		return ExitTimeout
	}
	if steps >= MaxSteps {
		return ExitMaxSteps
	}
	return ""
}

type AxisObs struct {
	ReqX, ReqY int
	ObsX, ObsY int
	Usable     bool
}

// CleanAlongRatio is the signed displacement along a pure axis request.
// Crosstalk above CrosstalkLimitPx excludes the sample from the aggregate.
func CleanAlongRatio(o AxisObs) (float64, bool) {
	if !o.Usable {
		return 0, false
	}
	var along, cross int
	switch {
	case o.ReqX != 0 && o.ReqY == 0:
		along, cross = o.ObsX, o.ObsY
	case o.ReqY != 0 && o.ReqX == 0:
		along, cross = o.ObsY, o.ObsX
	default:
		return 0, false
	}
	if abs(cross) > CrosstalkLimitPx {
		return 0, false
	}
	return float64(along) / float64(axisReq(o)), true
}

func axisReq(o AxisObs) int {
	if o.ReqX != 0 {
		return o.ReqX
	}
	return o.ReqY
}

// MaxInteriorRatio is the largest chebyshev(obs)/chebyshev(req) among usable
// samples. It is a safety gain, including unsettled motion, not a unit claim.
func MaxInteriorRatio(samples []AxisObs) (float64, int) {
	maxR := 0.0
	n := 0
	for _, o := range samples {
		if !o.Usable {
			continue
		}
		req := Chebyshev(o.ReqX, o.ReqY)
		if req == 0 {
			continue
		}
		r := float64(Chebyshev(o.ObsX, o.ObsY)) / float64(req)
		if r > maxR {
			maxR = r
		}
		n++
	}
	return maxR, n
}

func NoMotion(obsX, obsY int) bool { return obsX == 0 && obsY == 0 }

// SignConflict is true when an interior sample moved against its axis request
// by more than one pixel. The ladder stops before a larger step walks away.
func SignConflict(samples []AxisObs) bool {
	for _, o := range samples {
		if o.ReqX != 0 && o.ReqY == 0 && o.ObsX < -CrosstalkLimitPx {
			return true
		}
		if o.ReqY != 0 && o.ReqX == 0 && o.ObsY < -CrosstalkLimitPx {
			return true
		}
	}
	return false
}

var digitRun = regexp.MustCompile(`\d{4,}`)

// DigitRuns returns digit runs of length 4 or more. Callers pass a file base
// name or a version string, not a full path, so a directory number cannot match.
func DigitRuns(s string) []string {
	if s == "" {
		return nil
	}
	return digitRun.FindAllString(s, -1)
}

// DLLIdentity is the on-disk identity of one DLL file.
type DLLIdentity struct {
	Base             string
	FileVersion      string
	ProductVersion   string
	OriginalFilename string
}

// PairOK reports whether every version token on the DLL equals the token in
// the driver base name. A missing DLL token, or any disagreeing token, fails.
// Empty version fields are not tokens and do not prove a match by themselves.
func PairOK(driverBase string, dll DLLIdentity) (bool, string) {
	driver := DigitRuns(driverBase)
	if len(driver) != 1 {
		return false, "driver token missing or ambiguous"
	}
	name := DigitRuns(dll.Base)
	if len(name) != 1 {
		return false, "dll filename token missing or ambiguous"
	}
	if name[0] != driver[0] {
		return false, "filename token mismatch"
	}
	for _, field := range []struct {
		name, value string
	}{
		{"FileVersion", dll.FileVersion},
		{"ProductVersion", dll.ProductVersion},
		{"OriginalFilename", dll.OriginalFilename},
	} {
		runs := DigitRuns(field.value)
		for _, run := range runs {
			if run != driver[0] {
				return false, field.name + " token mismatch"
			}
		}
	}
	return true, ""
}

// WrapperRet maps a godd.MoveRelative error to the DLL code the wrapper
// accepted. A nil error is code 1, because that is the only success value
// godd treats as success. The boolean is false when the text has no code.
func WrapperRet(err error) (int32, bool) {
	if err == nil {
		return 1, true
	}
	const key = "failed (code "
	s := err.Error()
	i := strings.LastIndex(s, key)
	if i < 0 {
		return 0, false
	}
	rest := s[i+len(key):]
	j := strings.IndexByte(rest, ')')
	if j < 0 {
		return 0, false
	}
	n, conv := strconv.ParseInt(rest[:j], 10, 32)
	if conv != nil {
		return 0, false
	}
	return int32(n), true
}

// SelectService picks the installed DD driver service. A name is required when
// more than one dd*.sys service is present. The match is case-insensitive.
func SelectService(names []string, want string) (string, string) {
	if want != "" {
		for _, name := range names {
			if strings.EqualFold(name, want) {
				return name, ""
			}
		}
		return "", "named driver not found"
	}
	switch len(names) {
	case 0:
		return "", "no dd driver"
	case 1:
		return names[0], ""
	default:
		return "", "multiple dd drivers; pass -driver"
	}
}

// StoreAgrees reports whether the running driver image and its DriverStore
// copy are the same file. A missing store copy is recorded and allowed. An
// ambiguous copy, a missing hash, or a different hash refuses the run.
func StoreAgrees(imageHash, storePath, storeHash string) (bool, string) {
	if storePath == "ambiguous" {
		return false, "driver store copy ambiguous"
	}
	if storePath == "" {
		return true, "no store copy"
	}
	if imageHash == "" || storeHash == "" {
		return false, "store or image hash missing"
	}
	if !strings.EqualFold(imageHash, storeHash) {
		return false, "store hash differs from system image"
	}
	return true, ""
}

// PointerUnchanged compares the SPI mouse speed, thresholds, and enhancement
// flag read before the wait with the same values read after the idle check.
func PointerUnchanged(speed, t1, t2, epp, speed2, t1b, t2b, epp2 int, err1, err2 string) (bool, string) {
	if err1 != "" || err2 != "" {
		return false, "spi read failed"
	}
	if speed != speed2 || t1 != t1b || t2 != t2b || epp != epp2 {
		return false, "spi changed"
	}
	return true, ""
}

// ProtocolLines is the contract printed before the wait and before any motion.
func ProtocolLines() []string {
	return []string{
		"probe=ddmovr purpose=DD_movR relative displacement and a capped closed-loop measurement",
		"nonclaim=probe algorithm is not a godd API and is not the winput trajectory",
		"nonclaim=a converged token does not accept a later winput change",
		"nonclaim=one (+50,+50) sample is not a convergence result",
		"tolerance_px=" + strconv.Itoa(TolerancePx),
		"max_steps=" + strconv.Itoa(MaxSteps),
		"trial_timeout=" + TrialTimeout.String(),
		"max_step_px=" + strconv.Itoa(MaxStepPx),
		"inter_step=" + InterStep.String(),
		"admission_headroom=" + strconv.Itoa(AdmissionHeadroom),
		"edge_margin_px=" + strconv.Itoa(EdgeMarginPx),
		"anomaly_slack_px=" + strconv.Itoa(AnomalySlackPx),
		"hard_gain=" + strconv.Itoa(HardGain),
		"samples_ms=0,20,80",
		"burst_gaps_ms=0,8,30",
		"burst_count=" + strconv.Itoa(BurstCount),
		"pass_converged=abs error <= tolerance on both axes, within max_steps and trial_timeout, every sample inside the safe rect, wrapper ret 1",
		"pass_unit_sample=wrapper ret 1, sample stayed in the safe rect, sample at 0ms and 80ms differ by at most 1px per axis, crosstalk <= 1px",
		"run_abort=api-error edge left-monitor diverged anomaly-delta contaminated no-displacement interrupted edge-risk control-failed",
		"not_run_abort=converged max-steps timeout skipped-clearance",
		"exit_priority=converged,timeout,max-steps",
		"sign_stop=after +/-10, against-axis motion beyond 1px stops the run; one call aborts immediately only when inversion exceeds anomaly_slack_px",
		"reposition_settle=" + RepositionSettle.String(),
		"control_settle=" + ControlSettle.String(),
		"idle=" + strconv.Itoa(IdleSamples) + "x" + IdleGap.String(),
		"no_retry=each trial runs at most once",
		"interrupt=Ctrl+C is observed between DLL calls; a call inside the DLL cannot be cancelled; killing the process skips cursor restore",
		"environment=no pointer, DPI, driver, or package change; this process may call SetProcessDPIAware",
		"reposition=SetCursorPos returns to home between trials; closed-loop steps call only DD_movR",
		"wrapper_ret=MoveRelative nil is logged as ret 1; other codes are parsed from the wrapper error",
		"pair=DLL filename and version tokens must equal the installed driver token; other DLLs are not loaded",
		"pointer_guard=SPI speed and enhancement are compared again after the idle check; a difference aborts before motion",
	}
}
