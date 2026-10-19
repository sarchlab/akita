package hostsys

import (
	"bufio"
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Linux /proc constants: clock ticks per second (USER_HZ, 100 on every
// mainstream kernel) and the fields of /proc/<pid>/stat after "(comm)".
const (
	userHZ        = 100
	statStateIdx  = 0  // field 3 of stat
	statPPIDIdx   = 1  // field 4
	statUTimeIdx  = 11 // field 14
	statSTimeIdx  = 12 // field 15
	statStartIdx  = 19 // field 22
	statRSSIdx    = 21 // field 24
	statMinFields = 22

	procStatLineBuf = 64 << 10 // initial line buffer of /proc/stat
	procStatLineMax = 1 << 20  // longest /proc/stat line read
)

// procfsTable reads the process tree from /proc directly (gopsutil reads
// /proc/stat once per process, which is slow on many-core machines).
func procfsTable() *ProcTable {
	boot := bootTime()
	if boot == 0 {
		return nil
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	t := newTable()
	stats := map[int][]string{}
	pageKB := float64(os.Getpagesize()) / 1024
	now := float64(time.Now().UnixNano()) / 1e9

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		f := readProcStat(pid)
		if f == nil {
			continue
		}

		ppid, _ := strconv.Atoi(f[statPPIDIdx])
		stats[pid] = f
		t.add(pid, ppid)
	}

	t.load = func(pid int) *ProcInfo {
		return procfsInfo(pid, stats[pid], boot, now, pageKB)
	}

	return t
}

// readProcStat returns the fields of /proc/<pid>/stat after "(comm)".
func readProcStat(pid int) []string {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil
	}

	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return nil
	}

	f := strings.Fields(string(data[i+1:]))
	if len(f) < statMinFields+1 {
		return nil
	}

	return f
}

func procfsInfo(pid int, f []string, boot, now, pageKB float64) *ProcInfo {
	if f == nil {
		return nil
	}

	num := func(i int) float64 {
		v, _ := strconv.ParseFloat(f[i], 64)
		return v
	}

	cmd, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")

	return &ProcInfo{
		Etime:   math.Floor(max(0, now-(boot+num(statStartIdx)/userHZ))),
		CPUTime: math.Floor((num(statUTimeIdx) + num(statSTimeIdx)) / userHZ),
		RSSKB:   procRSSKB(pid, num(statRSSIdx)*pageKB),
		Stat:    f[statStateIdx],
		Args:    strings.TrimSpace(strings.ReplaceAll(string(cmd), "\x00", " ")),
	}
}

// procRSSKB returns VmRSS of /proc/<pid>/status (what ps shows), or def.
func procRSSKB(pid int, def float64) float64 {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return def
	}

	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			f := strings.Fields(v)
			if len(f) > 0 {
				if kb, err := strconv.ParseFloat(f[0], 64); err == nil {
					return kb
				}
			}
		}
	}

	return def
}

// bootTime reads the boot time (Unix seconds) from /proc/stat.
func bootTime() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, procStatLineBuf), procStatLineMax)

	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "btime "); ok {
			b, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
			return b
		}
	}

	return 0
}

// NewestWritten returns the newest mtime (Unix seconds) among the regular
// files a process has open, or 0.
func NewestWritten(pid int) float64 {
	dir := "/proc/" + strconv.Itoa(pid) + "/fd"

	fds, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	var newest float64

	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(dir, fd.Name()))
		if err != nil || !strings.HasPrefix(target, "/") ||
			strings.HasPrefix(target, "/dev/") || strings.HasPrefix(target, "/proc/") {
			continue
		}

		st, err := os.Stat(target)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}

		newest = max(newest, float64(st.ModTime().UnixNano())/1e9)
	}

	return newest
}
