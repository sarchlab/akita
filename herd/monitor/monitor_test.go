package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/sarchlab/akita/v5/herd/internal/testutil"
	"github.com/sarchlab/akita/v5/herd/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBackend answers Akita calls with a function; other Backend methods
// are not used by the monitor.
type fakeBackend struct {
	backend.Backend

	host herd.HostConfig

	mu       sync.Mutex
	calls    [][]string
	akita    func(args []string) ([]byte, error)
	akitaCtx func(ctx context.Context, args []string) ([]byte, error) // used when set
}

func (b *fakeBackend) Host() herd.HostConfig { return b.host }

func (b *fakeBackend) Workspace(project string) string { return "~/exp/" + project }

func (b *fakeBackend) Akita(ctx context.Context, args ...string) ([]byte, error) {
	b.mu.Lock()
	b.calls = append(b.calls, args)
	b.mu.Unlock()

	if b.akitaCtx != nil {
		return b.akitaCtx(ctx, args)
	}

	return b.akita(args)
}

func (b *fakeBackend) callCount(sub string) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := 0

	for _, c := range b.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			n++
		}
	}

	return n
}

func probeJSON(t *testing.T, p *herd.HostProbe) []byte {
	t.Helper()

	data, err := json.Marshal(p)
	require.NoError(t, err)

	return data
}

func runningProbe(now float64, cpuTime float64, hb map[string]any) *herd.HostProbe {
	return &herd.HostProbe{
		Schema: herd.SchemaProbe, Now: now,
		Projects: map[string]*herd.ProjectConfig{"proj": nil},
		Sessions: []herd.Session{{
			ID: "proj/s", Project: "proj", Name: "s", State: "running", LastOutput: now,
			Procs:     []herd.Process{{PID: 9, CPUTime: cpuTime}},
			Heartbeat: hb,
		}},
		Runs: []herd.Run{{ID: "proj/r", Project: "proj", Name: "r", Kind: "single"}},
	}
}

const (
	logStart = "[start] Thu Oct 15 02:59:54 PM UTC 2026  host  ./run\n"
	logExit0 = "[exit 0] Thu Oct 15 03:00:00 PM UTC 2026\n"
)

// testConfig is a user config with the default rules.
func testConfig() *herd.UserConfig {
	r := testRules()

	return &herd.UserConfig{
		StallMinutes: r.StallMinutes, IdleCPU: r.IdleCPU, SimStallMinutes: r.SimStallMinutes,
		WorkStallMinutes: r.WorkStallMinutes, Interval: 10,
	}
}

func testRules() herd.Rules {
	return herd.Rules{StallMinutes: 30, IdleCPU: 5, SimStallMinutes: 10, WorkStallMinutes: 30}
}

func TestMonitorMergesHosts(t *testing.T) {
	cfg := testConfig()
	good := &fakeBackend{host: herd.HostConfig{Name: "a", Type: "ssh"}, akita: func(args []string) ([]byte, error) {
		assert.Equal(t, probe.Args("~/exp"), args)
		return probeJSON(t, runningProbe(1000, 10, nil)), nil
	}}
	bad := &fakeBackend{host: herd.HostConfig{Name: "b", Type: "ssh"}, akita: func([]string) ([]byte, error) {
		return nil, errors.New("ssh: connect to host b: timed out")
	}}

	m := newWithBackends(cfg, []backend.Backend{good, bad})
	assert.False(t, m.Status().Hosts[0].OK)

	m.pollOnce(context.Background())
	st := m.Status()

	assert.Equal(t, herd.SchemaStatus, st.Schema)
	require.Len(t, st.Hosts, 2)
	assert.True(t, st.Hosts[0].OK)
	assert.False(t, st.Hosts[1].OK)
	assert.Contains(t, st.Hosts[1].Error, "timed out")
	require.Len(t, st.Sessions, 1)
	assert.Equal(t, "a/proj/s", st.Sessions[0].ID)
	assert.Equal(t, "a", st.Sessions[0].Host)
	assert.Equal(t, "a/proj/r", st.Runs[0].ID)
	assert.Contains(t, st.Projects, "proj")
	assert.Equal(t, "cpu", st.Sessions[0].Health.HangBasis)
}

func TestHealthCPURule(t *testing.T) {
	j := newHealthJudge(testRules())

	judge := func(now, cpu float64) herd.Session {
		p := runningProbe(now, cpu, nil)
		j.judgeHost("h", p)

		return p.Sessions[0]
	}

	s := judge(0, 100)
	assert.Nil(t, s.Health.CPUNow)

	s = judge(60, 160) // 100 % busy
	assert.InDelta(t, 100, *s.Health.CPUNow, 1e-9)
	assert.Nil(t, s.Health.LowFor)

	s = judge(120, 160) // idle since t=60
	assert.InDelta(t, 0, *s.Health.CPUNow, 1e-9)
	assert.InDelta(t, 60, *s.Health.LowFor, 1e-9)
	assert.Equal(t, "running", s.State)

	s = judge(60+30*60, 160)
	assert.Equal(t, "stalled", s.State)
	assert.Contains(t, s.Health.HangReason, "CPU")
	assert.False(t, s.Health.OutputIdle)

	s = judge(60+31*60, 200) // busy again
	assert.Equal(t, "running", s.State)
	assert.Nil(t, s.Health.LowFor)
}

func TestHealthOutputIdle(t *testing.T) {
	j := newHealthJudge(testRules())
	p := runningProbe(10000, 10, nil)
	p.Sessions[0].LastOutput = 10000 - 31*60
	j.judgeHost("h", p)
	assert.True(t, p.Sessions[0].Health.OutputIdle)
	assert.Equal(t, "running", p.Sessions[0].State)
}

func TestHealthHeartbeatRule(t *testing.T) {
	j := newHealthJudge(testRules())

	judge := func(now, sim, work, wall float64) herd.Session {
		hb := map[string]any{
			"pid": 9.0, "wall": wall, "sim_time_ns": sim, "wgs_completed": work,
			"finished": false, "args": []any{"-max-wg=1000"},
		}
		p := runningProbe(now, 0, hb) // CPU idle: the heartbeat overrides
		j.judgeHost("h", p)

		return p.Sessions[0]
	}

	s := judge(0, 1, 0, 0)
	assert.Equal(t, "heartbeat", s.Health.HangBasis)
	assert.Nil(t, s.Health.SimRate)

	s = judge(60, 2, 100, 60)
	assert.InDelta(t, 1.0/60, *s.Health.SimRate, 1e-9)
	assert.InDelta(t, 100.0/60, *s.Health.WGRate, 1e-9)
	assert.InDelta(t, 900/(100.0/60), *s.Health.ETA, 1e-6)
	assert.Equal(t, "running", s.State)

	// Simulated time stops advancing for SimStallMinutes.
	s = judge(60+10*60, 2, 100, 60+10*60)
	assert.Equal(t, "stalled", s.State)
	assert.Contains(t, s.Health.HangReason, "simulated time")

	// The heartbeat itself stops.
	j2 := newHealthJudge(testRules())
	p := runningProbe(2000, 0, map[string]any{"pid": 9.0, "wall": 2000.0 - 11*60, "sim_time_ns": 1.0})
	j2.judgeHost("h", p)
	assert.Equal(t, "stalled", p.Sessions[0].State)
	assert.Contains(t, p.Sessions[0].Health.HangReason, "heartbeat")

	// A heartbeat of another process falls back to the CPU rule.
	j3 := newHealthJudge(testRules())
	p = runningProbe(0, 0, map[string]any{"pid": 12345.0, "wall": 0.0})
	j3.judgeHost("h", p)
	assert.Equal(t, "cpu", p.Sessions[0].Health.HangBasis)
}

func TestHealthWorkStall(t *testing.T) {
	j := newHealthJudge(testRules())

	judge := func(now, sim float64) herd.Session {
		p := runningProbe(now, 0, map[string]any{"pid": 9.0, "wall": now, "sim_time_ns": sim, "wgs_completed": 5.0})
		j.judgeHost("h", p)

		return p.Sessions[0]
	}

	for i := 0; i <= 30; i++ {
		s := judge(float64(i*60), float64(i))
		if i < 30 {
			assert.Equal(t, "running", s.State, "minute %d", i)
		} else {
			assert.Equal(t, "stalled", s.State)
			assert.Contains(t, s.Health.HangReason, "completed work")
		}
	}
}

func TestHealthMonitorRule(t *testing.T) {
	j := newHealthJudge(testRules())

	judge := func(now, cpu float64, finished uint64) herd.Session {
		p := runningProbe(now, cpu, nil)
		p.Sessions[0].Progress = []herd.Progress{{ID: "akita:1:1", Finished: finished}}
		j.judgeHost("h", p)

		return p.Sessions[0]
	}

	cpu := 0.0
	for i := 0; i <= 30; i++ {
		cpu += 60 // busy
		s := judge(float64(i*60), cpu, 3)
		assert.Equal(t, "monitor", s.Health.HangBasis)

		if i == 30 {
			assert.Equal(t, "stalled", s.State)
			assert.Contains(t, s.Health.HangReason, "monitor progress")
		}
	}

	s := judge(31*60, cpu+60, 4)
	assert.Equal(t, "running", s.State)
}

func TestHealthForgetsEndedSessions(t *testing.T) {
	j := newHealthJudge(testRules())
	j.judgeHost("h", runningProbe(0, 0, nil))
	assert.Len(t, j.cpu, 1)

	p := runningProbe(10, 0, nil)
	p.Sessions[0].State = "ok"
	j.judgeHost("h", p)
	assert.Empty(t, j.cpu)
}

func TestMonitorDetailCache(t *testing.T) {
	cfg := testConfig()
	finished := false
	b := &fakeBackend{host: herd.HostConfig{Name: "a", Type: "ssh"}, akita: func(args []string) ([]byte, error) {
		switch args[2] {
		case "run":
			return json.Marshal(&herd.RunDetail{Project: args[5], Name: args[6], Finished: finished})
		case "campaign":
			return []byte(`{"cells":[{"key":{"w":"a"},"status":"ok"}],"total":1}`), nil
		}

		return nil, errors.New("unexpected")
	}}

	m := newWithBackends(cfg, []backend.Backend{b})
	clock := time.Unix(1000, 0)
	m.now = func() time.Time { return clock }

	ctx := context.Background()
	d, err := m.RunDetail(ctx, "a", "proj", "r", false)
	require.NoError(t, err)
	assert.Equal(t, "a", d.Host)
	assert.Equal(t, probe.RunArgs("~/exp", "proj", "r"), b.calls[0])

	_, _ = m.RunDetail(ctx, "a", "proj", "r", false)
	assert.Equal(t, 1, b.callCount("run"), "cached for the TTL")

	clock = clock.Add(cacheTTL + time.Second)
	finished = true
	_, _ = m.RunDetail(ctx, "a", "proj", "r", false)
	assert.Equal(t, 2, b.callCount("run"), "stale after the TTL")

	clock = clock.Add(time.Hour)
	_, _ = m.RunDetail(ctx, "a", "proj", "r", false)
	assert.Equal(t, 2, b.callCount("run"), "finished runs stay cached")

	_, _ = m.RunDetail(ctx, "a", "proj", "r", true)
	assert.Equal(t, 3, b.callCount("run"), "refresh re-fetches")

	_, _ = m.RunDetail(ctx, "a", "proj", "r", true)
	assert.Equal(t, 3, b.callCount("run"), "but not more often than cacheRefreshMin")

	c, err := m.Campaign(ctx, "a", "proj", "camp", false)
	require.NoError(t, err)
	assert.True(t, c.Finished)
	assert.Equal(t, "camp", c.Name)
	assert.Equal(t, herd.SchemaCells, c.Schema)

	_, err = m.RunDetail(ctx, "nohost", "proj", "r", false)

	var uh *unknownHostError
	require.ErrorAs(t, err, &uh)
}

func TestLocalMonitorProbesOnlyItsProject(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, "akxscope")
	testutil.WriteFile(t, filepath.Join(proj, "logs", "a.log"), logStart+logExit0)
	testutil.WriteFile(t, filepath.Join(base, "sibling", "logs", "x.log"), logStart+logExit0)
	testutil.WriteFile(t, filepath.Join(base, "sibling", "results", "r", "m"), "")

	t.Chdir(proj)
	cwd, _ := os.Getwd()

	b, err := backend.New(herd.HostConfig{Name: "local", Type: "local"})
	require.NoError(t, err)

	// A later change of directory does not move the workspace.
	t.Chdir(base)
	assert.Equal(t, cwd, b.Workspace("akxscope"))

	m := newWithBackends(testConfig(), []backend.Backend{b})
	h := m.hosts[0]
	assert.Equal(t, filepath.Dir(cwd), h.root)
	assert.Equal(t, []string{"akxscope"}, h.projects)
	assert.Equal(t, []string{"herd", "probe", "--json", "--root", filepath.Dir(cwd), "--project", "akxscope"},
		probe.Args(h.root, h.projects...))

	p, err := probe.Probe(context.Background(), h.root, probe.Options{Projects: h.projects})
	require.NoError(t, err)
	assert.Equal(t, []string{"akxscope"}, slices.Collect(maps.Keys(p.Projects)))
	require.Len(t, p.Sessions, 1)
	assert.Equal(t, "akxscope", p.Sessions[0].Project)
	assert.Empty(t, p.Runs)

	_, err = m.RunDetail(context.Background(), "local", "sibling", "r", false)
	require.ErrorContains(t, err, "unknown project")
}

func TestMonitorDetailFetchIsDetached(t *testing.T) {
	started, release := make(chan struct{}, 1), make(chan struct{})
	fetchErr := make(chan error, 1)
	b := &fakeBackend{host: herd.HostConfig{Name: "a", Type: "ssh"},
		akitaCtx: func(ctx context.Context, args []string) ([]byte, error) {
			started <- struct{}{}
			<-release
			fetchErr <- ctx.Err()

			return json.Marshal(&herd.RunDetail{Project: args[5], Name: args[6]})
		}}
	m := newWithBackends(testConfig(), []backend.Backend{b})

	// The first client goes away while the fetch runs.
	ctx1, cancel1 := context.WithCancel(context.Background())
	first := make(chan error, 1)

	go func() {
		_, err := m.RunDetail(ctx1, "a", "proj", "r", false)
		first <- err
	}()

	<-started
	cancel1()
	require.ErrorIs(t, <-first, context.Canceled)

	// A second client joins the same fetch, which was not cancelled.
	second := make(chan *herd.RunDetail, 1)

	go func() {
		d, err := m.RunDetail(context.Background(), "a", "proj", "r", false)
		assert.NoError(t, err)
		second <- d
	}()

	close(release)
	require.NoError(t, <-fetchErr, "the fetch must not use the first client's context")
	assert.Equal(t, "r", (<-second).Name)
	assert.Equal(t, 1, b.callCount("run"))
}

func TestMonitorDetailCacheIsBounded(t *testing.T) {
	b := &fakeBackend{host: herd.HostConfig{Name: "a", Type: "ssh"}, akita: func(args []string) ([]byte, error) {
		return json.Marshal(&herd.RunDetail{Project: args[5], Name: args[6], Finished: true})
	}}
	m := newWithBackends(testConfig(), []backend.Backend{b})
	ctx := context.Background()

	for i := range maxCacheEntries + 50 {
		_, err := m.RunDetail(ctx, "a", "proj", fmt.Sprintf("r%d", i), false)
		require.NoError(t, err)
	}

	m.cache.mu.Lock()
	defer m.cache.mu.Unlock()

	assert.Len(t, m.cache.entries, maxCacheEntries)
	assert.Empty(t, m.cache.inflight)
}

func TestMonitorKeepsHistoryAcrossFailedPolls(t *testing.T) {
	now, fail := 0.0, false
	b := &fakeBackend{host: herd.HostConfig{Name: "a", Type: "ssh"}, akita: func([]string) ([]byte, error) {
		if fail {
			return nil, errors.New("ssh: connection reset")
		}

		return probeJSON(t, runningProbe(now, 100, nil)), nil // idle: CPU time constant
	}}
	m := newWithBackends(testConfig(), []backend.Backend{b})
	ctx := context.Background()

	m.pollOnce(ctx) // t=0: first sample
	now = 60
	m.pollOnce(ctx) // t=60: idle since 0

	fail = true
	m.pollOnce(ctx) // a transient failure
	assert.Len(t, m.judge.cpu, 1, "history kept across a failed poll")

	fail, now = false, 31*60
	m.pollOnce(ctx)

	s := m.Status().Sessions[0]
	require.NotNil(t, s.Health.LowFor)
	assert.InDelta(t, 31*60, *s.Health.LowFor, 1e-9, "idle time counted from before the failure")
	assert.Equal(t, "stalled", s.State)
}

func TestMonitorQueueAndActivity(t *testing.T) {
	q := &herd.QueueStatus{Root: "~/exp", Pending: []herd.Task{{Project: "proj", Name: "w"}}}
	b := &fakeBackend{host: herd.HostConfig{Name: "a", Type: "ssh"}, akita: func([]string) ([]byte, error) {
		p := runningProbe(1000, 10, nil)
		p.Queue = q

		return probeJSON(t, p), nil
	}}

	m := newWithBackends(testConfig(), []backend.Backend{b})
	m.pollOnce(context.Background())

	for i := range maxActivity + 3 {
		m.AddActivity(fmt.Sprintf("line %d", i))
	}

	st := m.Status()
	require.NotNil(t, st.Hosts[0].Queue)
	assert.Equal(t, "w", st.Hosts[0].Queue.Pending[0].Name)
	require.Len(t, st.Activity, maxActivity)
	assert.Equal(t, fmt.Sprintf("line %d", maxActivity+2), st.Activity[maxActivity-1])
}
