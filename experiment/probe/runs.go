package probe

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// Run listing limits.
const (
	maxSummaryBytes   = 3000
	maxFilesListed    = 30
	maxFailuresListed = 20
	summaryFile       = "summary.csv"
	resultFileSuffix  = "_result.json" // a campaign cell's outcome (EXPERIMENT_METADATA.json style)
)

// resultDir is a candidate run directory.
type resultDir struct {
	ws       wsRef
	name     string
	path     string
	mtime    float64 // of the directory
	marker   string  // campaign marker file found, or ""
	markTime float64 // max(directory, marker) mtime
	archived bool
}

func (p *prober) runs() []experiment.Run {
	dirs := make([]resultDir, 0, len(p.workspaces))

	for _, w := range p.workspaces {
		dirs = append(dirs, p.resultDirs(w)...)
	}

	maxAge := p.opts.maxAge.Seconds()

	var recent []resultDir

	for _, d := range dirs {
		if p.now-d.mtime <= maxAge {
			recent = append(recent, d)
		}
	}

	sort.SliceStable(recent, func(i, j int) bool { return recent[i].mtime > recent[j].mtime })

	// Archived runs must not crowd the live ones out of the cap.
	selected := append(capRuns(recent, false, p.opts.liveRunCap), capRuns(recent, true, p.opts.archivedRunCap)...)
	in := map[string]bool{}

	for _, d := range selected {
		in[d.path] = true
	}

	// Campaigns beyond the cap are listed anyway (by name).
	var extra []resultDir

	for _, d := range dirs {
		if d.marker != "" && !in[d.path] && p.now-d.markTime <= maxAge {
			d.mtime = d.markTime
			extra = append(extra, d)
		}
	}

	sort.SliceStable(extra, func(i, j int) bool { return extra[i].name < extra[j].name })

	runs := make([]experiment.Run, 0, len(selected)+len(extra))
	for _, d := range append(selected, extra...) {
		runs = append(runs, p.run(d))
	}

	return runs
}

func capRuns(dirs []resultDir, archived bool, n int) []resultDir {
	var out []resultDir

	for _, d := range dirs {
		if d.archived == archived && len(out) < n {
			out = append(out, d)
		}
	}

	return out
}

// resultDirs lists results/*/ of a workspace with their campaign markers.
// It stats each directory and marker but reads nothing else.
func (p *prober) resultDirs(w wsRef) []resultDir {
	base := workspace.ResultsPath(w.dir)
	cfg := p.projects[w.project]
	dirs := workspace.ResultDirs(base)
	out := make([]resultDir, 0, len(dirs))

	for _, r := range dirs {
		d := resultDir{
			ws: w, name: r.Name, path: filepath.Join(base, r.Name),
			mtime:    experiment.UnixSeconds(r.MTime),
			archived: p.isArchived(w, r.Name),
		}
		d.markTime = d.mtime

		for _, marker := range cfg.CampaignDetect() {
			if mst, err := os.Stat(filepath.Join(d.path, marker)); err == nil {
				d.marker = marker
				d.markTime = max(d.mtime, experiment.UnixSeconds(mst.ModTime()))

				break
			}
		}

		out = append(out, d)
	}

	return out
}

// run reads one result directory.
func (p *prober) run(d resultDir) experiment.Run {
	cfg := p.projects[d.ws.project]
	names := listNames(d.path)
	meta := readJSON(filepath.Join(d.path, experiment.MetaFile))
	rec, _ := experiment.ReadRecord(d.path)

	r := experiment.Run{
		ID: d.ws.project + "/" + d.name, Project: d.ws.project, Name: d.name,
		Archived: d.archived, MTime: d.mtime, Kind: "single",
		Failures: []string{}, Deviation: deviation(meta),
		Files: names[:min(len(names), maxFilesListed)],
	}

	if hostsys.Exists(filepath.Join(d.path, summaryFile)) {
		r.Summary = hostsys.ReadText(filepath.Join(d.path, summaryFile), maxSummaryBytes)
	}

	if code, ok := meta["exit_code"].(float64); ok {
		r.ExitCode = new(int(code))
	}

	var campaignMeta map[string]any
	if d.marker != "" {
		campaignMeta = p.campaign(&r, d, names)
	}

	r.Properties = []experiment.Property{}
	if rec != nil {
		r.Properties = append(r.Properties, rec.Properties...)
		r.Fields = rec.Fields
		r.Metrics = rec.Metrics
	}

	if r.Fields == nil {
		r.Fields = experiment.ParseName(d.name, namingOf(cfg))
	}

	r.Properties = experiment.MergeProperties(r.Properties, experiment.ExecInfo(p.ctx, d.path, names), false)
	r.Properties = experiment.MergeProperties(r.Properties, experiment.MetaPropertiesOf(meta, campaignMeta), false)

	return r
}

func namingOf(cfg *experiment.ProjectConfig) *experiment.Naming {
	if cfg == nil {
		return nil
	}

	return cfg.Naming
}

// readJSON reads a JSON document of a result directory.
func readJSON(path string) map[string]any {
	return hostsys.ReadJSONMap(path, experiment.MaxRecordBytes)
}

// listNames are the sorted names of a directory's entries.
func listNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	sort.Strings(names)

	return names
}

// deviation lists the flags meta.json's config_deviation reports as
// removed.
func deviation(meta map[string]any) []string {
	out := []string{}

	list, _ := meta["config_deviation"].([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok || m["change"] != "removed" {
			continue
		}

		if flag, ok := m["flag"].(string); ok {
			out = append(out, flag)
		}
	}

	return out
}

// campaignCounts is the cheap progress of a campaign.
type campaignCounts struct {
	total    *uint64
	ok       int
	failed   int
	failures []string
}

// campaign fills a campaign run's kind and progress and returns the
// campaign metadata used for properties. Two fast paths, neither runs the
// adapter:
//
//   - EXPERIMENT_METADATA.json style: that file gives the total
//     ("experiment_count"), and each <cell>_result.json its outcome
//     ("success").
//   - generic: campaign.json {"total": N, "kind": ...} gives the total, and
//     each subdirectory's run.json its "status".
func (p *prober) campaign(r *experiment.Run, d resultDir, names []string) map[string]any {
	r.Kind = "campaign"

	var (
		meta   map[string]any
		counts campaignCounts
	)

	if raw := readJSON(filepath.Join(d.path, experiment.CampaignMetaFile)); raw != nil ||
		d.marker == experiment.CampaignMetaFile {
		meta = experiment.ScalarMap(raw)
		counts = resultJSONCounts(d.path, names)
		counts.total = jsonCount(raw["experiment_count"])
	} else {
		raw := readJSON(filepath.Join(d.path, experiment.CampaignFile))
		meta = experiment.ScalarMap(raw)
		counts = subdirCounts(d.path)
		counts.total = jsonCount(raw["total"])

		if raw["kind"] == "sensitivity" {
			r.Kind = "sensitivity"
		}
	}

	if hostsys.Exists(filepath.Join(d.path, experiment.SensitivityMarkerFile)) {
		r.Kind = "sensitivity"
	}

	sort.Strings(counts.failures)

	r.Failed = new(counts.failed)
	r.Failures = counts.failures[:min(len(counts.failures), maxFailuresListed)]
	r.Progress = &experiment.Progress{
		ID: experiment.ProgressPrefixCampaign + d.name, Name: d.name, Total: counts.total,
		Finished: uint64(counts.ok + counts.failed),
	}

	return meta
}

func jsonCount(v any) *uint64 {
	f, ok := v.(float64)
	if !ok || f < 0 {
		return nil
	}

	return new(uint64(f))
}

func resultJSONCounts(dir string, names []string) campaignCounts {
	c := campaignCounts{failures: []string{}}

	for _, n := range names {
		cell, ok := strings.CutSuffix(n, resultFileSuffix)
		if !ok {
			continue
		}

		m := readJSON(filepath.Join(dir, n))
		if m == nil {
			continue
		}

		if experiment.PyString(m["success"]) == "True" {
			c.ok++
		} else {
			c.failed++
			c.failures = append(c.failures, cell)
		}
	}

	return c
}

func subdirCounts(dir string) campaignCounts {
	c := campaignCounts{failures: []string{}}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return c
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		rec, err := experiment.ReadRecord(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		switch rec.Status {
		case "ok":
			c.ok++
		case "failed", "bad":
			c.failed++
			c.failures = append(c.failures, e.Name())
		}
	}

	return c
}

// attachCampaignBars counts the simulators writing to each campaign and
// adds the campaign's progress bar to the sessions running them.
func (p *prober) attachCampaignBars(sessions []experiment.Session, runs []experiment.Run) {
	type key struct{ project, name string }

	byName := map[key]*experiment.Run{}

	for i := range runs {
		if runs[i].Progress != nil {
			byName[key{runs[i].Project, runs[i].Name}] = &runs[i]
		}
	}

	for _, s := range sessions {
		for _, sim := range s.Sims {
			if r := byName[key{s.Project, sim.Campaign}]; r != nil && sim.Campaign != "" {
				r.Progress.InProgress++
			}
		}
	}

	for i := range sessions {
		s := &sessions[i]
		seen := map[string]bool{}

		for _, sim := range s.Sims {
			r := byName[key{s.Project, sim.Campaign}]
			if r == nil || seen[sim.Campaign] {
				continue
			}

			seen[sim.Campaign] = true
			s.Progress = append(s.Progress, *r.Progress)
		}
	}
}
