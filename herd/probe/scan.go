package probe

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// Scan limits: directories listed per directory, and in all.
const (
	scanDirEntries = 200
	scanDirsTotal  = 5000
	scanLogsRead   = 3
	logStartMarker = "[start] "
)

// Scan looks at this machine for akita herd host scan: the machine, the
// roots that hold workspaces of Herd or of the former exp tool (roots,
// ~/exp, and the directories one and two levels below the home
// directory), and the user's simulations that no Herd session started.
// It only reads.
func Scan(ctx context.Context, roots []string) *herd.HostScan {
	return scan(ctx, roots, Options{})
}

func scan(ctx context.Context, roots []string, opts Options) *herd.HostScan {
	opts.fill()

	out := &herd.HostScan{Schema: herd.SchemaScan, OS: runtime.GOOS, Arch: runtime.GOARCH}
	out.Hostname, _ = os.Hostname()

	if u, err := user.Current(); err == nil {
		out.User = u.Username
	}

	st := opts.sys.hostStats(ctx)
	out.CPUs, out.Load = st.CPUs, st.Load
	out.MemTotalGB = round1(float64(st.MemTotal) / bytesPerGiB)
	out.MemAvailGB = round1(float64(st.MemAvail) / bytesPerGiB)

	_, err := exec.LookPath("tmux")
	out.Tmux = err == nil

	home, _ := os.UserHomeDir()
	for _, r := range findRoots(home, roots) {
		out.Roots = append(out.Roots, scanRoot(ctx, r, home))
	}

	p := newProber(ctx, opts)
	out.Unmanaged = p.unmanaged()

	table, uid := p.processes(), opts.sys.uid()
	for _, pid := range table.PIDs() {
		if info := table.Info(pid); info != nil && cpuPercent(info) >= unmanagedMinCPU &&
			info.Etime >= unmanagedMinAge && opts.sys.procOwner(pid) != uid {
			out.OthersBusy++
		}
	}

	return out
}

// findRoots returns the directories that hold workspaces: those given
// (when they exist), ~/exp, and those one and two levels below home.
func findRoots(home string, given []string) []string {
	seen := map[string]bool{}

	var roots []string

	add := func(dir string) {
		if dir = filepath.Clean(herd.ExpandHome(dir)); !seen[dir] {
			seen[dir] = true

			if isRoot(dir) {
				roots = append(roots, dir)
			}
		}
	}

	for _, r := range given {
		add(r)
	}

	add(herd.DefaultRoot)

	if home == "" {
		return roots
	}

	budget := scanDirsTotal

	for _, d := range subdirs(home, &budget) {
		add(d)

		for _, dd := range subdirs(d, &budget) {
			add(dd)
		}
	}

	return roots
}

// subdirs are the directories in dir that are not hidden, at most
// scanDirEntries, within the budget.
func subdirs(dir string, budget *int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []string

	for _, e := range entries {
		if *budget <= 0 || len(out) >= scanDirEntries {
			break
		}

		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			*budget--

			out = append(out, filepath.Join(dir, e.Name()))
		}
	}

	return out
}

// isRoot tells whether a directory holds a queue or a workspace.
func isRoot(dir string) bool {
	if hostsys.IsDir(filepath.Join(dir, queue.Dir)) {
		return true
	}

	budget := scanDirEntries
	for _, w := range subdirs(dir, &budget) {
		if isWorkspace(w) {
			return true
		}
	}

	return false
}

// isWorkspace tells whether a directory is a workspace: it has archive
// records, or a log that starts with a session's start marker.
func isWorkspace(dir string) bool {
	if hostsys.Exists(filepath.Join(dir, workspace.ArchiveFile)) ||
		hostsys.Exists(filepath.Join(dir, workspace.LegacyArchiveFile)) {
		return true
	}

	logs, _ := filepath.Glob(filepath.Join(workspace.LogsPath(dir), "*.log"))
	for i, l := range logs {
		if i >= scanLogsRead {
			break
		}

		if head, err := hostsys.ReadLimited(l, int64(len(logStartMarker))); err == nil &&
			bytes.Equal(head, []byte(logStartMarker)) {
			return true
		}
	}

	return false
}

// scanRoot counts the sessions and results of each workspace of a root.
func scanRoot(ctx context.Context, root, home string) herd.ScanRoot {
	out := herd.ScanRoot{Root: homeRelative(root, home), Queue: hostsys.IsDir(filepath.Join(root, queue.Dir))}

	budget := scanDirEntries
	for _, w := range subdirs(root, &budget) {
		if !isWorkspace(w) {
			continue
		}

		p := herd.ScanProject{Name: filepath.Base(w)}

		sessCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		sessions, _ := ListSessions(sessCtx, w, p.Name, false, 0)

		cancel()

		for _, s := range sessions {
			p.Sessions++

			if s.State == "running" || s.State == "queued" {
				p.Running++
			}
		}

		if entries, err := os.ReadDir(filepath.Join(w, workspace.ResultsDir)); err == nil {
			p.Results = len(entries)
		}

		out.Projects = append(out.Projects, p)
	}

	return out
}
