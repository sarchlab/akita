package workspace

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
)

// LiveSystem is what Liveness asks of the machine.
type LiveSystem interface {
	// PIDAlive tells whether a process exists.
	PIDAlive(pid int) bool
	// SlurmJob asks about a SLURM job once: "queued", "running", or ""
	// when it is not in the queue, then its final state.
	SlurmJob(ctx context.Context, id string) (state, final string)
}

// HostSystem is the LiveSystem of this machine.
type HostSystem struct{}

// PIDAlive tells whether a process of this machine exists.
func (HostSystem) PIDAlive(pid int) bool { return hostsys.PIDAlive(pid) }

// SlurmJob asks squeue and sacct of this machine about a job.
func (HostSystem) SlurmJob(ctx context.Context, id string) (state, final string) {
	return hostsys.SlurmJobState(ctx, id)
}

// LiveInput is what decides whether a session is alive.
type LiveInput struct {
	Finished bool // its log has an "[exit N]" or "[stopped]" marker
	Shadowed bool // an archived log whose name a log in logs/ has
	Tmux     bool // a tmux session has its exact tmux session name
	TmuxPID  int  // that tmux session's pane pid (0 when unknown)
	Job      *Job // its .job file; nil when it has none
}

// Live is a session's liveness.
type Live struct {
	State string // "running", "queued", or "" (not alive)
	PID   int    // root of the process tree of a running tmux or nohup session
	Final string // final SLURM state of a job that has left the queue
}

// Liveness is the one rule that tells whether a session is alive, used by
// the probe, the session listing, and the archive busy check: the
// finished markers of its log win; an archived log whose name a newer log
// has never owns a live session; otherwise its tmux session, its process,
// or its SLURM job (asked about once) decides.
func Liveness(ctx context.Context, sys LiveSystem, in LiveInput) Live {
	switch {
	case in.Finished, in.Shadowed:
		return Live{}
	case in.Tmux:
		return Live{State: "running", PID: in.TmuxPID}
	case in.Job == nil:
		return Live{}
	case in.Job.Kind == JobSlurm:
		state, final := sys.SlurmJob(ctx, in.Job.ID)
		return Live{State: state, Final: final}
	}

	pid, err := strconv.Atoi(in.Job.ID)
	if err != nil || !sys.PIDAlive(pid) {
		return Live{}
	}

	return Live{State: "running", PID: pid}
}

// Shadowed tells whether session name of logs/archived/ is shadowed by a
// newer session of the same name in logs/ (archivedDir's parent).
func Shadowed(archivedDir, name string) bool {
	return hostsys.Exists(SessionFile(filepath.Dir(archivedDir), name, LogExt))
}
