//go:build !linux

package hostsys

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

// PIDAlive tells whether a process exists.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	// Signal 0 (what PidExists sends here) only checks existence.
	ok, err := process.PidExists(int32(pid))

	return err == nil && ok
}

// ProcStartTick is the creation time of a process (ms since the epoch),
// which tells a reused pid apart; 0 when the process does not exist.
func ProcStartTick(pid int) uint64 {
	pr, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0
	}

	t, err := pr.CreateTime()
	if err != nil || t < 0 {
		return 0
	}

	return uint64(t)
}

// ProcessArgs returns the command line of every process of this machine.
func ProcessArgs(ctx context.Context) map[int][]string {
	list, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}

	out := map[int][]string{}

	for _, pr := range list {
		if args, err := pr.CmdlineSliceWithContext(ctx); err == nil && len(args) > 0 {
			out[int(pr.Pid)] = args
		}
	}

	return out
}
