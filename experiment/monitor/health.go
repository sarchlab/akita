package monitor

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// Hang rule constants.
const (
	rateWindow    = 600.0 // seconds of heartbeat samples for rates
	minRateSpan   = 30.0  // seconds of samples before a rate is shown
	secondsPerMin = 60
)

// cpuSample is the CPU history of a running session.
type cpuSample struct {
	total    float64  // CPU seconds of the process tree
	at       float64  // probe time
	lowSince *float64 // since when CPU has been below the idle threshold
}

// progressTrack is the progress history of a running session.
type progressTrack struct {
	pid     float64
	sim     float64
	simAt   float64
	work    float64
	workAt  float64
	samples [][3]float64 // time, sim, work (sampleSim, sampleWork)
}

// Columns of progressTrack.samples.
const (
	sampleSim  = 1
	sampleWork = 2
)

// healthJudge applies the hang rules across polls.
type healthJudge struct {
	rules    experiment.Rules
	cpu      map[string]*cpuSample
	progress map[string]*progressTrack
}

func newHealthJudge(r experiment.Rules) *healthJudge {
	return &healthJudge{rules: r, cpu: map[string]*cpuSample{}, progress: map[string]*progressTrack{}}
}

// judgeHost fills the health of the running sessions of one host's probe
// (probe.Now is the clock) and marks hung ones "stalled".
func (j *healthJudge) judgeHost(host string, p *experiment.HostProbe) {
	seen := map[string]bool{}

	for i := range p.Sessions {
		s := &p.Sessions[i]
		s.Health = experiment.Health{}

		if s.State != "running" {
			continue
		}

		key := host + "/" + s.Project + "/" + s.Name
		seen[key] = true

		if j.judge(key, s, p.Now) {
			s.State = "stalled"
		}
	}

	j.forget(host, seen)
}

// forget drops the history of a host's sessions not in seen.
func (j *healthJudge) forget(host string, seen map[string]bool) {
	prefix := host + "/"

	for k := range j.cpu {
		if strings.HasPrefix(k, prefix) && !seen[k] {
			delete(j.cpu, k)
		}
	}

	for k := range j.progress {
		if strings.HasPrefix(k, prefix) && !seen[k] {
			delete(j.progress, k)
		}
	}
}

// judge applies the rules to one running session and reports a hang.
func (j *healthJudge) judge(key string, s *experiment.Session, now float64) bool {
	stalled := j.cpuRule(key, s, now)
	s.Health.HangBasis = "cpu"

	if basis, hung, ok := j.progressRule(key, s, workspace.ParseHeartbeat(s.Heartbeat), now); ok {
		s.Health.HangBasis = basis
		if basis == "heartbeat" {
			// The heartbeat replaces the CPU and output rules.
			s.Health.OutputIdle = false
			return hung
		}

		return stalled || hung
	}

	return stalled
}

// cpuRule: the process tree's CPU use (between two polls) stays below
// IdleCPU percent for StallMinutes. It also sets OutputIdle: a busy
// session whose log and open files have not changed for StallMinutes.
func (j *healthJudge) cpuRule(key string, s *experiment.Session, now float64) bool {
	stallS := float64(j.rules.StallMinutes * secondsPerMin)
	total := 0.0

	for _, p := range s.Procs {
		total += p.CPUTime
	}

	var lowSince *float64

	if prev := j.cpu[key]; prev != nil && now > prev.at && total >= prev.total {
		cpuNow := 100 * (total - prev.total) / (now - prev.at)
		s.Health.CPUNow = new(cpuNow)

		if cpuNow < float64(j.rules.IdleCPU) {
			lowSince = prev.lowSince
			if lowSince == nil {
				lowSince = new(prev.at)
			}
		}
	}

	j.cpu[key] = &cpuSample{total: total, at: now, lowSince: lowSince}

	stalled := false

	if lowSince != nil {
		s.Health.LowFor = new(now - *lowSince)
		stalled = *s.Health.LowFor >= stallS
	}

	if stalled {
		s.Health.HangReason = fmt.Sprintf("CPU below %d%% for %d minutes",
			j.rules.IdleCPU, int(*s.Health.LowFor)/secondsPerMin)
	}

	s.Health.OutputIdle = !stalled && now-s.LastOutput >= stallS

	return stalled
}

// progressRule judges by progress instead of CPU: the heartbeat
// (simulated time or completed work stops advancing, or the heartbeat
// itself stops), else live Akita monitor bars (completed work stops
// advancing). ok is false when the session has neither.
func (j *healthJudge) progressRule(key string, s *experiment.Session, hb *workspace.Heartbeat, now float64,
) (basis string, hung, ok bool) {
	if hb != nil && !hb.Finished && heartbeatPIDLive(hb, s) {
		s.Health.HangReason = ""

		return "heartbeat", j.heartbeatRule(key, s, hb, now), true
	}

	work, ok := monitorWork(s)
	if !ok {
		delete(j.progress, key)
		return "", false, false
	}

	t := j.track(key, -1, 0, work, now)
	s.Health.WorkStillFor = new(now - t.workAt)
	s.Health.WGRate = rate(t, now, sampleWork)

	if *s.Health.WorkStillFor >= float64(j.rules.WorkStallMinutes*secondsPerMin) {
		s.Health.HangReason = fmt.Sprintf("monitor progress has not advanced for %d minutes",
			int(*s.Health.WorkStillFor)/secondsPerMin)

		return "monitor", true, true
	}

	return "monitor", false, true
}

// heartbeatPIDLive tells whether the heartbeat's writer is a process of
// the session.
func heartbeatPIDLive(hb *workspace.Heartbeat, s *experiment.Session) bool {
	if hb.PID == nil {
		return false
	}

	pid := *hb.PID

	for _, p := range s.Procs {
		if float64(p.PID) == pid {
			return true
		}
	}

	for _, p := range s.Sims {
		if p.PID != 0 && float64(p.PID) == pid {
			return true
		}
	}

	return false
}

// monitorWork sums the finished counts of a session's Akita monitor bars.
func monitorWork(s *experiment.Session) (float64, bool) {
	work, ok := 0.0, false

	for _, b := range s.Progress {
		if strings.HasPrefix(b.ID, experiment.ProgressPrefixAkita) {
			work += float64(b.Finished)
			ok = true
		}
	}

	return work, ok
}

// track records a progress sample and returns the session's track.
func (j *healthJudge) track(key string, pid, sim, work, now float64) *progressTrack {
	t := j.progress[key]
	if t == nil || t.pid != pid {
		t = &progressTrack{pid: pid, sim: sim, simAt: now, work: work, workAt: now}
		j.progress[key] = t
	}

	if sim != t.sim {
		t.sim, t.simAt = sim, now
	}

	if work != t.work {
		t.work, t.workAt = work, now
	}

	kept := t.samples[:0]

	for _, x := range t.samples {
		if now-x[0] <= rateWindow {
			kept = append(kept, x)
		}
	}

	t.samples = append(kept, [3]float64{now, sim, work})

	return t
}

// rate is the change per second of sample column col over the window.
func rate(t *progressTrack, now float64, col int) *float64 {
	first := t.samples[0]
	span := now - first[0]

	if span < minRateSpan {
		return nil
	}

	last := t.samples[len(t.samples)-1]

	return new((last[col] - first[col]) / span)
}

func (j *healthJudge) heartbeatRule(key string, s *experiment.Session, hb *workspace.Heartbeat, now float64) bool {
	pid := 0.0
	if hb.PID != nil {
		pid = *hb.PID
	}

	work, total := hb.Work()

	t := j.track(key, pid, hb.SimTimeNS, work, now)
	h := &s.Health
	h.SimRate = rate(t, now, sampleSim)
	h.WGRate = rate(t, now, sampleWork)

	if total > 0 && h.WGRate != nil && *h.WGRate > 0 {
		h.ETA = new(max(0, total-work) / *h.WGRate)
	}

	h.SimStillFor = new(now - t.simAt)
	h.WorkStillFor = new(now - t.workAt)

	wall := now
	if hb.Wall != nil {
		wall = *hb.Wall
	}

	h.HeartbeatAge = new(now - wall)

	simStallS := float64(j.rules.SimStallMinutes * secondsPerMin)

	switch {
	case *h.HeartbeatAge >= simStallS:
		h.HangReason = fmt.Sprintf("heartbeat not updated for %d minutes (process may be stuck)",
			int(*h.HeartbeatAge)/secondsPerMin)
	case *h.SimStillFor >= simStallS:
		h.HangReason = fmt.Sprintf("simulated time has not advanced for %d minutes",
			int(*h.SimStillFor)/secondsPerMin)
	case *h.WorkStillFor >= float64(j.rules.WorkStallMinutes*secondsPerMin):
		h.HangReason = fmt.Sprintf("completed work has not increased for %d minutes "+
			"(simulated time still advancing)", int(*h.WorkStillFor)/secondsPerMin)
	default:
		return false
	}

	return true
}
