package hostsys

import (
	"bytes"
	"os"
	"strconv"
	"syscall"
)

// maxEnviron bounds the environment read from /proc/<pid>/environ.
const maxEnviron = 256 << 10

// ProcOwner returns the uid of a process, or -1.
func ProcOwner(pid int) int {
	info, err := os.Stat("/proc/" + strconv.Itoa(pid))
	if err != nil {
		return -1
	}

	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}

	return -1
}

// ProcCwd returns the working directory of a process ("" when it cannot
// be read, e.g. for another user's process).
func ProcCwd(pid int) string {
	d, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	if err != nil {
		return ""
	}

	return d
}

// ProcStartedByHerd reports whether a process runs in a Herd session: its
// environment has HERD_NAME, HERD_TASK, HERD_HEARTBEAT, or EXP_HEARTBEAT
// (sessions of the former exp tool).
func ProcStartedByHerd(pid int) bool {
	env, err := ReadLimited("/proc/"+strconv.Itoa(pid)+"/environ", maxEnviron)
	if err != nil {
		return false
	}

	for _, kv := range bytes.Split(env, []byte{0}) {
		for _, key := range []string{"HERD_NAME=", "HERD_TASK=", "HERD_HEARTBEAT=", "EXP_HEARTBEAT="} {
			if bytes.HasPrefix(kv, []byte(key)) {
				return true
			}
		}
	}

	return false
}
