package probe

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// What counts as an unmanaged simulation: a process of the user that has
// run for unmanagedMinAge seconds and used unmanagedMinCPU percent of a
// CPU on average, and is not one of the tools below.
const (
	unmanagedMinAge = 60
	unmanagedMinCPU = 50
	maxUnmanaged    = 50
	launcherDepth   = 6
)

// notSimulations are executables that are never simulations.
var notSimulations = []string{
	"bash", "sh", "zsh", "dash", "fish", "tmux", "screen", "sshd", "ssh", "sftp-server",
	"rsync", "vim", "nvim", "emacs", "less", "top", "htop", "systemd", "dbus-daemon", "gpg-agent",
}

// toolMarkers are parts of the command lines of tools that are busy but
// not simulations: editors' remote servers and akita herd itself.
var toolMarkers = []string{
	".vscode-server", ".cursor-server", "herd agent run", " herd web", " herd probe", " herd _local",
	" exp web", "exp_web.py",
}

// wrappers are commands that only start another: the launcher of a
// simulation is looked for above them.
var wrappers = []string{"time", "nohup", "timeout", "setsid", "env", "stdbuf", "nice", "ionice"}

// unmanaged lists the user's busy processes that no Herd session started:
// not in a running session found by this probe, not under a tmux session
// of Herd or of the former exp tool, and (on Linux) without a Herd
// session's environment.
func (p *prober) unmanaged() []herd.Unmanaged {
	if p.light {
		return nil
	}

	sys := p.opts.sys
	table := p.processes()
	managed := p.managedPIDs(table)
	uid := sys.uid()
	home, _ := os.UserHomeDir()

	var out []herd.Unmanaged

	for _, pid := range table.PIDs() {
		info := table.Info(pid)
		if managed[pid] || info == nil || info.Etime < unmanagedMinAge || cpuPercent(info) < unmanagedMinCPU {
			continue
		}

		exe := exeName(info.Args)
		if slices.Contains(notSimulations, exe) || hasToolMarker(info.Args) ||
			sys.procOwner(pid) != uid || sys.procStartedByHerd(pid) {
			continue
		}

		out = append(out, herd.Unmanaged{
			PID: pid, Exe: exe, Args: truncate(homeArgs(info.Args, home), maxProcArgs),
			Cwd:   homeRelative(sys.procCwd(pid), home),
			Start: math.Round(p.now - info.Etime), CPU: math.Round(cpuPercent(info)),
			RSSGB:    math.Round(info.RSSKB/kbPerMB/mibPerGiB*10) / 10,
			Launcher: homeArgs(launcher(table, pid), home),
		})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].CPU > out[j].CPU })

	if len(out) > maxUnmanaged {
		out = out[:maxUnmanaged]
	}

	return out
}

// managedPIDs are the processes of the sessions this probe found and of
// every tmux session of Herd or of the former exp tool (sessions of other
// projects and roots, and agents).
func (p *prober) managedPIDs(table *hostsys.ProcTable) map[int]bool {
	managed := map[int]bool{}
	for pid := range p.sessionPIDs {
		managed[pid] = true
	}

	panes, _ := p.opts.sys.tmuxPanes(p.ctx)
	for name, pane := range panes {
		if workspace.IsHerdTmuxSession(name) {
			for _, d := range table.Descendants(pane) {
				managed[d] = true
			}
		}
	}

	return managed
}

// exeName is the base name of a command line's executable.
func exeName(args string) string {
	f := strings.Fields(args)
	if len(f) == 0 {
		return ""
	}

	return filepath.Base(f[0])
}

func hasToolMarker(args string) bool {
	for _, m := range toolMarkers {
		if strings.Contains(args, m) {
			return true
		}
	}

	return false
}

// launcher is the command line of the script above a process that
// started it (e.g. "bash run.sh <name>"), skipping wrappers such as time
// and nohup; "" when the process was started from an interactive shell.
func launcher(table *hostsys.ProcTable, pid int) string {
	for q, i := table.Parent(pid), 0; q > 1 && i < launcherDepth; q, i = table.Parent(q), i+1 {
		info := table.Info(q)
		if info == nil {
			return ""
		}

		f := strings.Fields(info.Args)
		exe := exeName(info.Args)

		switch {
		case slices.Contains(wrappers, exe):
			continue
		case len(f) < 2 || strings.HasPrefix(f[0], "-") || exe == "tmux" || exe == "sshd" || exe == "systemd":
			return "" // an interactive shell, tmux, or the system
		default:
			return truncate(info.Args, maxProcArgs)
		}
	}

	return ""
}

// homeArgs writes the paths under home in a command line as "~/...".
func homeArgs(args, home string) string {
	if home == "" {
		return args
	}

	return strings.ReplaceAll(args, home+"/", "~/")
}

// homeRelative writes a path under home as "~/...".
func homeRelative(p, home string) string {
	switch {
	case p == "" || home == "":
		return p
	case p == home:
		return "~"
	case strings.HasPrefix(p, home+"/"):
		return "~/" + p[len(home)+1:]
	default:
		return p
	}
}
