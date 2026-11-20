//go:build windows

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rpdg/godd"
)

// runner measures DD_movR. The closed loop is a fixed-cap instrument: each
// step requests at most MaxStepPx, with no jitter and no curve. It is not the
// winput trajectory and its exit token does not accept that algorithm.
type runner struct {
	ctx   context.Context
	log   *lineLog
	dd    *godd.DD
	place Placement

	gain  float64
	gainN int
	stop  string

	unitMin, unitMax float64
	unitN            int
	loopN            int
	converged        int
	skipped          int
	shots            int
}

func execute(ctx context.Context, log *lineLog, dd *godd.DD, place Placement) string {
	r := &runner{ctx: ctx, log: log, dd: dd, place: place}
	saved, err := cursorPos()
	if err != nil {
		log.Line("setup", kv("op", "GetCursorPos"), kv("err", err.Error()))
		return ExitControl
	}
	defer func() {
		if err := setCursor(saved.X, saved.Y); err != nil {
			log.Line("restore", kv("ok", false), kv("err", err.Error()))
			return
		}
		got, gerr := cursorPos()
		log.Line("restore", kv("ok", gerr == nil), kv("want", coord(saved)), kv("got", coord(got)), kv("err", errText(gerr)))
	}()

	if err := setCursor(place.Home.X, place.Home.Y); err != nil {
		log.Line("control", kv("ok", false), kv("err", err.Error()))
		return ExitControl
	}
	if !r.sleep(ControlSettle) {
		return r.stop
	}
	got, err := cursorPos()
	if err != nil || got != place.Home {
		log.Line("control", kv("ok", false), kv("want", coord(place.Home)), kv("got", coord(got)), kv("err", errText(err)))
		return ExitControl
	}
	log.Line("control", kv("ok", true), kv("at", coord(got)), kv("settle", ControlSettle.String()))

	r.early()
	r.axis(50)
	r.diagonals()
	r.axis(200)
	r.bursts()
	r.loops()
	r.summary()
	return r.stop
}

func (r *runner) summary() {
	minR, maxR := "", ""
	if r.unitN > 0 {
		minR = fmt.Sprintf("%.4f", r.unitMin)
		maxR = fmt.Sprintf("%.4f", r.unitMax)
	}
	r.log.Line("summary",
		kv("plan_complete", r.stop == ""),
		kv("abort", r.stop),
		kv("shots", r.shots),
		kv("unit_samples", r.unitN),
		kv("unit_ratio_min", minR),
		kv("unit_ratio_max", maxR),
		kv("loops", r.loopN),
		kv("converged", r.converged),
		kv("skipped", r.skipped),
		kv("gain", fmt.Sprintf("%.4f", r.gain)),
		kv("note", "exit 0 means the plan finished; it does not mean every loop converged or that winput is accepted"),
	)
}

func (r *runner) stopped() bool { return r.stop != "" }

func (r *runner) early() {
	if r.stopped() {
		return
	}
	var planned []Move
	for _, m := range AxisTrials() {
		if Chebyshev(m.DX, m.DY) <= EarlyMaxRequestPx {
			planned = append(planned, m)
		}
	}
	for _, m := range planned {
		if !AdmitEarly(m.DX, m.DY, r.place.PhysClearX, r.place.PhysClearY) {
			r.log.Line("skip", kv("trial", m.Name), kv("reason", ExitEdgeRisk), kv("stage", "early"))
			r.stop = ExitEdgeRisk
			return
		}
	}
	var ten []AxisObs
	for _, m := range planned {
		obs, ok := r.single(m)
		if !ok {
			return
		}
		if Chebyshev(m.DX, m.DY) == 10 {
			ten = append(ten, obs)
		}
	}
	if len(ten) != 4 {
		r.log.Line("result", kv("stage", "axis10"), kv("exit", ExitEdgeRisk), kv("n", len(ten)))
		r.stop = ExitEdgeRisk
		return
	}
	allZero := true
	for _, o := range ten {
		if !NoMotion(o.ObsX, o.ObsY) {
			allZero = false
		}
	}
	if allZero {
		r.log.Line("result", kv("stage", "axis10"), kv("exit", ExitNoDisplacement))
		r.stop = ExitNoDisplacement
		return
	}
	// This is the only SignConflict call. classify uses SameDirection, which
	// already compares the dot product and stops one call only past AnomalySlackPx.
	if SignConflict(ten) {
		r.log.Line("result", kv("stage", "axis10"), kv("exit", ExitDiverged))
		r.stop = ExitDiverged
		return
	}
	ratio, n := MaxInteriorRatio(ten)
	r.gain, r.gainN = ratio, n
	r.log.Line("gain", kv("source", "axis10"), kv("max_ratio", fmt.Sprintf("%.4f", ratio)), kv("n", n))
	if n == 0 {
		r.stop = ExitNoDisplacement
	}
}

func (r *runner) axis(n int) {
	if r.stopped() {
		return
	}
	for _, m := range AxisTrials() {
		if Chebyshev(m.DX, m.DY) != n {
			continue
		}
		r.admittedShot(m)
	}
}

func (r *runner) diagonals() {
	if r.stopped() {
		return
	}
	for _, m := range DiagonalTrials() {
		r.admittedShot(m)
	}
}

func (r *runner) admittedShot(m Move) {
	if r.stopped() {
		return
	}
	if !Admit(m.DX, m.DY, r.gain, r.place.SafeClearX, r.place.SafeClearY) {
		r.skipped++
		r.log.Line("skip", kv("trial", m.Name), kv("reason", ExitSkipped),
			kv("req", coord(Point{m.DX, m.DY})), kv("gain", fmt.Sprintf("%.4f", r.gain)))
		return
	}
	obs, ok := r.single(m)
	if !ok || !obs.Usable {
		return
	}
	r.raiseGain(obs)
}

func (r *runner) raiseGain(o AxisObs) {
	ratio, n := MaxInteriorRatio([]AxisObs{o})
	if n == 1 && ratio > r.gain {
		r.gain = ratio
		r.log.Line("gain", kv("source", "raised"), kv("max_ratio", fmt.Sprintf("%.4f", r.gain)))
	}
}

func (r *runner) single(m Move) (AxisObs, bool) {
	if !r.home() {
		return AxisObs{}, false
	}
	before, err := cursorPos()
	if err != nil {
		r.stop = ExitControl
		r.log.Line("shot", kv("trial", m.Name), kv("exit", ExitControl), kv("err", err.Error()))
		return AxisObs{}, false
	}
	ret, callErr, dt := r.move(m.DX, m.DY)
	if r.stop == ExitInterrupted {
		return AxisObs{}, false
	}
	returned := time.Now()
	at0, err0 := cursorPos()
	if !r.sleepUntil(returned.Add(Sample20)) {
		r.logShot(m, ret, callErr, dt, before, at0, at0, at0, false, false, r.stop)
		return AxisObs{}, false
	}
	at20, err20 := cursorPos()
	if !r.sleepUntil(returned.Add(Sample80)) {
		r.logShot(m, ret, callErr, dt, before, at0, at20, at20, false, false, r.stop)
		return AxisObs{}, false
	}
	at80, err80 := cursorPos()
	readErr := firstErr(err0, err20, err80)
	if readErr != nil {
		r.stop = ExitControl
		r.logShot(m, ret, callErr, dt, before, at0, at20, at80, false, false, ExitControl)
		return AxisObs{}, false
	}
	reason := ""
	if callErr != nil {
		reason = ExitAPI
	} else if r.ctx.Err() != nil {
		reason = ExitInterrupted
	} else {
		reason = r.classify(m.DX, m.DY, before, at80)
	}
	immediate, settled := ShotStability(at0, at20, at80)
	r.logShot(m, ret, callErr, dt, before, at0, at20, at80, immediate, settled, reason)
	obs := AxisObs{
		ReqX: m.DX, ReqY: m.DY,
		ObsX: at80.X - before.X, ObsY: at80.Y - before.Y,
		Usable: reason == "" && callErr == nil,
	}
	if obs.Usable && settled {
		r.noteUnit(obs)
	}
	if reason != "" {
		r.stop = reason
		return obs, false
	}
	return obs, true
}

func (r *runner) logShot(m Move, ret int32, callErr error, dt time.Duration, before, at0, at20, at80 Point, immediate, settled bool, reason string) {
	r.shots++
	r.log.Line("shot",
		kv("trial", m.Name),
		kv("req", coord(Point{m.DX, m.DY})),
		kv("ret", ret),
		kv("ret_parsed", parsedRet(callErr)),
		kv("err", errText(callErr)),
		kv("before", coord(before)),
		kv("at0", coord(at0)),
		kv("at20", coord(at20)),
		kv("at80", coord(at80)),
		kv("d0", coord(Point{at0.X - before.X, at0.Y - before.Y})),
		kv("d20", coord(Point{at20.X - before.X, at20.Y - before.Y})),
		kv("d80", coord(Point{at80.X - before.X, at80.Y - before.Y})),
		kv("call_us", dt.Microseconds()),
		kv("immediate", immediate),
		kv("settled", settled),
		kv("exit", reason),
	)
}

func (r *runner) noteUnit(o AxisObs) {
	ratio, ok := CleanAlongRatio(o)
	if !ok {
		return
	}
	if r.unitN == 0 {
		r.unitMin, r.unitMax = ratio, ratio
	} else {
		if ratio < r.unitMin {
			r.unitMin = ratio
		}
		if ratio > r.unitMax {
			r.unitMax = ratio
		}
	}
	r.unitN++
}

func (r *runner) classify(reqX, reqY int, before, after Point) string {
	if !r.place.Monitor.Bounds.Contains(after.X, after.Y) || !r.place.Monitor.Bounds.Contains(before.X, before.Y) {
		return ExitLeftMonitor
	}
	if r.place.Monitor.Bounds.OnEdge(after.X, after.Y) {
		return ExitEdge
	}
	dx, dy := after.X-before.X, after.Y-before.Y
	if UnsafeDelta(reqX, reqY, dx, dy) {
		return ExitAnomaly
	}
	if !SameDirection(reqX, reqY, dx, dy) {
		return ExitDiverged
	}
	if !InsideSafe(before, r.place.Monitor.Bounds, EdgeMarginPx) || !InsideSafe(after, r.place.Monitor.Bounds, EdgeMarginPx) {
		return ExitEdgeRisk
	}
	return ""
}

func (r *runner) bursts() {
	if r.stopped() {
		return
	}
	for _, b := range BurstTrials() {
		if r.stopped() {
			return
		}
		tx, ty := b.Travel()
		if !Admit(tx, ty, r.gain, r.place.SafeClearX, r.place.SafeClearY) {
			r.skipped++
			r.log.Line("skip", kv("trial", b.Name), kv("reason", ExitSkipped),
				kv("travel", coord(Point{tx, ty})), kv("gain", fmt.Sprintf("%.4f", r.gain)))
			continue
		}
		r.oneBurst(b)
	}
}

func (r *runner) oneBurst(b Burst) {
	if !r.home() {
		return
	}
	start, err := cursorPos()
	if err != nil {
		r.stop = ExitControl
		return
	}
	immediate := start
	for i := 0; i < b.Count; i++ {
		if err := r.ctx.Err(); err != nil {
			r.stop = ExitInterrupted
			return
		}
		before, err := cursorPos()
		if err != nil {
			r.stop = ExitControl
			return
		}
		ret, callErr, dt := r.move(b.DX, b.DY)
		if r.stop == ExitInterrupted {
			return
		}
		after, aerr := cursorPos()
		r.shots++
		r.log.Line("burst-step",
			kv("trial", b.Name), kv("i", i+1),
			kv("req", coord(Point{b.DX, b.DY})),
			kv("ret", ret), kv("ret_parsed", parsedRet(callErr)), kv("err", errText(callErr)),
			kv("before", coord(before)), kv("after", coord(after)),
			kv("call_us", dt.Microseconds()), kv("read_err", errText(aerr)),
		)
		if aerr != nil {
			r.stop = ExitControl
			return
		}
		if callErr != nil {
			r.stop = ExitAPI
			return
		}
		if r.ctx.Err() != nil {
			r.stop = ExitInterrupted
			return
		}
		if reason := r.classify(b.DX, b.DY, before, after); reason != "" {
			r.stop = reason
			r.log.Line("result", kv("trial", b.Name), kv("exit", reason), kv("i", i+1))
			return
		}
		immediate = after
		if i+1 < b.Count && !r.sleep(b.Gap) {
			return
		}
	}
	if !r.sleep(Late50) {
		return
	}
	late50, _ := cursorPos()
	if !r.sleep(Late150 - Late50) {
		return
	}
	late150, err := cursorPos()
	dx, dy := immediate.X-start.X, immediate.Y-start.Y
	r.log.Line("burst",
		kv("trial", b.Name), kv("gap", b.Gap.String()), kv("count", b.Count),
		kv("req_each", coord(Point{b.DX, b.DY})),
		kv("travel_req", coord(Point{b.DX * b.Count, b.DY * b.Count})),
		kv("start", coord(start)), kv("immediate", coord(immediate)),
		kv("delta", coord(Point{dx, dy})),
		kv("late50", coord(late50)), kv("late150", coord(late150)),
		kv("late_delta", coord(Point{late150.X - immediate.X, late150.Y - immediate.Y})),
		kv("read_err", errText(err)),
	)
	if err != nil {
		r.stop = ExitControl
		return
	}
	if reason := r.classify(b.DX*b.Count, b.DY*b.Count, start, late150); reason != "" {
		r.stop = reason
		return
	}
	obs := AxisObs{ReqX: b.DX * b.Count, ReqY: b.DY * b.Count, ObsX: late150.X - start.X, ObsY: late150.Y - start.Y, Usable: true}
	r.raiseGain(obs)
}

func (r *runner) loops() {
	if r.stopped() {
		return
	}
	for _, m := range LoopTrials() {
		if r.stopped() {
			return
		}
		if !Admit(m.DX, m.DY, r.gain, r.place.SafeClearX, r.place.SafeClearY) {
			r.skipped++
			r.log.Line("skip", kv("trial", m.Name), kv("reason", ExitSkipped),
				kv("req", coord(Point{m.DX, m.DY})), kv("gain", fmt.Sprintf("%.4f", r.gain)))
			continue
		}
		r.oneLoop(m)
	}
}

func (r *runner) oneLoop(m Move) {
	if !r.home() {
		return
	}
	target := Point{X: r.place.Home.X + m.DX, Y: r.place.Home.Y + m.DY}
	started := time.Now()
	steps := 0
	reason := ""
	var last Point
	for {
		if err := r.ctx.Err(); err != nil {
			reason = ExitInterrupted
			break
		}
		before, err := cursorPos()
		if err != nil {
			reason = ExitControl
			break
		}
		last = before
		remX, remY := target.X-before.X, target.Y-before.Y
		if exit := LoopExit(remX, remY, steps, time.Since(started), TrialTimeout); exit != "" {
			reason = exit
			break
		}
		if stepReason := r.classify(remX, remY, before, before); stepReason != "" {
			reason = stepReason
			break
		}
		reqX, reqY := StepRequest(remX, remY, MaxStepPx)
		t0 := time.Now()
		ret, callErr, dt := r.move(reqX, reqY)
		if r.stop == ExitInterrupted {
			reason = ExitInterrupted
			break
		}
		after, aerr := cursorPos()
		steps++
		r.shots++
		stepMS := time.Since(t0).Milliseconds()
		r.log.Line("step",
			kv("trial", m.Name), kv("n", steps),
			kv("req", coord(Point{reqX, reqY})),
			kv("ret", ret), kv("err", errText(callErr)),
			kv("before", coord(before)), kv("after", coord(after)),
			kv("rem_before", coord(Point{remX, remY})),
			kv("rem_after", coord(Point{target.X - after.X, target.Y - after.Y})),
			kv("call_us", dt.Microseconds()), kv("step_ms", stepMS),
			kv("read_err", errText(aerr)),
		)
		if callErr != nil {
			reason = ExitAPI
			break
		}
		if aerr != nil {
			reason = ExitControl
			break
		}
		last = after
		if stepReason := r.classify(reqX, reqY, before, after); stepReason != "" {
			reason = stepReason
			break
		}
		if !r.sleep(InterStep) {
			reason = r.stop
			break
		}
		settled, serr := cursorPos()
		if serr != nil {
			reason = ExitControl
			break
		}
		if settled != after {
			r.log.Line("step-settle", kv("trial", m.Name), kv("n", steps),
				kv("after", coord(after)), kv("settled", coord(settled)),
				kv("delta", coord(Point{settled.X - after.X, settled.Y - after.Y})))
		}
		last = settled
	}
	if reason == "" {
		reason = ExitInterrupted
	}
	errX, errY := target.X-last.X, target.Y-last.Y
	r.loopN++
	if reason == ExitConverged {
		r.converged++
	}
	r.log.Line("result",
		kv("trial", m.Name), kv("exit", reason), kv("steps", steps),
		kv("elapsed_ms", time.Since(started).Milliseconds()),
		kv("target", coord(target)), kv("final", coord(last)),
		kv("err_px", coord(Point{errX, errY})),
		kv("within_1px", abs(errX) <= TolerancePx && abs(errY) <= TolerancePx),
		kv("winput_human_band_abs_lt_3", abs(errX) < 3 && abs(errY) < 3),
	)
	if AbortsRun(reason) {
		r.stop = reason
	}
}

func (r *runner) home() bool {
	if r.stopped() {
		return false
	}
	if err := r.ctx.Err(); err != nil {
		r.stop = ExitInterrupted
		return false
	}
	if err := setCursor(r.place.Home.X, r.place.Home.Y); err != nil {
		r.log.Line("setup", kv("op", "SetCursorPos"), kv("err", err.Error()))
		r.stop = ExitControl
		return false
	}
	if !r.sleep(RepositionSettle) {
		return false
	}
	got, err := cursorPos()
	if err != nil {
		r.stop = ExitControl
		r.log.Line("setup", kv("op", "reposition"), kv("err", err.Error()))
		return false
	}
	if got != r.place.Home {
		r.log.Line("setup", kv("op", "reposition"), kv("want", coord(r.place.Home)), kv("got", coord(got)))
		r.stop = ExitContaminated
		return false
	}
	return true
}

func (r *runner) move(dx, dy int) (int32, error, time.Duration) {
	if err := r.ctx.Err(); err != nil {
		r.stop = ExitInterrupted
		return 0, err, 0
	}
	t := time.Now()
	err := r.dd.MoveRelative(dx, dy)
	dt := time.Since(t)
	code, ok := WrapperRet(err)
	if err != nil && !ok {
		return 0, err, dt
	}
	return code, err, dt
}

func (r *runner) sleep(d time.Duration) bool {
	return r.sleepUntil(time.Now().Add(d))
}

func (r *runner) sleepUntil(t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		if err := r.ctx.Err(); err != nil {
			r.stop = ExitInterrupted
			return false
		}
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-r.ctx.Done():
		r.stop = ExitInterrupted
		return false
	case <-timer.C:
		return true
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func parsedRet(err error) bool {
	_, ok := WrapperRet(err)
	return ok
}
