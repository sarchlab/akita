//go:build !linux

package hostsys

import (
	"github.com/shirou/gopsutil/v4/process"
)

// ProcOwner returns the uid of a process, or -1.
func ProcOwner(pid int) int {
	pr, err := process.NewProcess(int32(pid)) //nolint:gosec // pids fit in int32
	if err != nil {
		return -1
	}

	uids, err := pr.Uids()
	if err != nil || len(uids) == 0 {
		return -1
	}

	return int(uids[0])
}

// ProcCwd returns the working directory of a process ("" when it cannot
// be read).
func ProcCwd(pid int) string {
	pr, err := process.NewProcess(int32(pid)) //nolint:gosec // pids fit in int32
	if err != nil {
		return ""
	}

	d, _ := pr.Cwd()

	return d
}

// ProcStartedByHerd reports whether a process runs in a Herd session;
// only Linux can tell (its environment).
func ProcStartedByHerd(int) bool { return false }
