package probe

import (
	"math"
	"sort"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// Session reading limits (bytes and lines) of live and archived sessions,
// and of the listed processes.
const (
	liveTailBytes     = 40000
	archivedTailBytes = 8000
	liveTailLines     = 200
	archivedTailLines = 50
	maxCmdBytes       = 4000
	maxProcsListed    = 8
	maxProcArgs       = 400
	kbPerMB           = 1024
)

// logRef is one session found under logs/ or logs/archived/.
type logRef struct {
	ws       wsRef
	entry    workspace.Entry
	dir      string // the directory holding <name>.log and its companions
	archived bool
}

func (l logRef) path(ext string) string {
	return workspace.SessionFile(l.dir, l.entry.Name, ext)
}

// sessions reads the sessions of the workspaces. Without the tmux sessions
// (when tmux cannot be asked) it still lists them, and returns the error.
func (p *prober) sessions() ([]herd.Session, error) {
	var refs []logRef

	for _, w := range p.workspaces {
		logs := workspace.LogsPath(w.dir)
		for _, e := range workspace.ListEntries(logs, true) {
			refs = append(refs, logRef{ws: w, entry: e, dir: logs})
		}

		if !p.withArchived {
			continue
		}

		arch := workspace.ArchivedPath(w.dir)
		for _, e := range workspace.ListEntries(arch, false) {
			refs = append(refs, logRef{ws: w, entry: e, dir: arch, archived: true})
		}
	}

	panes, err := p.opts.sys.tmuxPanes(p.ctx)
	sessions := make([]herd.Session, 0, len(refs))

	for _, r := range refs {
		if s, ok := p.session(r, panes); ok {
			sessions = append(sessions, s)
		}
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].MTime > sessions[j].MTime
	})

	return sessions, err
}

func (p *prober) session(r logRef, panes map[string]int) (herd.Session, bool) {
	tailBytes, tailLines := int64(liveTailBytes), liveTailLines

	switch {
	case p.light:
		tailLines = p.tailLines
	case r.archived:
		tailBytes, tailLines = archivedTailBytes, archivedTailLines
	}

	lc := &workspace.Log{}

	if r.entry.HasLog {
		var err error
		if lc, err = workspace.ReadLog(r.path(workspace.LogExt), workspace.LogHeadBytes, tailBytes); err != nil {
			return herd.Session{}, false
		}
	}

	s := newSession(r, lc, tailLines)

	job := workspace.ReadJob(r.dir, s.Name)
	if job != nil {
		s.Job = job.ID
	}

	if !p.light {
		s.Heartbeat = workspace.ReadHeartbeat(r.dir, s.Name)
	}

	pane, inTmux := workspace.TmuxPane(panes, r.ws.project, s.Name)
	live := workspace.Liveness(p.ctx, p.opts.sys, workspace.LiveInput{
		Finished: s.State != "died",
		Shadowed: r.archived && workspace.Shadowed(r.dir, s.Name),
		Tmux:     inTmux,
		TmuxPID:  pane,
		Job:      job,
	})

	switch {
	case live.State == "":
		if p.light && live.Final != "" {
			s.Tail = append(s.Tail, "[slurm] "+live.Final)
		}
	case live.PID > 0 && !p.light:
		p.markRunning(&s, live.PID, lc)
	default:
		s.State = live.State
	}

	return s, true
}

// newSession is a session as its files and log markers describe it.
func newSession(r logRef, lc *workspace.Log, tailLines int) herd.Session {
	name := r.entry.Name
	st := lc.Markers()

	return herd.Session{
		ID:         r.ws.project + "/" + name,
		Project:    r.ws.project,
		Name:       name,
		Archived:   r.archived,
		State:      st.State,
		ExitCode:   st.ExitCode,
		Start:      st.Start,
		End:        st.End,
		MTime:      lc.MTime,
		LastOutput: lc.MTime,
		LogSize:    lc.Size,
		Cmd:        strings.TrimSpace(hostsys.ReadText(r.path(workspace.CmdExt), maxCmdBytes)),
		Group:      workspace.ReadGroup(r.dir, name),
		Tail:       lc.TailLines(tailLines),
		Progress:   []herd.Progress{},
		Sims:       []herd.Simulator{},
		Procs:      []herd.Process{},
	}
}

// markRunning fills a live session from the process tree under pid.
func (p *prober) markRunning(s *herd.Session, pid int, lc *workspace.Log) {
	s.State = "running"
	table := p.processes()
	cfg := p.projects[s.Project]

	for _, d := range table.Descendants(pid) {
		p.sessionPIDs[d] = true

		info := table.Info(d)
		if info == nil {
			continue
		}

		if sim, ok := simulatorOf(cfg, d, info); ok {
			s.Sims = append(s.Sims, sim)
		}

		if w := p.opts.sys.newestWritten(d); w > s.LastOutput {
			s.LastOutput = w
		}

		if skipProcess(info.Args) {
			continue
		}

		s.Procs = append(s.Procs, herd.Process{
			PID: d, Etime: info.Etime, CPUTime: info.CPUTime,
			CPU: cpuPercent(info), Stat: info.Stat,
			RSSMB: math.Round(info.RSSKB / kbPerMB), Args: truncate(info.Args, maxProcArgs),
		})
	}

	sort.SliceStable(s.Procs, func(i, j int) bool { return s.Procs[i].CPUTime > s.Procs[j].CPUTime })

	if len(s.Procs) > maxProcsListed {
		s.Procs = s.Procs[:maxProcsListed]
	}

	sort.SliceStable(s.Sims, func(i, j int) bool {
		a, b := s.Sims[i], s.Sims[j]
		return a.Campaign+"\x00"+a.Label+"\x00"+a.Config < b.Campaign+"\x00"+b.Label+"\x00"+b.Config
	})

	s.Progress = append(s.Progress, heartbeatBars(workspace.ParseHeartbeat(s.Heartbeat), s.Start)...)
	p.attachMonitors(s, lc)
}

// skipProcess hides the processes of the session machinery.
func skipProcess(args string) bool {
	for _, prefix := range []string{"tmux", "tee", "bash -lc", "sleep"} {
		if strings.HasPrefix(args, prefix) {
			return true
		}
	}

	return false
}

// cpuPercent is a process's average CPU use since it started.
func cpuPercent(info *hostsys.ProcInfo) float64 {
	return 100 * info.CPUTime / max(info.Etime, 1)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return strings.ToValidUTF8(s[:n], "")
}
