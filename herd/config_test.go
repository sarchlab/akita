package herd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The config names before the rename (exp.json, AKITA_EXP_CONFIG) are read
// while the new ones are absent.
func TestUserConfigPathFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(userConfigEnv, "")
	t.Setenv(legacyUserConfigEnv, "")

	dir := filepath.Join(home, ".config", "akita")
	newPath, oldPath := filepath.Join(dir, "herd.json"), filepath.Join(dir, "exp.json")

	assert.Equal(t, newPath, UserConfigPath(), "neither exists: the new name")

	writeFile(t, oldPath, `{"hosts": [{"name": "old", "type": "local"}]}`)
	assert.Equal(t, oldPath, UserConfigPath(), "only exp.json exists")

	cfg, err := LoadUserConfig()
	require.NoError(t, err)

	_, ok := cfg.Host("old")
	assert.True(t, ok)

	writeFile(t, newPath, `{"hosts": [{"name": "new", "type": "local"}]}`)
	assert.Equal(t, newPath, UserConfigPath(), "herd.json wins")

	t.Setenv(legacyUserConfigEnv, "/x/legacy.json")
	assert.Equal(t, "/x/legacy.json", UserConfigPath(), "AKITA_EXP_CONFIG beats the files")

	t.Setenv(userConfigEnv, "/x/herd.json")
	assert.Equal(t, "/x/herd.json", UserConfigPath(), "AKITA_HERD_CONFIG beats AKITA_EXP_CONFIG")
}

func TestProjectConfigFallback(t *testing.T) {
	dir := t.TempDir()

	cfg, err := LoadProjectConfig(dir)
	require.NoError(t, err)
	assert.Nil(t, cfg)
	assert.Equal(t, filepath.Join(dir, ProjectConfigFile), ProjectConfigPath(dir))

	writeFile(t, filepath.Join(dir, LegacyProjectConfigFile),
		`{"schema": "exp-project/1", "sync": {"exclude": ["old/"]}}`)
	assert.Equal(t, filepath.Join(dir, LegacyProjectConfigFile), ProjectConfigPath(dir))

	cfg, err = LoadProjectConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"old/"}, cfg.Sync.Exclude)

	writeFile(t, filepath.Join(dir, ProjectConfigFile),
		`{"schema": "herd-project/1", "sync": {"exclude": ["new/"]}}`)

	cfg, err = LoadProjectConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"new/"}, cfg.Sync.Exclude, "herd.json wins")
}

func TestProjectConfigErrorNamesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, LegacyProjectConfigFile), `{"schema": "other/1"}`)

	_, err := LoadProjectConfig(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exp.json: unknown schema")
}

func TestNormalizeSchema(t *testing.T) {
	for in, want := range map[string]string{
		"exp-project/1": SchemaProject, "exp-run/1": SchemaRun, "exp-task/1": SchemaTask,
		"exp-cells/1": SchemaCells, "exp-status/1": SchemaStatus, "exp-probe/1": SchemaProbe,
		SchemaRun: SchemaRun, "": "", "other/1": "other/1",
	} {
		assert.Equal(t, want, NormalizeSchema(in), in)
	}
}

func TestReadRecordLegacySchema(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, RunRecordFile), `{"schema":"exp-run/1","name":"x","properties":[]}`)

	rec, err := ReadRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, SchemaRun, rec.Schema)
}
