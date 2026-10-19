package probe

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// Detail limits.
const (
	maxDetailFiles   = 500
	maxDetailSummary = 20000
	statsLineBuf     = 64 << 10 // initial line buffer of metrics.csv
	maxStatsLine     = 4 << 20
	configUsedFile   = "config_used.json"
	statsFile        = "stats/metrics.csv"
	statsMinColumns  = 4 // "_,where,what,value"
)

// runPath checks project and name and returns <root>/<project>/results/<name>.
func runPath(root, project, name string) (string, error) {
	for _, s := range []string{project, name} {
		if workspace.ValidateRunName(s) != nil {
			return "", fmt.Errorf("bad project or run name %q", s)
		}
	}

	dir := filepath.Join(workspace.ResultsPath(filepath.Join(experiment.ExpandHome(root), project)), name)

	st, err := os.Stat(dir)
	if err != nil {
		return "", err
	}

	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}

	return dir, nil
}

// Run reads one result directory <root>/<project>/results/<name> in
// detail.
func Run(ctx context.Context, root, project, name string) (*experiment.RunDetail, error) {
	dir, err := runPath(root, project, name)
	if err != nil {
		return nil, err
	}

	cfg, _ := experiment.LoadProjectConfig(filepath.Join(experiment.ExpandHome(root), project))
	names := listNames(dir)
	rec, _ := experiment.ReadRecord(dir)

	d := &experiment.RunDetail{
		Project: project, Name: name, Record: rec,
		Meta:    readJSON(filepath.Join(dir, experiment.MetaFile)),
		Summary: readSummary(filepath.Join(dir, summaryFile)),
		Files:   names[:min(len(names), maxDetailFiles)],
		Config:  readConfigUsed(filepath.Join(dir, configUsedFile)),
		Metrics: readStatsDigest(filepath.Join(dir, filepath.FromSlash(statsFile))),
	}

	d.Properties = []experiment.Property{}
	if rec != nil {
		d.Properties = append(d.Properties, rec.Properties...)
		d.Fields = rec.Fields
	}

	if d.Fields == nil {
		d.Fields = experiment.ParseName(name, namingOf(cfg))
	}

	d.Properties = experiment.MergeProperties(d.Properties, experiment.ExecInfo(ctx, dir, names), false)
	d.Properties = experiment.MergeProperties(d.Properties, experiment.MetaProperties(dir), false)
	d.Finished = d.Meta["exit_code"] != nil || hasProperty(d.Properties, "End Time")

	return d, nil
}

// readSummary reads summary.csv with "\n" line ends; nil without one.
func readSummary(path string) *string {
	data, err := hostsys.ReadLimited(path, maxDetailSummary)
	if err != nil {
		return nil
	}

	return new(strings.ReplaceAll(strings.ToValidUTF8(string(data), "�"), "\r\n", "\n"))
}

func hasProperty(props []experiment.Property, name string) bool {
	for _, p := range props {
		if p.Property == name && p.Value != "" {
			return true
		}
	}

	return false
}

func readConfigUsed(path string) *experiment.ConfigUsed {
	used := readJSON(path)
	if used == nil {
		return nil
	}

	c := &experiment.ConfigUsed{Files: used["config_files"], Flags: used["flags"], CommandLine: []string{}}
	if c.Files == nil {
		c.Files = []any{}
	}

	if c.Flags == nil {
		c.Flags = map[string]any{}
	}

	all, _ := used["all_flags"].(map[string]any)
	for k, v := range all {
		if m, ok := v.(map[string]any); ok && m["source"] == "command line" {
			c.CommandLine = append(c.CommandLine, k)
		}
	}

	sort.Strings(c.CommandLine)

	return c
}

var indexRe = regexp.MustCompile(`\[\d+\]`)

func readStatsDigest(path string) *experiment.StatsDigest {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	d := &experiment.StatsDigest{Agg: map[string]experiment.StatAgg{}, Nonfinite: map[string]int{}}

	var lines []string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, statsLineBuf), maxStatsLine)

	for first := true; sc.Scan(); first = false {
		if first {
			continue
		}

		if line, ok := addStatsRow(d, sc.Text()); ok {
			lines = append(lines, line)
		}
	}

	sort.Strings(lines)
	d.Count = len(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	d.Fingerprint = hex.EncodeToString(sum[:])

	return d
}

// addStatsRow counts one metrics.csv row and returns its canonical form.
func addStatsRow(d *experiment.StatsDigest, row string) (string, bool) {
	parts := strings.Split(row, ",")
	if len(parts) < statsMinColumns {
		return "", false
	}

	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	value, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return "", false
	}

	key := indexRe.ReplaceAllString(parts[1], "") + "|" + parts[2]
	if math.IsInf(value, 0) || math.IsNaN(value) {
		d.Nonfinite[key]++
	} else {
		a, ok := d.Agg[key]
		if !ok {
			a.Min, a.Max = value, value
		}

		a.Sum += value
		a.N++
		a.Min = min(a.Min, value)
		a.Max = max(a.Max, value)
		d.Agg[key] = a
	}

	return strings.Join(parts[1:4], ","), true
}
