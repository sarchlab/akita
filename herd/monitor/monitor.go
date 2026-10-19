// Package monitor polls hosts with `akita herd probe`, judges whether
// running sessions hang, and serves the web UI and /api/v1.
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/sarchlab/akita/v5/herd/probe"
)

// Monitor timings and limits.
const (
	probeTimeout       = 60 * time.Second
	detailTimeout      = 2 * time.Minute
	campaignTimeout    = 6 * time.Minute
	maxProbeErrorBytes = 300
	maxActivity        = 50
)

// CampaignResponse is a campaign as /api/v1/campaign serves it.
type CampaignResponse struct {
	herd.CampaignCells

	Host      string  `json:"host"`
	Project   string  `json:"project"`
	Name      string  `json:"name"`
	FetchedAt float64 `json:"fetched_at"`
	Finished  bool    `json:"finished"`
}

// monitorHost is one polled host.
type monitorHost struct {
	backend  backend.Backend
	root     string
	projects []string // the only projects probed; all when empty
}

// hostResult is the latest poll of a host.
type hostResult struct {
	probe *herd.HostProbe
	err   error
	fetch time.Duration
}

// Monitor polls hosts with `akita herd probe` and keeps a Status.
type Monitor struct {
	cfg   *herd.UserConfig
	hosts []monitorHost
	now   func() time.Time

	mu       sync.RWMutex
	status   *herd.Status
	results  map[string]hostResult
	judge    *healthJudge
	activity []string

	cache *detailCache
}

// New creates a monitor of the named hosts of cfg (all hosts when names is
// empty).
func New(cfg *herd.UserConfig, names []string) (*Monitor, error) {
	var backends []backend.Backend

	for _, h := range cfg.Hosts {
		if len(names) > 0 && !slices.Contains(names, h.Name) {
			continue
		}

		b, err := backend.New(h)
		if err != nil {
			return nil, err
		}

		backends = append(backends, b)
	}

	for _, n := range names {
		if _, ok := cfg.Host(n); !ok {
			return nil, fmt.Errorf("unknown host %q", n)
		}
	}

	return newWithBackends(cfg, backends), nil
}

// newWithBackends creates a monitor of the given backends.
func newWithBackends(cfg *herd.UserConfig, backends []backend.Backend) *Monitor {
	m := &Monitor{
		cfg:     cfg,
		now:     time.Now,
		results: map[string]hostResult{},
		judge:   newHealthJudge(cfg.Rules()),
	}
	m.cache = newDetailCache(func() time.Time { return m.now() })

	for _, b := range backends {
		root, projects := backend.ProbeScope(b)
		m.hosts = append(m.hosts, monitorHost{backend: b, root: root, projects: projects})
	}

	m.status = m.build()

	return m
}

// Run polls every Interval seconds until ctx is done.
func (m *Monitor) Run(ctx context.Context) {
	interval := time.Duration(max(m.cfg.Interval, 1)) * time.Second

	for {
		m.pollOnce(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// pollOnce probes all hosts concurrently and updates the status.
func (m *Monitor) pollOnce(ctx context.Context) {
	results := make([]hostResult, len(m.hosts))

	var wg sync.WaitGroup

	for i, h := range m.hosts {
		wg.Go(func() { results[i] = probeHost(ctx, h) })
	}

	wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, h := range m.hosts {
		name := h.backend.Host().Name
		r := results[i]

		// A failed poll (e.g. a transient ssh error) says nothing about the
		// sessions: their hang history is kept. Only a successful probe
		// forgets the sessions it no longer lists.
		if r.err == nil {
			m.judge.judgeHost(name, r.probe)
		}

		m.results[name] = r
	}

	m.status = m.build()
}

func probeHost(ctx context.Context, h monitorHost) hostResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	start := time.Now()
	out, err := h.backend.Akita(ctx, probe.Args(h.root, h.projects...)...)

	r := hostResult{fetch: time.Since(start)}
	if err != nil {
		r.err = err
		return r
	}

	p := &herd.HostProbe{}
	if err := json.Unmarshal(out, p); err != nil {
		r.err = fmt.Errorf("bad probe output: %w", err)
		return r
	}

	p.Schema = herd.NormalizeSchema(p.Schema)
	r.probe = p

	return r
}

// AddActivity records a line of what the balance loop did; the status
// keeps the last 50.
func (m *Monitor) AddActivity(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activity = append(m.activity, line)
	if len(m.activity) > maxActivity {
		m.activity = m.activity[len(m.activity)-maxActivity:]
	}

	m.status = m.build()
}

// Status returns the latest status. It must not be modified.
func (m *Monitor) Status() *herd.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.status
}

// build assembles the status from the latest host results.
func (m *Monitor) build() *herd.Status {
	st := &herd.Status{
		Schema:   herd.SchemaStatus,
		Updated:  herd.UnixSeconds(m.now()),
		Interval: m.cfg.Interval,
		Rules:    m.cfg.Rules(),
		Projects: map[string]*herd.ProjectConfig{},
		Hosts:    []herd.HostStatus{},
		Sessions: []herd.Session{},
		Runs:     []herd.Run{},
		Activity: slices.Clone(m.activity),
	}

	for _, h := range m.hosts {
		hc := h.backend.Host()
		hs := herd.HostStatus{Name: hc.Name, Type: hc.Type}

		r, polled := m.results[hc.Name]

		switch {
		case !polled:
			hs.Error = "not polled yet"
		case r.err != nil:
			hs.Error = errorText(r.err)
		default:
			addProbe(st, &hs, r)
		}

		st.Hosts = append(st.Hosts, hs)
	}

	return st
}

func addProbe(st *herd.Status, hs *herd.HostStatus, r hostResult) {
	p := r.probe
	hs.OK = true
	hs.Now, hs.Load, hs.CPUs = p.Now, p.Load, p.CPUs
	hs.MemTotalGB, hs.MemAvailGB, hs.GPUs = p.MemTotalGB, p.MemAvailGB, p.GPUs
	hs.FetchS = math.Round(r.fetch.Seconds()*10) / 10
	hs.Queue = p.Queue

	for name, cfg := range p.Projects {
		if old, ok := st.Projects[name]; !ok || old == nil {
			st.Projects[name] = cfg
		}
	}

	for _, s := range p.Sessions {
		s.Host = hs.Name
		s.ID = hs.Name + "/" + s.Project + "/" + s.Name
		st.Sessions = append(st.Sessions, s)
	}

	for _, run := range p.Runs {
		run.Host = hs.Name
		run.ID = hs.Name + "/" + run.Project + "/" + run.Name
		st.Runs = append(st.Runs, run)
	}
}

func errorText(err error) string {
	s := err.Error()
	if len(s) > maxProbeErrorBytes {
		s = s[len(s)-maxProbeErrorBytes:]
	}

	return s
}

// host returns the polled host of a run or campaign of project.
func (m *Monitor) host(name, project string) (monitorHost, error) {
	for _, h := range m.hosts {
		if h.backend.Host().Name != name {
			continue
		}

		if len(h.projects) > 0 && !slices.Contains(h.projects, project) {
			return monitorHost{}, fmt.Errorf("host %s: unknown project %q", name, project)
		}

		return h, nil
	}

	return monitorHost{}, &unknownHostError{Host: name}
}

// unknownHostError is returned for a host the monitor does not poll.
type unknownHostError struct {
	Host string
}

func (e *unknownHostError) Error() string {
	return fmt.Sprintf("unknown host %q", e.Host)
}

// RunDetail returns the detail of a run, fetched from its host.
func (m *Monitor) RunDetail(ctx context.Context, host, project, name string, refresh bool,
) (*herd.RunDetail, error) {
	h, err := m.host(host, project)
	if err != nil {
		return nil, err
	}

	key := "run\x00" + host + "\x00" + project + "\x00" + name

	e, err := m.cache.get(ctx, key, refresh, detailTimeout, func(ctx context.Context) (any, bool, error) {
		out, err := h.backend.Akita(ctx, probe.RunArgs(h.root, project, name)...)
		if err != nil {
			return nil, false, err
		}

		d := &herd.RunDetail{}
		if err := json.Unmarshal(out, d); err != nil {
			return nil, false, fmt.Errorf("bad run detail: %w", err)
		}

		d.Host = host

		return d, d.Finished, nil
	})
	if err != nil {
		return nil, err
	}

	return e.value.(*herd.RunDetail), nil
}

// Campaign returns the cells of a campaign, fetched from its host.
func (m *Monitor) Campaign(ctx context.Context, host, project, name string, refresh bool,
) (*CampaignResponse, error) {
	h, err := m.host(host, project)
	if err != nil {
		return nil, err
	}

	key := "campaign\x00" + host + "\x00" + project + "\x00" + name

	e, err := m.cache.get(ctx, key, refresh, campaignTimeout, func(ctx context.Context) (any, bool, error) {
		out, err := h.backend.Akita(ctx, probe.CampaignArgs(h.root, project, name)...)
		if err != nil {
			return nil, false, err
		}

		c := &CampaignResponse{}
		if err := json.Unmarshal(out, &c.CampaignCells); err != nil {
			return nil, false, fmt.Errorf("bad campaign cells: %w", err)
		}

		probe.NormalizeCells(&c.CampaignCells)
		c.Host, c.Project, c.Name = host, project, name
		c.FetchedAt = herd.UnixSeconds(m.now())
		c.Finished = probe.CampaignFinished(&c.CampaignCells)

		return c, c.Finished, nil
	})
	if err != nil {
		return nil, err
	}

	return e.value.(*CampaignResponse), nil
}
