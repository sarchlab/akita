package probe

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeRunDetail(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "proj")
	writeFile(t, filepath.Join(ws, "exp.json"), testProjectConfig)

	dir := filepath.Join(ws, "results", "1015_abl_wg_base_3e9a41c")
	writeFile(t, filepath.Join(dir, "meta.json"), `{"exit_code": 0}`)
	writeFile(t, filepath.Join(dir, "config_used.json"), `{"config_files": ["a.json"], "flags": {"x": 1},
		"all_flags": {"-b": {"source": "command line"}, "-a": {"source": "command line"}, "-c": {"source": "default"}}}`)
	writeFile(t, filepath.Join(dir, "stats", "metrics.csv"),
		"id,where,what,value\n0,GPU[0].CU[1],cpi,2\n1,GPU[1].CU[0],cpi,4\n2,GPU[0].CU[2],cpi,NaN\n3,bad\n")

	d, err := Run(context.Background(), root, "proj", "1015_abl_wg_base_3e9a41c")
	require.NoError(t, err)
	assert.True(t, d.Finished)
	assert.Equal(t, "abl", d.Fields["category"])
	assert.Contains(t, d.Properties, experiment.Property{Property: "Exit Code", Value: "0"})
	assert.Equal(t, []string{"-a", "-b"}, d.Config.CommandLine)
	assert.Equal(t, []any{"a.json"}, d.Config.Files)

	require.NotNil(t, d.Metrics)
	assert.Equal(t, 3, d.Metrics.Count)
	assert.Equal(t, experiment.StatAgg{Sum: 6, N: 2, Min: 2, Max: 4}, d.Metrics.Agg["GPU.CU|cpi"])
	assert.Equal(t, 1, d.Metrics.Nonfinite["GPU.CU|cpi"])
	assert.Len(t, d.Metrics.Fingerprint, 64)
	assert.Nil(t, d.Summary)
	assert.Nil(t, d.Record)

	_, err = Run(context.Background(), root, "proj", "../proj")
	require.Error(t, err)
	_, err = Run(context.Background(), root, "proj", "missing")
	require.Error(t, err)
}

func TestProbeCampaignRunsAdapter(t *testing.T) {
	root := t.TempDir()
	camp := filepath.Join(root, "proj", "results", "camp")
	writeFile(t, filepath.Join(camp, "campaign.json"), `{"total": 3, "order": {"w": ["b", "a"]}}`)
	writeFile(t, filepath.Join(camp, "c1", "run.json"),
		`{"fields":{"w":"a","config":"m1"},"properties":[],"metrics":{"t":1}}`)
	writeFile(t, filepath.Join(camp, "c2", "run.json"),
		`{"fields":{"w":"b","config":"m1"},"properties":[],"status":"failed"}`)
	writeFile(t, filepath.Join(camp, "c3", "run.json"), `{"fields":{"w":"c","config":"m1"},"properties":[]}`)

	c, err := Campaign(context.Background(), root, "proj", "camp")
	require.NoError(t, err)
	assert.Equal(t, experiment.SchemaCells, c.Schema)
	assert.Equal(t, 3, c.Total)
	assert.Equal(t, []string{"config", "w"}, c.Dimensions)
	assert.Equal(t, []string{"b", "a"}, c.Order["w"])
	require.Len(t, c.Cells, 3)
	assert.Equal(t, "ok", c.Cells[0].Status)
	assert.Equal(t, "failed", c.Cells[1].Status)
	assert.Equal(t, "running", c.Cells[2].Status)
	assert.False(t, CampaignFinished(c))
}

func TestProbeCampaignCommandAdapter(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "proj")
	writeFile(t, filepath.Join(ws, "exp.json"),
		`{"schema":"exp-project/1","campaigns":{"adapter":{"type":"command",
		  "command":["sh","-c","cat \"$0/cells.json\"; pwd >&2"]}}}`)
	camp := filepath.Join(ws, "results", "camp")
	writeFile(t, filepath.Join(camp, "cells.json"),
		`{"dimensions":["w"],"cells":[{"key":{"w":"a"},"status":"ok","metrics":{"t":1}}],"total":1}`)

	c, err := Campaign(context.Background(), root, "proj", "camp")
	require.NoError(t, err)
	assert.Equal(t, []string{"w"}, c.Dimensions)
	assert.Equal(t, 1.0, c.Cells[0].Metrics["t"])
	assert.True(t, CampaignFinished(c))

	writeFile(t, filepath.Join(ws, "exp.json"),
		`{"schema":"exp-project/1","campaigns":{"adapter":{"type":"command","command":["sh","-c","echo boom >&2; exit 3"]}}}`)
	_, err = Campaign(context.Background(), root, "proj", "camp")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
