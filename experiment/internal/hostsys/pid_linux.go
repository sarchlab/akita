package hostsys

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// PIDAlive tells whether a process exists.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	_, err := os.Stat("/proc/" + strconv.Itoa(pid))

	return err == nil
}

// ProcStartTick is the start time of a process (clock ticks since boot),
// which tells a reused pid apart; 0 when the process does not exist.
func ProcStartTick(pid int) uint64 {
	f := readProcStat(pid)
	if f == nil {
		return 0
	}

	v, _ := strconv.ParseUint(f[statStartIdx], 10, 64)

	return v
}

// ProcessArgs returns the command line of every process of this machine.
func ProcessArgs(context.Context) map[int][]string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	out := map[int][]string{}

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		data, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil || len(data) == 0 {
			continue
		}

		out[pid] = strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	}

	return out
}
