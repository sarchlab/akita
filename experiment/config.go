package experiment

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
const ProjectConfigFile = "exp.json"

// userConfigEnv names the environment variable that overrides the user
// config path.
const userConfigEnv = "AKITA_EXP_CONFIG"

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

// LoadProjectConfig reads <dir>/exp.json. It returns a nil config and a
// nil error when the file does not exist.
func LoadProjectConfig(dir string) (*ProjectConfig, error) {
	data, err := os.ReadFile(filepath.Join(dir, ProjectConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // a project without exp.json has no config
	}

	if err != nil {
		return nil, err
	}

	return parseProjectConfig(data)
}

// parseProjectConfig decodes an exp.json document and checks its naming
// rule.
func parseProjectConfig(data []byte) (*ProjectConfig, error) {
	cfg := &ProjectConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("exp.json: %w", err)
	}

	if cfg.Schema != "" && cfg.Schema != SchemaProject {
		return nil, fmt.Errorf("exp.json: unknown schema %q", cfg.Schema)
	}

	if cfg.Naming != nil {
		if _, err := compileNaming(cfg.Naming); err != nil {
			return nil, fmt.Errorf("exp.json: naming: %w", err)
		}
	}

	if err := cfg.Build.validate(); err != nil {
		return nil, fmt.Errorf("exp.json: build: %w", err)
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

// UserConfigPath returns $AKITA_EXP_CONFIG, or ~/.config/akita/exp.json.
func UserConfigPath() string {
	if p := os.Getenv(userConfigEnv); p != "" {
		return p
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "akita", "exp.json")
	}

	return filepath.Join(home, ".config", "akita", "exp.json")
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
