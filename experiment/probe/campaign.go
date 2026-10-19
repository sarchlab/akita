package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
)

// Campaign adapter limits.
const (
	adapterTimeout     = 5 * time.Minute
	maxAdapterErrBytes = 500
)

// Campaign reads the cells of the campaign <root>/<project>/results/<name>
// with the project's campaign adapter. A "command" adapter runs in the
// workspace with the campaign directory appended to its arguments and must
// print a CampaignCells document.
func Campaign(ctx context.Context, root, project, name string) (*experiment.CampaignCells, error) {
	dir, err := runPath(root, project, name)
	if err != nil {
		return nil, err
	}

	wsp := filepath.Join(experiment.ExpandHome(root), project)

	cfg, err := experiment.LoadProjectConfig(wsp)
	if err != nil {
		return nil, err
	}

	adapter := cfg.CampaignAdapter()

	var cells *experiment.CampaignCells

	switch adapter.Type {
	case "runs":
		cells = cellsFromRuns(dir, cfg)
	case "command":
		cells, err = cellsFromCommand(ctx, wsp, dir, adapter.Command)
	default:
		err = fmt.Errorf("unknown campaign adapter %q", adapter.Type)
	}

	if err != nil {
		return nil, err
	}

	NormalizeCells(cells)

	return cells, nil
}

func cellsFromCommand(ctx context.Context, wsp, dir string, command []string) (*experiment.CampaignCells, error) {
	if len(command) == 0 {
		return nil, errors.New("campaign adapter has no command")
	}

	ctx, cancel := context.WithTimeout(ctx, adapterTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	args := append(append([]string{}, command[1:]...), dir)
	cmd := exec.CommandContext(ctx, command[0], args...)
	cmd.Dir = wsp
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > maxAdapterErrBytes {
			msg = msg[len(msg)-maxAdapterErrBytes:]
		}

		return nil, fmt.Errorf("campaign adapter %s: %w: %s", command[0], err, msg)
	}

	cells := &experiment.CampaignCells{}
	if err := json.Unmarshal(stdout.Bytes(), cells); err != nil {
		return nil, fmt.Errorf("campaign adapter %s: bad output: %w", command[0], err)
	}

	return cells, nil
}

// cellsFromRuns is the "runs" adapter: each subdirectory with a run.json
// is a cell keyed by its fields. campaign.json, when present, provides
// total, dimensions, labels, order, and sweep.
func cellsFromRuns(dir string, cfg *experiment.ProjectConfig) *experiment.CampaignCells {
	cells := &experiment.CampaignCells{}

	data, err := hostsys.ReadLimited(filepath.Join(dir, experiment.CampaignFile), experiment.MaxRecordBytes)
	if err == nil {
		_ = json.Unmarshal(data, cells)
	}

	cells.Cells = nil
	dims := map[string]bool{}

	for _, n := range listNames(dir) {
		rec, err := experiment.ReadRecord(filepath.Join(dir, n))
		if err != nil {
			continue
		}

		key := rec.Fields
		if key == nil {
			key = experiment.ParseName(n, namingOf(cfg))
		}

		if key == nil {
			key = map[string]string{"name": n}
		}

		for k := range key {
			dims[k] = true
		}

		cells.Cells = append(cells.Cells, experiment.Cell{
			Key: key, Status: cellStatus(rec), Metrics: rec.Metrics,
		})
	}

	if len(cells.Dimensions) == 0 {
		cells.Dimensions = cellDimensions(dims, namingOf(cfg))
	}

	return cells
}

func cellStatus(rec *experiment.RunRecord) string {
	switch {
	case rec.Status != "":
		return rec.Status
	case len(rec.Metrics) > 0:
		return "ok"
	default:
		return "running"
	}
}

// cellDimensions orders the key names: naming fields first, the rest
// sorted.
func cellDimensions(dims map[string]bool, n *experiment.Naming) []string {
	out := []string{}

	if n != nil {
		for _, f := range n.Fields {
			if dims[f.Name] {
				out = append(out, f.Name)
				delete(dims, f.Name)
			}
		}
	}

	rest := make([]string, 0, len(dims))
	for k := range dims {
		rest = append(rest, k)
	}

	sort.Strings(rest)

	return append(out, rest...)
}

// NormalizeCells fills the defaults of a campaign's cells: the schema,
// empty lists and metrics, and the total.
func NormalizeCells(c *experiment.CampaignCells) {
	if c.Schema == "" {
		c.Schema = experiment.SchemaCells
	}

	if c.Cells == nil {
		c.Cells = []experiment.Cell{}
	}

	if c.Dimensions == nil {
		c.Dimensions = []string{}
	}

	if c.Warnings == nil {
		c.Warnings = []string{}
	}

	for i := range c.Cells {
		if c.Cells[i].Metrics == nil {
			c.Cells[i].Metrics = map[string]float64{}
		}
	}

	if c.Total == 0 {
		c.Total = len(c.Cells)
	}
}

// CampaignFinished reports whether no cell of a campaign is running or
// missing and every expected cell exists.
func CampaignFinished(c *experiment.CampaignCells) bool {
	for _, cell := range c.Cells {
		if cell.Status == "running" || cell.Status == "missing" {
			return false
		}
	}

	return len(c.Cells) >= c.Total
}
