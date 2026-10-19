package hostsys

import (
	"context"
	"math"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// ProcInfo is one process of the process table.
type ProcInfo struct {
	Etime   float64 // seconds since start
	CPUTime float64 // user + system seconds
	RSSKB   float64
	Stat    string // ps-style state letter
	Args    string
}

// ProcTable is the machine's process tree; details are read lazily.
type ProcTable struct {
	children map[int][]int
	procs    map[int]bool
	details  map[int]*ProcInfo
	load     func(pid int) *ProcInfo
}

// NewProcTable makes a process table from the parent of each process and
// a function that reads a process's details.
func NewProcTable(ppid map[int]int, load func(pid int) *ProcInfo) *ProcTable {
	t := newTable()
	for pid, parent := range ppid {
		t.add(pid, parent)
	}

	t.load = load

	return t
}

func newTable() *ProcTable {
	return &ProcTable{children: map[int][]int{}, procs: map[int]bool{}, details: map[int]*ProcInfo{}}
}

func (t *ProcTable) add(pid, ppid int) {
	t.procs[pid] = true
	t.children[ppid] = append(t.children[ppid], pid)
}

// Descendants returns pid and the processes below it.
func (t *ProcTable) Descendants(pid int) []int {
	var out []int

	seen := map[int]bool{}
	stack := []int{pid}

	for len(stack) > 0 {
		q := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if seen[q] {
			continue
		}

		seen[q] = true
		out = append(out, q)
		stack = append(stack, t.children[q]...)
	}

	return out
}

// Info returns the details of a process, or nil.
func (t *ProcTable) Info(pid int) *ProcInfo {
	if d, ok := t.details[pid]; ok {
		return d
	}

	var d *ProcInfo
	if t.procs[pid] && t.load != nil {
		d = t.load(pid)
	}

	t.details[pid] = d

	return d
}

// Processes reads the machine's process table: from /proc on Linux, with
// gopsutil elsewhere (or when /proc cannot be read).
func Processes(ctx context.Context) *ProcTable {
	if t := procfsTable(); t != nil {
		return t
	}

	t := newTable()
	handles := map[int]*process.Process{}

	list, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return t
	}

	for _, pr := range list {
		ppid, err := pr.PpidWithContext(ctx)
		if err != nil {
			continue
		}

		handles[int(pr.Pid)] = pr
		t.add(int(pr.Pid), int(ppid))
	}

	t.load = func(pid int) *ProcInfo { return loadProcInfo(ctx, handles[pid]) }

	return t
}

func loadProcInfo(ctx context.Context, pr *process.Process) *ProcInfo {
	created, err := pr.CreateTimeWithContext(ctx)
	if err != nil {
		return nil
	}

	info := &ProcInfo{Etime: math.Floor(max(0, float64(time.Now().UnixMilli()-created)/1000))}

	if times, err := pr.TimesWithContext(ctx); err == nil {
		info.CPUTime = math.Floor(times.User + times.System)
	}

	if m, err := pr.MemoryInfoWithContext(ctx); err == nil {
		info.RSSKB = float64(m.RSS) / 1024
	}

	if st, err := pr.StatusWithContext(ctx); err == nil && len(st) > 0 {
		info.Stat = statusLetter(st[0])
	}

	info.Args, _ = pr.CmdlineWithContext(ctx)

	return info
}

func statusLetter(status string) string {
	switch status {
	case process.Running:
		return "R"
	case process.Sleep:
		return "S"
	case process.Stop:
		return "T"
	case process.Idle:
		return "I"
	case process.Zombie:
		return "Z"
	case process.Blocked:
		return "D"
	case process.Wait:
		return "W"
	case process.Lock:
		return "L"
	default:
		return "?"
	}
}
