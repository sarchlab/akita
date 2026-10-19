package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/sarchlab/akita/v5/experiment/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var writeFile = testutil.WriteFile

func TestWriteRecordCreatesAndMerges(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "1015_abl_wg_base_3e9a41c")
	writeFile(t, filepath.Join(dir, "meta.json"),
		`{"commit": "3e9a41c", "host": "server2", "exit_code": 0, "start": "2026-10-15 15:00:16"}`)

	rec, err := WriteRecord(dir, testNaming(), []Property{{Property: "Command", Value: "./run"}})
	require.NoError(t, err)
	assert.Equal(t, SchemaRun, rec.Schema)
	assert.Equal(t, "1015_abl_wg_base_3e9a41c", rec.Name)
	assert.Equal(t, "abl", rec.Fields["category"])
	assert.Equal(t, "./run", rec.Property("Command"))
	assert.Equal(t, "3e9a41c", rec.Property("Git Revision"))
	assert.Equal(t, "server2", rec.Property("Host"))
	assert.Equal(t, "0", rec.Property("Exit Code"))
	assert.Equal(t, "2026-10-15 15:00:16", rec.Property("Start Time"))

	// Add an unknown key by hand; a second write keeps it and replaces
	// properties of the same name.
	var raw map[string]any

	data, err := os.ReadFile(filepath.Join(dir, RunRecordFile))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["custom"] = "kept"
	raw["metrics"] = map[string]any{"total_time": 1.5}
	data, err = json.Marshal(raw)
	require.NoError(t, err)
	writeFile(t, filepath.Join(dir, RunRecordFile), string(data))

	rec, err = WriteRecord(dir, testNaming(),
		[]Property{{Property: "Command", Value: "./run2"}, {Property: "Note", Value: "x"}})
	require.NoError(t, err)
	assert.Equal(t, "./run2", rec.Property("Command"))
	assert.Equal(t, "x", rec.Property("Note"))
	assert.Equal(t, 1.5, rec.Metrics["total_time"])

	count := 0

	for _, p := range rec.Properties {
		if p.Property == "Command" {
			count++
		}
	}

	assert.Equal(t, 1, count)

	data, err = os.ReadFile(filepath.Join(dir, RunRecordFile))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "kept", raw["custom"])

	back, err := ReadRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, rec.Properties, back.Properties)
}

func TestWriteRecordUnparsedNameKeepsFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "oldname")
	writeFile(t, filepath.Join(dir, RunRecordFile), `{"schema":"exp-run/1","name":"x","fields":{"a":"b"},"properties":[]}`)

	rec, err := WriteRecord(dir, testNaming(), nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b"}, rec.Fields)
	assert.Equal(t, "oldname", rec.Name)
}

func TestReadRecordMissing(t *testing.T) {
	_, err := ReadRecord(t.TempDir())
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestParseProperties(t *testing.T) {
	props, err := ParseProperties([]string{"Command=./a b=c", "Note="})
	require.NoError(t, err)
	assert.Equal(t, []Property{{"Command", "./a b=c"}, {"Note", ""}}, props)

	_, err = ParseProperties([]string{"nokey"})
	require.Error(t, err)
}

func TestMetaPropertiesFromCampaignMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "EXPERIMENT_METADATA.json"),
		`{"experiment_count": 65, "git_revision": "abc", "created_at": "2026-10-15T15:00:16", "experiments": [1,2]}`)

	props := MetaProperties(dir)
	assert.Contains(t, props, Property{"Runs", "65"})
	assert.Contains(t, props, Property{"Git Revision", "abc"})
	assert.Contains(t, props, Property{"Start Time", "2026-10-15T15:00:16"})
}

func TestExecInfo(t *testing.T) {
	dir := t.TempDir()
	testutil.MakeExecInfoDB(t, filepath.Join(dir, "a.sqlite3"),
		map[string]string{"Command": "./sim -trace", "Start Time": "t0"})
	writeFile(t, filepath.Join(dir, "broken.sqlite3"), "not a database")
	writeFile(t, filepath.Join(dir, "empty.sqlite3"), "")

	names := []string{"a.sqlite3", "broken.sqlite3", "empty.sqlite3", "missing.sqlite3", "other.txt"}
	props := ExecInfo(context.Background(), dir, names)
	assert.ElementsMatch(t, []Property{{"Command", "./sim -trace"}, {"Start Time", "t0"}}, props)

	assert.Nil(t, readExecInfo(context.Background(), filepath.Join(dir, "missing.sqlite3")))
	assert.Nil(t, ExecInfo(context.Background(), filepath.Join(dir, "nodir"), []string{"x.sqlite3"}))
}

func TestWriteRecordConcurrentAndAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "1015_abl_wg_base_3e9a41c")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	// A leftover of the old fixed temporary name does not matter.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "run.json.tmp"), 0o755))

	var wg sync.WaitGroup

	errs := make(chan error, 16)

	for i := range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := WriteRecord(dir, testNaming(), []Property{{Property: "N", Value: strconv.Itoa(i)}})
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	rec, err := ReadRecord(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, rec.Property("N"))

	fi, err := os.Stat(filepath.Join(dir, RunRecordFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, e := range entries {
		assert.Contains(t, []string{"run.json", "run.json.tmp"}, e.Name(), "temporary file left behind")
	}
}
