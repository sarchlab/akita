package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/internal/testutil"
	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSystem is a scripted machine for the probe.
type fakeSystem struct {
	panes  map[string]int
	procs  map[int]hostsys.ProcInfo
	ppid   map[int]int
	slurm  map[string]string
	final  map[string]string
	http   map[string]string
	writes map[int]float64
}

func (f *fakeSystem) tmuxPanes(context.Context) (map[string]int, error) { return f.panes, nil }

func (f *fakeSystem) processes(context.Context) *hostsys.ProcTable {
	ppid := map[int]int{}
	for pid := range f.procs {
		ppid[pid] = f.ppid[pid]
	}

	return hostsys.NewProcTable(ppid, func(pid int) *hostsys.ProcInfo {
		p := f.procs[pid]
		return &p
	})
}

func (f *fakeSystem) PIDAlive(pid int) bool {
	_, ok := f.procs[pid]
	return ok
}

func (f *fakeSystem) SlurmJob(_ context.Context, id string) (state, final string) {
	return f.slurm[id], f.final[id]
}

func (f *fakeSystem) newestWritten(pid int) float64 { return f.writes[pid] }

func (f *fakeSystem) httpGet(_ context.Context, url string) ([]byte, error) {
	if body, ok := f.http[url]; ok {
		return []byte(body), nil
	}

	return nil, errors.New("connection refused")
}

func (f *fakeSystem) hostStats(context.Context) hostsys.Stats {
	return hostsys.Stats{Load: []float64{1, 2, 3}, CPUs: 8}
}

const testProjectConfig = `{
  "schema": "herd-project/1",
  "naming": {"separator": "_", "fields": [
    {"name": "date", "pattern": "\\d{4}"},
    {"name": "category", "values": {"abl": "ablation"}},
    {"name": "item", "pattern": "[a-z0-9]+"},
    {"name": "config", "pattern": "[a-z0-9]+"},
    {"name": "version", "pattern": "[0-9a-f]{7}"}]},
  "simulators": [{"executable": "sim", "label_flag": "-benchmark", "output_flag": "-metric-file-name"}]
}`

const (
	logStart = "[start] Thu Oct 15 02:59:54 PM UTC 2026  host  ./run\n"
	logExit0 = "[exit 0] Thu Oct 15 03:00:00 PM UTC 2026\n"
)

// testWorkspace builds <root>/proj with sessions and runs of every kind.
func testWorkspace(t *testing.T, now time.Time) string {
	t.Helper()

	root := t.TempDir()
	ws := filepath.Join(root, "proj")
	logs := filepath.Join(ws, "logs")
	res := filepath.Join(ws, "results")

	writeFile(t, filepath.Join(ws, "herd.json"), testProjectConfig)
	writeFile(t, filepath.Join(root, "other", "logs", "x.log"), logStart+logExit0)

	writeFile(t, filepath.Join(logs, "done.log"), logStart+"GOPATH set to GOROOT\nhello\n"+logExit0)
	writeFile(t, filepath.Join(logs, "done.cmd"), "./run\n")
	writeFile(t, filepath.Join(logs, "done.group"), "g1\n")
	writeFile(t, filepath.Join(logs, "fail.log"), logStart+"[exit 2] Thu Oct 15 03:00:00 PM UTC 2026\n")
	writeFile(t, filepath.Join(logs, "stop.log"), logStart+"[stopped] Thu Oct 15 03:01:00 PM UTC 2026  exp stop\n")
	writeFile(t, filepath.Join(logs, "dead.log"), logStart+"working\n")
	writeFile(t, filepath.Join(logs, "live.log"),
		logStart+"Monitoring simulation with http://localhost:1234\nstep\n")
	writeFile(t, filepath.Join(logs, "live.heartbeat"), fmt.Sprintf(
		`{"pid": 101, "wall": %d, "sim_time_ns": 5, "wgs_completed": 7, "finished": false, "args": ["-max-wg=100"]}`,
		now.Unix()))
	writeFile(t, filepath.Join(logs, "bg.log"), logStart+"bg\n")
	writeFile(t, filepath.Join(logs, "bg.job"), "4242\npid\n")
	writeFile(t, filepath.Join(logs, "q.job"), "77\nslurm\n")
	writeFile(t, filepath.Join(logs, "archived", "done.log"), logStart+logExit0)
	writeFile(t, filepath.Join(logs, "archived", "live.log"), logStart+logExit0)

	single := filepath.Join(res, "1015_abl_wg_base_3e9a41c")
	writeFile(t, filepath.Join(single, "meta.json"),
		`{"commit": "3e9a41c", "exit_code": 0, "config_deviation": [{"flag": "-x", "change": "removed"}]}`)
	writeFile(t, filepath.Join(single, "summary.csv"), "a,b\n1,2\n")
	testutil.MakeExecInfoDB(t, filepath.Join(single, "trace.sqlite3"),
		map[string]string{"Command": "./sim -benchmark fir"})

	camp1 := filepath.Join(res, "camp1")
	writeFile(t, filepath.Join(camp1, "EXPERIMENT_METADATA.json"), `{"experiment_count": 4, "git_revision": "abc"}`)
	writeFile(t, filepath.Join(camp1, "sim_fir_m1_result.json"), `{"success": true}`)
	writeFile(t, filepath.Join(camp1, "sim_aes_m1_result.json"), `{"success": false}`)

	camp2 := filepath.Join(res, "camp2")
	writeFile(t, filepath.Join(camp2, "campaign.json"), `{"total": 3, "kind": "sensitivity"}`)
	writeFile(t, filepath.Join(camp2, "c1", "run.json"),
		`{"schema":"herd-run/1","name":"c1","fields":{"w":"a"},"properties":[],"status":"ok","metrics":{"t":1}}`)
	writeFile(t, filepath.Join(camp2, "c2", "run.json"),
		`{"schema":"herd-run/1","name":"c2","fields":{"w":"b"},"properties":[],"status":"failed"}`)
	// A cell with metrics and no status is done (as the campaign adapter
	// says); one with neither is still running.
	writeFile(t, filepath.Join(camp2, "c3", "run.json"), `{"name":"c3","fields":{"w":"c"},"metrics":{"t":2}}`)
	writeFile(t, filepath.Join(camp2, "c4", "run.json"), `{"name":"c4","fields":{"w":"d"}}`)
	writeFile(t, filepath.Join(ws, ".herd_archive.json"), `{"results": {"camp2": {"archived_at": "x"}}}`)

	old := filepath.Join(res, "old")
	writeFile(t, filepath.Join(old, "run.json"), `{}`)

	tenDays := now.Add(-10 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(old, tenDays, tenDays))

	for _, d := range []string{single, camp1, camp2} {
		require.NoError(t, os.Chtimes(d, now, now))
	}

	return root
}

func testSystem() *fakeSystem {
	return &fakeSystem{
		panes: map[string]int{"herd-proj-live": 100},
		ppid:  map[int]int{101: 100, 102: 100, 4243: 4242},
		procs: map[int]hostsys.ProcInfo{
			100: {Etime: 100, Args: "bash -lc ./run", Stat: "S"},
			101: {
				Etime: 100, CPUTime: 90, RSSKB: 2048, Stat: "R",
				Args: "/bin/sim -benchmark fir -metric-file-name /w/results/camp1/sim_fir_m1_metrics",
			},
			102:  {Etime: 100, Args: "tee -a logs/live.log", Stat: "S"},
			4242: {Etime: 50, CPUTime: 1, Stat: "S", Args: "python3 bg.py"},
			4243: {
				Etime: 50, CPUTime: 2, Stat: "T",
				Args: "/bin/sim -benchmark aes -metric-file-name /w/results/0101_x/stats/metrics",
			},
		},
		slurm: map[string]string{"77": "queued"},
		http: map[string]string{"http://localhost:1234/api/progress": `[{"id": 3, "name": "kernel",
			"start_time": "2026-10-15T15:00:00Z", "total": 10, "finished": 4, "in_progress": 1}]`},
		writes: map[int]float64{101: 2e9},
	}
}

func sessionByName(t *testing.T, p *herd.HostProbe, name string, archived bool) herd.Session {
	t.Helper()

	for _, s := range p.Sessions {
		if s.Project == "proj" && s.Name == name && s.Archived == archived {
			return s
		}
	}

	t.Fatalf("no session %s (archived %v)", name, archived)

	return herd.Session{}
}

func runByName(t *testing.T, p *herd.HostProbe, name string) herd.Run {
	t.Helper()

	for _, r := range p.Runs {
		if r.Name == name {
			return r
		}
	}

	t.Fatalf("no run %s", name)

	return herd.Run{}
}

func probeTestWorkspace(t *testing.T) (*herd.HostProbe, string) {
	t.Helper()

	now := time.Now()
	root := testWorkspace(t, now)
	before := testutil.Snapshot(t, root)

	p, err := Probe(context.Background(), root, Options{now: now, sys: testSystem()})
	require.NoError(t, err)

	assert.Equal(t, before, testutil.Snapshot(t, root), "the probe must not write under the root")

	return p, root
}

func TestProbeSessions(t *testing.T) {
	p, _ := probeTestWorkspace(t)

	assert.Equal(t, herd.SchemaProbe, p.Schema)
	assert.Equal(t, 8, p.CPUs)
	assert.Contains(t, p.Projects, "proj")
	assert.Contains(t, p.Projects, "other")
	assert.Nil(t, p.Projects["other"])
	assert.Len(t, p.Sessions, 10)

	done := sessionByName(t, p, "done", false)
	assert.Equal(t, "ok", done.State)
	assert.Equal(t, "proj/done", done.ID)
	assert.Equal(t, "./run", done.Cmd)
	assert.Equal(t, "g1", done.Group)
	require.NotNil(t, done.Start)
	assert.Equal(t, float64(time.Date(2026, 10, 15, 14, 59, 54, 0, time.UTC).Unix()), *done.Start)
	require.NotNil(t, done.End)
	assert.Equal(t, 0, *done.ExitCode)
	assert.NotContains(t, strings.Join(done.Tail, "\n"), "GOPATH")

	assert.Equal(t, "failed", sessionByName(t, p, "fail", false).State)
	assert.Equal(t, "stopped", sessionByName(t, p, "stop", false).State)
	assert.NotNil(t, sessionByName(t, p, "stop", false).End)
	assert.Equal(t, "died", sessionByName(t, p, "dead", false).State)
	assert.Equal(t, "queued", sessionByName(t, p, "q", false).State)
	assert.Equal(t, "ok", sessionByName(t, p, "done", true).State)
	assert.Equal(t, "ok", sessionByName(t, p, "live", true).State, "an archived log never owns the live tmux session")

	bg := sessionByName(t, p, "bg", false)
	assert.Equal(t, "running", bg.State)
	assert.Equal(t, "4242", bg.Job, "the job id, not the .job file")
	assert.Equal(t, "77", sessionByName(t, p, "q", false).Job)
	require.Len(t, bg.Sims, 1)
	assert.Equal(t, "0101_x", bg.Sims[0].Campaign)
	assert.True(t, bg.Sims[0].Paused)
}

func TestProbeLiveSession(t *testing.T) {
	p, _ := probeTestWorkspace(t)

	live := sessionByName(t, p, "live", false)
	assert.Equal(t, "running", live.State)
	assert.Equal(t, 2e9, live.LastOutput)
	require.Len(t, live.Procs, 1, "bash -lc and tee are hidden")
	assert.Equal(t, 101, live.Procs[0].PID)
	assert.InDelta(t, 90.0, live.Procs[0].CPU, 1e-9)
	assert.Equal(t, 2.0, live.Procs[0].RSSMB)

	require.Len(t, live.Sims, 1)
	sim := live.Sims[0]
	assert.Equal(t, "fir", sim.Label)
	assert.Equal(t, "camp1", sim.Campaign)
	assert.Equal(t, "m1", sim.Config)
	assert.Equal(t, "http://localhost:1234", sim.Monitor)

	ids := map[string]herd.Progress{}
	for _, b := range live.Progress {
		ids[b.ID] = b
	}

	require.Contains(t, ids, "wg")
	assert.Equal(t, uint64(7), ids["wg"].Finished)
	assert.Equal(t, uint64(100), *ids["wg"].Total)
	require.Contains(t, ids, "akita:1234:3")
	assert.Equal(t, uint64(4), ids["akita:1234:3"].Finished)
	assert.NotNil(t, ids["akita:1234:3"].StartTime)
	require.Contains(t, ids, "campaign:camp1")
	assert.Equal(t, uint64(1), ids["campaign:camp1"].InProgress)
}

func TestProbeRuns(t *testing.T) {
	p, _ := probeTestWorkspace(t)

	assert.Len(t, p.Runs, 3, "the 10-day-old run is not listed")

	single := runByName(t, p, "1015_abl_wg_base_3e9a41c")
	assert.Equal(t, "single", single.Kind)
	assert.Equal(t, "abl", single.Fields["category"])
	assert.Contains(t, single.Properties, herd.Property{Property: "Command", Value: "./sim -benchmark fir"})
	assert.Contains(t, single.Properties, herd.Property{Property: "Git Revision", Value: "3e9a41c"})
	assert.Equal(t, 0, *single.ExitCode)
	assert.Equal(t, []string{"-x"}, single.Deviation)
	assert.Equal(t, "a,b\n1,2\n", single.Summary)
	assert.Nil(t, single.Progress)
	assert.Nil(t, single.Failed)

	camp1 := runByName(t, p, "camp1")
	assert.Equal(t, "campaign", camp1.Kind)
	assert.Nil(t, camp1.Fields)
	require.NotNil(t, camp1.Progress)
	assert.Equal(t, uint64(4), *camp1.Progress.Total)
	assert.Equal(t, uint64(2), camp1.Progress.Finished)
	assert.Equal(t, uint64(1), camp1.Progress.InProgress)
	assert.Equal(t, 1, *camp1.Failed)
	assert.Equal(t, []string{"sim_aes_m1"}, camp1.Failures)
	assert.Contains(t, camp1.Properties, herd.Property{Property: "Runs", Value: "4"})

	camp2 := runByName(t, p, "camp2")
	assert.Equal(t, "sensitivity", camp2.Kind)
	assert.True(t, camp2.Archived)
	assert.Equal(t, uint64(3), *camp2.Progress.Total)
	assert.Equal(t, uint64(3), camp2.Progress.Finished)
	assert.Equal(t, []string{"c2"}, camp2.Failures)
}

func TestProbeRunCaps(t *testing.T) {
	now := time.Now()
	root := t.TempDir()

	for i := range 5 {
		d := filepath.Join(root, "p", "results", fmt.Sprintf("r%d", i))
		writeFile(t, filepath.Join(d, "x"), "")

		mt := now.Add(-time.Duration(i) * time.Minute)
		require.NoError(t, os.Chtimes(d, mt, mt))
	}

	camp := filepath.Join(root, "p", "results", "zcamp")
	writeFile(t, filepath.Join(camp, "EXPERIMENT_METADATA.json"), `{}`)

	mt := now.Add(-time.Hour)
	require.NoError(t, os.Chtimes(camp, mt, mt))

	p, err := Probe(context.Background(), root, Options{now: now, liveRunCap: 2, sys: &fakeSystem{}})
	require.NoError(t, err)

	names := make([]string, 0, len(p.Runs))
	for _, r := range p.Runs {
		names = append(names, r.Name)
	}

	assert.Equal(t, []string{"r0", "r1", "zcamp"}, names, "campaigns beyond the cap are kept")
	assert.Nil(t, runByName(t, p, "zcamp").Progress.Total)
}

func TestOutputTarget(t *testing.T) {
	c, cfg := outputTarget("/r/results/camp/baseline_fft_m3_metrics", "baseline", "fft")
	assert.Equal(t, "camp", c)
	assert.Equal(t, "m3", cfg)

	c, cfg = outputTarget("/r/results/run1/stats/metrics", "baseline", "fft")
	assert.Equal(t, "run1", c)
	assert.Empty(t, cfg)
}

func TestProbeQueue(t *testing.T) {
	root := t.TempDir()

	p, err := Probe(context.Background(), root, Options{})
	require.NoError(t, err)
	assert.Nil(t, p.Queue, "a root without a queue")

	require.NoError(t, queue.Open(root).Put([]*herd.Task{{Project: "p", Name: "a", Cmd: "true"}}))

	p, err = Probe(context.Background(), root, Options{})
	require.NoError(t, err)
	require.NotNil(t, p.Queue)
	require.Len(t, p.Queue.Pending, 1)
	assert.Equal(t, "a", p.Queue.Pending[0].Name)
	assert.False(t, p.Queue.Alive)
}
