// Package probe reads the workspaces of the machine it runs on, without
// writing anything: sessions with their state, process trees, simulators,
// and progress; result directories and campaigns; and host statistics.
// `akita herd probe` prints what it finds as JSON for the monitor.
package probe

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// Probe defaults.
const (
	defaultRunMaxAge      = 3 * 24 * time.Hour
	defaultLiveRunCap     = 150
	defaultArchivedRunCap = 100
	defaultMonitorTimeout = time.Second

	bytesPerGiB = 1 << 30
	mibPerGiB   = 1024
)

// Options tunes Probe. Zero values mean the defaults.
type Options struct {
	// maxAge: result directories older than this are not listed.
	maxAge time.Duration
	// liveRunCap and archivedRunCap bound the number of listed runs
	// (campaigns are always listed).
	liveRunCap     int
	archivedRunCap int
	// monitorTimeout bounds each request to an Akita monitor.
	monitorTimeout time.Duration
	// now is the probe time; zero means time.Now().
	now time.Time
	// Projects, when not empty, are the only workspaces under the root
	// that are probed.
	Projects []string

	sys system // nil: the real machine
}

func (o *Options) fill() {
	if o.maxAge <= 0 {
		o.maxAge = defaultRunMaxAge
	}

	if o.liveRunCap <= 0 {
		o.liveRunCap = defaultLiveRunCap
	}

	if o.archivedRunCap <= 0 {
		o.archivedRunCap = defaultArchivedRunCap
	}

	if o.monitorTimeout <= 0 {
		o.monitorTimeout = defaultMonitorTimeout
	}

	if o.now.IsZero() {
		o.now = time.Now()
	}

	if o.sys == nil {
		o.sys = &realSystem{monitorTimeout: o.monitorTimeout}
	}
}

// Probe scans the workspaces <root>/<project>/ on this machine: sessions
// from logs/, runs from results/, project configs, and host statistics.
// It only reads; nothing under root is written. A leading "~" in root is
// expanded.
func Probe(ctx context.Context, root string, opts Options) (*herd.HostProbe, error) {
	opts.fill()
	root = herd.ExpandHome(root)

	if _, err := os.Stat(root); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	p := newProber(ctx, opts)
	for _, name := range projectNames(root, opts.Projects) {
		p.workspaces = append(p.workspaces, wsRef{project: name, dir: filepath.Join(root, name)})
	}

	p.projects = p.loadProjects()
	out := &herd.HostProbe{Schema: herd.SchemaProbe, Now: p.now, Projects: p.projects}
	p.hostStats(out)

	out.Sessions, _ = p.sessions() // a probe shows what it can
	out.Runs = p.runs()
	p.attachCampaignBars(out.Sessions, out.Runs)

	if q := queue.Open(root); q.Exists() {
		out.Queue = q.Status(opts.now)
	}

	return out, nil
}

// wsRef is a workspace and the name of its project.
type wsRef struct {
	project string
	dir     string
}

// prober holds the state of one probe.
type prober struct {
	ctx        context.Context //nolint:containedctx // per-call state of one probe
	opts       Options
	now        float64
	workspaces []wsRef
	projects   map[string]*herd.ProjectConfig
	procs      *hostsys.ProcTable
	archived   map[string]map[string]bool // project -> archived result directories

	// light lists sessions only: no process trees, monitors, or heartbeat;
	// tailLines lines of every log, with the final SLURM state of a died
	// job; archived sessions only with withArchived.
	light        bool
	tailLines    int
	withArchived bool
}

func newProber(ctx context.Context, opts Options) *prober {
	return &prober{
		ctx: ctx, opts: opts, now: herd.UnixSeconds(opts.now),
		archived: map[string]map[string]bool{}, withArchived: true,
	}
}

// projectNames are the directories under root that are not hidden, or
// only those of projects.
func projectNames(root string, projects []string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var names []string

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") ||
			(len(projects) > 0 && !slices.Contains(projects, e.Name())) {
			continue
		}

		if hostsys.IsDir(filepath.Join(root, e.Name())) {
			names = append(names, e.Name())
		}
	}

	sort.Strings(names)

	return names
}

func (p *prober) loadProjects() map[string]*herd.ProjectConfig {
	projects := map[string]*herd.ProjectConfig{}

	for _, w := range p.workspaces {
		cfg, err := herd.LoadProjectConfig(w.dir)
		if err != nil {
			cfg = nil
		}

		projects[w.project] = cfg
	}

	return projects
}

func (p *prober) hostStats(out *herd.HostProbe) {
	s := p.opts.sys.hostStats(p.ctx)
	out.CPUs, out.Load, out.GPUs = s.CPUs, s.Load, []herd.GPU{}
	out.MemTotalGB = round1(float64(s.MemTotal) / bytesPerGiB)
	out.MemAvailGB = round1(float64(s.MemAvail) / bytesPerGiB)

	for _, g := range s.GPUs {
		out.GPUs = append(out.GPUs, herd.GPU{
			Name: g.Name, Util: g.Util,
			MemUsedGB: round1(g.MemUsedMiB / mibPerGiB), MemTotalGB: round1(g.MemTotalMiB / mibPerGiB),
		})
	}
}

// processes returns the process table, read once per probe.
func (p *prober) processes() *hostsys.ProcTable {
	if p.procs == nil {
		p.procs = p.opts.sys.processes(p.ctx)
	}

	return p.procs
}

// isArchived tells whether a workspace's .herd_archive.json marks a result
// directory as archived.
func (p *prober) isArchived(w wsRef, run string) bool {
	m, ok := p.archived[w.project]
	if !ok {
		m = workspace.LoadArchivedResults(w.dir)
		p.archived[w.project] = m
	}

	return m[run]
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
