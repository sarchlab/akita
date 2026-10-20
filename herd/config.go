package herd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ProjectConfigFile is the name of a project's config file.
const ProjectConfigFile = "herd.json"

// LegacyProjectConfigFile is the project config name of the tool's former
// name (akita exp). It is read when ProjectConfigFile does not exist.
const LegacyProjectConfigFile = "exp.json"

// userConfigEnv names the environment variable that overrides the user
// config path; legacyUserConfigEnv is its former name, read when
// userConfigEnv is unset.
const (
	userConfigEnv       = "AKITA_HERD_CONFIG"
	legacyUserConfigEnv = "AKITA_EXP_CONFIG"
)

// User config file names in ~/.config/akita/: the current one and the
// former one, read when the current one does not exist.
const (
	userConfigFile       = "herd.json"
	legacyUserConfigFile = "exp.json"
)

// DefaultRoot is the workspace root of ssh and slurm hosts.
const DefaultRoot = "~/exp"

// Monitor rule defaults, used when the user config leaves them zero.
const (
	defaultStallMinutes     = 30
	defaultIdleCPU          = 5
	defaultSimStallMinutes  = 10
	defaultWorkStallMinutes = 30
	defaultInterval         = 10
)

// defaultCampaignDetect are the marker files that make a run a campaign
// when the project config does not list its own.
var defaultCampaignDetect = []string{CampaignMetaFile, CampaignFile}

// ExpandHome expands a leading "~" of a path on this machine.
func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}

	return filepath.Join(home, p[1:])
}

// ProjectConfigPath returns the path of a project's config file:
// <dir>/herd.json, or <dir>/exp.json when only that (the former name)
// exists.
func ProjectConfigPath(dir string) string {
	p := filepath.Join(dir, ProjectConfigFile)
	if fileExists(p) {
		return p
	}

	if legacy := filepath.Join(dir, LegacyProjectConfigFile); fileExists(legacy) {
		return legacy
	}

	return p
}

// LoadProjectConfig reads the project config (see ProjectConfigPath). It
// returns a nil config and a nil error when the file does not exist.
func LoadProjectConfig(dir string) (*ProjectConfig, error) {
	path := ProjectConfigPath(dir)

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // a project without herd.json has no config
	}

	if err != nil {
		return nil, err
	}

	return parseProjectConfigFile(filepath.Base(path), data)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// parseProjectConfig decodes a herd.json document and checks its naming
// rule.
func parseProjectConfig(data []byte) (*ProjectConfig, error) {
	return parseProjectConfigFile(ProjectConfigFile, data)
}

// parseProjectConfigFile is parseProjectConfig with the file name that
// errors give.
func parseProjectConfigFile(name string, data []byte) (*ProjectConfig, error) {
	cfg := &ProjectConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	if cfg.Schema != "" && NormalizeSchema(cfg.Schema) != SchemaProject {
		return nil, fmt.Errorf("%s: unknown schema %q", name, cfg.Schema)
	}

	if cfg.Naming != nil {
		if _, err := compileNaming(cfg.Naming); err != nil {
			return nil, fmt.Errorf("%s: naming: %w", name, err)
		}
	}

	if err := cfg.Build.validate(); err != nil {
		return nil, fmt.Errorf("%s: build: %w", name, err)
	}

	return cfg, nil
}

func (b *BuildConfig) validate() error {
	if b == nil {
		return nil
	}

	seen := map[string]bool{}

	for _, bin := range b.Binaries {
		if bin.Name == "" || strings.ContainsAny(bin.Name, "/\\ {}:") || strings.HasPrefix(bin.Name, ".") {
			return fmt.Errorf("bad binary name %q", bin.Name)
		}

		if seen[bin.Name] {
			return fmt.Errorf("duplicate binary %q", bin.Name)
		}

		seen[bin.Name] = true

		if len(bin.Command) == 0 {
			return fmt.Errorf("binary %s: no command", bin.Name)
		}
	}

	return nil
}

// Binary returns the build spec of a binary.
func (c *ProjectConfig) Binary(name string) (BinarySpec, bool) {
	if c == nil || c.Build == nil {
		return BinarySpec{}, false
	}

	for _, b := range c.Build.Binaries {
		if b.Name == name {
			return b, true
		}
	}

	return BinarySpec{}, false
}

// CampaignDetect returns the marker files that make a run a campaign.
func (c *ProjectConfig) CampaignDetect() []string {
	if c == nil || c.Campaigns == nil || len(c.Campaigns.Detect) == 0 {
		return defaultCampaignDetect
	}

	return c.Campaigns.Detect
}

// CampaignAdapter returns the project's campaign adapter ("runs" by
// default).
func (c *ProjectConfig) CampaignAdapter() CampaignAdapter {
	if c == nil || c.Campaigns == nil || c.Campaigns.Adapter.Type == "" {
		return CampaignAdapter{Type: "runs"}
	}

	return c.Campaigns.Adapter
}

// UserConfigPath returns the user config path: $AKITA_HERD_CONFIG, else
// $AKITA_EXP_CONFIG, else ~/.config/akita/herd.json, or
// ~/.config/akita/exp.json when only that (the former name) exists.
func UserConfigPath() string {
	for _, env := range []string{userConfigEnv, legacyUserConfigEnv} {
		if p := os.Getenv(env); p != "" {
			return p
		}
	}

	dir := filepath.Join(".config", "akita")
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, dir)
	}

	p := filepath.Join(dir, userConfigFile)
	if !fileExists(p) {
		if legacy := filepath.Join(dir, legacyUserConfigFile); fileExists(legacy) {
			return legacy
		}
	}

	return p
}

// UserConfigFromEnv reports whether $AKITA_HERD_CONFIG or
// $AKITA_EXP_CONFIG names the user config.
func UserConfigFromEnv() bool {
	return os.Getenv(userConfigEnv) != "" || os.Getenv(legacyUserConfigEnv) != ""
}

// defaultUserConfig is the config used when no user config exists: the
// local host only, default rules.
func defaultUserConfig() *UserConfig {
	cfg := &UserConfig{Hosts: []HostConfig{{Name: HostLocal, Type: HostLocal}}}
	cfg.applyDefaults()

	return cfg
}

// LoadUserConfig reads the user config (see UserConfigPath) and fills in
// defaults. A missing file yields defaultUserConfig.
func LoadUserConfig() (*UserConfig, error) {
	return loadUserConfigFile(UserConfigPath())
}

// loadUserConfigFile reads a user config from path and fills in defaults.
// A missing file yields defaultUserConfig.
func loadUserConfigFile(path string) (*UserConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultUserConfig(), nil
	}

	if err != nil {
		return nil, err
	}

	cfg := &UserConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	cfg.applyDefaults()

	return cfg, nil
}

func (c *UserConfig) validate() error {
	seen := map[string]bool{}

	for _, h := range c.Hosts {
		if h.Name == "" || strings.ContainsAny(h.Name, "/ \t") {
			return fmt.Errorf("bad host name %q", h.Name)
		}

		if seen[h.Name] {
			return fmt.Errorf("duplicate host %q", h.Name)
		}

		seen[h.Name] = true

		switch h.Type {
		case HostLocal, HostSSH, HostSlurm, "":
		default:
			return fmt.Errorf("host %s: unknown type %q", h.Name, h.Type)
		}

		if h.Type == HostSSH && h.Address == "" {
			return fmt.Errorf("host %s: ssh host needs an address", h.Name)
		}
	}

	return nil
}

func (c *UserConfig) applyDefaults() {
	setDefault(&c.StallMinutes, defaultStallMinutes)
	setDefault(&c.IdleCPU, defaultIdleCPU)
	setDefault(&c.SimStallMinutes, defaultSimStallMinutes)
	setDefault(&c.WorkStallMinutes, defaultWorkStallMinutes)
	setDefault(&c.Interval, defaultInterval)

	for i := range c.Hosts {
		h := &c.Hosts[i]
		if h.Type == "" {
			h.Type = HostLocal
		}

		if h.Root == "" && h.Type != HostLocal {
			h.Root = DefaultRoot
		}
	}
}

func setDefault(v *int, def int) {
	if *v <= 0 {
		*v = def
	}
}

// Rules returns the monitor's hang rules.
func (c *UserConfig) Rules() Rules {
	return Rules{
		StallMinutes:     c.StallMinutes,
		IdleCPU:          c.IdleCPU,
		SimStallMinutes:  c.SimStallMinutes,
		WorkStallMinutes: c.WorkStallMinutes,
	}
}

// Host returns the host with the given name.
func (c *UserConfig) Host(name string) (HostConfig, bool) {
	for _, h := range c.Hosts {
		if h.Name == name {
			return h, true
		}
	}

	return HostConfig{}, false
}
