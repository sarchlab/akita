// Package herd holds the documents of Herd (akita herd): the user and
// project configs, result directory naming, run records, and the JSON
// types the probe and the web UI exchange. It is the bottom of the tool;
// workspace, backend, probe, and monitor build on it. See DESIGN.md.
package herd

import (
	"path/filepath"
	"strings"
	"time"
)

// Schema identifiers of the JSON documents this package reads and writes.
const (
	SchemaProject = "herd-project/1"
	SchemaRun     = "herd-run/1"
	SchemaStatus  = "herd-status/1"
	SchemaCells   = "herd-cells/1"
	SchemaProbe   = "herd-probe/1"
)

// legacySchemaPrefix and schemaPrefix start the schema identifiers of the
// tool's former name (akita exp: "exp-run/1", ...) and of the current one.
const (
	legacySchemaPrefix = "exp-"
	schemaPrefix       = "herd-"
)

// NormalizeSchema maps a schema identifier of the former name
// ("exp-<doc>/<version>") to the current one ("herd-<doc>/<version>"), so
// that documents written before the rename are still read. Other
// identifiers are returned unchanged.
func NormalizeSchema(s string) string {
	if rest, ok := strings.CutPrefix(s, legacySchemaPrefix); ok {
		return schemaPrefix + rest
	}

	return s
}

// Host types.
const (
	HostLocal = "local"
	HostSSH   = "ssh"
	HostSlurm = "slurm"
)

// ID prefixes of progress bars: an Akita monitor's bar is
// "akita:<port>:<id>", a campaign's "campaign:<run>".
const (
	ProgressPrefixAkita    = "akita:"
	ProgressPrefixCampaign = "campaign:"
)

// UnixSeconds is a time as the documents carry it.
func UnixSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e9
}

// Property is one entry of run metadata, the shape of Akita's exec_info.
type Property struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// Progress is a progress bar. It carries the fields of
// monitoring.ProgressBar; ID is a string and StartTime Unix seconds so that
// bars from heartbeats, campaigns, and live monitors share one shape.
type Progress struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	StartTime  *float64 `json:"start_time"`
	Total      *uint64  `json:"total"`
	Finished   uint64   `json:"finished"`
	InProgress uint64   `json:"in_progress"`
}

// HostConfig is one entry of the user config's host list.
type HostConfig struct {
	Name    string       `json:"name"`
	Type    string       `json:"type"` // HostLocal, HostSSH, or HostSlurm
	Address string       `json:"address,omitempty"`
	Root    string       `json:"root,omitempty"` // default "~/exp" (ssh, slurm)
	Slurm   *SlurmConfig `json:"slurm,omitempty"`
	// Akita is the path of the akita binary on the host (default "akita";
	// "~/" is expanded on the host).
	Akita string `json:"akita,omitempty"`
	// Tmux, when false, runs sessions under nohup setsid even where tmux
	// is installed. Default true.
	Tmux *bool `json:"tmux,omitempty"`
	// Queue holds the settings of the host's agent.
	Queue *QueueConfig `json:"queue,omitempty"`
}

// QueueConfig holds the settings of a host's agent; zero means the
// default.
type QueueConfig struct {
	Slots        int      `json:"slots,omitempty"`          // default: CPUs / 2
	MemReserveGB float64  `json:"mem_reserve_gb,omitempty"` // default 4
	MemPauseGB   float64  `json:"mem_pause_gb,omitempty"`   // default 2
	MemResumeGB  float64  `json:"mem_resume_gb,omitempty"`  // default 6
	AvoidSignals []string `json:"avoid_signals,omitempty"`
}

// SlurmConfig holds the sbatch options of a SLURM host.
type SlurmConfig struct {
	Partition string   `json:"partition,omitempty"`
	Account   string   `json:"account,omitempty"`
	Time      string   `json:"time,omitempty"`
	CPUs      int      `json:"cpus,omitempty"`
	Mem       string   `json:"mem,omitempty"`
	Extra     []string `json:"extra,omitempty"` // more "#SBATCH" options
}

// UserConfig is ~/.config/akita/herd.json.
type UserConfig struct {
	Hosts []HostConfig `json:"hosts"`
	// Monitor rules; zero means the default.
	StallMinutes     int `json:"stall_minutes,omitempty"`      // default 30
	IdleCPU          int `json:"idle_cpu,omitempty"`           // default 5 (%)
	SimStallMinutes  int `json:"sim_stall_minutes,omitempty"`  // default 10
	WorkStallMinutes int `json:"work_stall_minutes,omitempty"` // default 30
	Interval         int `json:"interval,omitempty"`           // default 10 (s)
}

// NamingField is one field of a result directory name.
type NamingField struct {
	Name             string            `json:"name"`
	Label            string            `json:"label,omitempty"`
	Pattern          string            `json:"pattern,omitempty"`
	Values           map[string]string `json:"values,omitempty"` // value -> display label
	Not              []string          `json:"not,omitempty"`
	VariantSeparator string            `json:"variant_separator,omitempty"`
}

// Naming is the result directory naming rule of a project.
type Naming struct {
	Separator string        `json:"separator"`
	Fields    []NamingField `json:"fields"`
	GroupBy   []string      `json:"group_by,omitempty"`
	CompareBy string        `json:"compare_by,omitempty"`
	Legacy    string        `json:"legacy,omitempty"`
}

// SimulatorSpec tells how to recognise simulator processes.
type SimulatorSpec struct {
	Executable string `json:"executable"`
	LabelFlag  string `json:"label_flag,omitempty"`
	OutputFlag string `json:"output_flag,omitempty"`
}

// Matches tells whether a command line is one of the spec's simulators:
// the executable's base name contains Executable and, with a LabelFlag,
// the flag has a value (as probe reads it).
func (s SimulatorSpec) Matches(args []string) bool {
	if s.Executable == "" || len(args) == 0 || !strings.Contains(filepath.Base(args[0]), s.Executable) {
		return false
	}

	if s.LabelFlag == "" {
		return true
	}

	for i, a := range args {
		if strings.HasPrefix(a, s.LabelFlag+"=") || (a == s.LabelFlag && i+1 < len(args)) {
			return true
		}
	}

	return false
}

// CampaignAdapter tells how to read the cells of a campaign.
type CampaignAdapter struct {
	Type    string   `json:"type"` // "runs" (default) or "command"
	Command []string `json:"command,omitempty"`
}

// CampaignConfig tells which runs are campaigns and how to read them.
type CampaignConfig struct {
	Detect  []string        `json:"detect,omitempty"`
	Adapter CampaignAdapter `json:"adapter"`
}

// PlotSpec is a plot kind offered for campaigns.
type PlotSpec struct {
	Name          string   `json:"name"`
	Label         string   `json:"label,omitempty"`
	Kind          string   `json:"kind"` // "ratio" or "sweep"
	Metric        string   `json:"metric"`
	LowerIsBetter bool     `json:"lower_is_better,omitempty"`
	X             string   `json:"x"`
	Series        string   `json:"series"`
	Baseline      string   `json:"baseline,omitempty"`
	Match         []string `json:"match,omitempty"`
	Summary       string   `json:"summary,omitempty"` // "gmean" or ""
}

// SyncConfig holds rsync options.
type SyncConfig struct {
	Exclude []string `json:"exclude,omitempty"`
}

// BinarySpec is a binary that `akita herd build` builds: Command runs in
// Dir (relative to the project) with Env, "{out}" replaced by the output
// path.
type BinarySpec struct {
	Name    string            `json:"name"`
	Dir     string            `json:"dir,omitempty"`
	Command []string          `json:"command"`
	Env     map[string]string `json:"env,omitempty"`
}

// BuildConfig lists the binaries of a project.
type BuildConfig struct {
	Binaries []BinarySpec `json:"binaries"`
}

// CollectConfig holds the rsync excludes of `akita herd collect`.
type CollectConfig struct {
	Exclude []string `json:"exclude,omitempty"`
}

// ProjectConfig is a project's herd.json.
type ProjectConfig struct {
	Schema     string          `json:"schema"`
	Sync       SyncConfig      `json:"sync,omitzero"`
	Build      *BuildConfig    `json:"build,omitempty"`
	Collect    *CollectConfig  `json:"collect,omitempty"`
	Naming     *Naming         `json:"naming,omitempty"`
	Simulators []SimulatorSpec `json:"simulators,omitempty"`
	Campaigns  *CampaignConfig `json:"campaigns,omitempty"`
	Plots      []PlotSpec      `json:"plots,omitempty"`
}

// GPU is one GPU of a host.
type GPU struct {
	Name       string  `json:"name"`
	Util       float64 `json:"util"`
	MemUsedGB  float64 `json:"mem_used_gb"`
	MemTotalGB float64 `json:"mem_total_gb"`
}

// Simulator is a simulator process found in a session.
type Simulator struct {
	PID      int     `json:"pid"`
	Label    string  `json:"bench"`    // e.g. the benchmark
	Config   string  `json:"config"`   // cell config, from the output name
	Campaign string  `json:"campaign"` // run directory it writes to
	Paused   bool    `json:"paused"`
	Etime    float64 `json:"etime"`
	CPU      float64 `json:"cpu"`
	RSSMB    float64 `json:"rss_mb"`
	Monitor  string  `json:"monitor,omitempty"` // Akita monitor URL
}

// Process is a process of a session's process tree.
type Process struct {
	PID     int     `json:"pid"`
	Etime   float64 `json:"etime"`
	CPUTime float64 `json:"cpu_time"`
	CPU     float64 `json:"cpu"`
	Stat    string  `json:"stat"`
	RSSMB   float64 `json:"rss_mb"`
	Args    string  `json:"args"`
}

// Health explains a running session's hang judgement.
type Health struct {
	CPUNow       *float64 `json:"cpu_now"`
	LowFor       *float64 `json:"low_for"`
	OutputIdle   bool     `json:"output_idle"`
	HangBasis    string   `json:"hang_basis"` // "cpu", "heartbeat", or "monitor"
	HangReason   string   `json:"hang_reason,omitempty"`
	SimRate      *float64 `json:"sim_rate"`
	WGRate       *float64 `json:"wg_rate"`
	ETA          *float64 `json:"eta_s"`
	SimStillFor  *float64 `json:"sim_still_for"`
	WorkStillFor *float64 `json:"work_still_for"`
	HeartbeatAge *float64 `json:"heartbeat_age"`
}

// Session is one launched command.
type Session struct {
	ID         string         `json:"id"` // host/project/name
	Host       string         `json:"host"`
	Project    string         `json:"project"`
	Name       string         `json:"name"`
	Group      string         `json:"group,omitempty"`
	Archived   bool           `json:"archived"`
	State      string         `json:"state"` // running stalled ok failed died stopped queued
	ExitCode   *int           `json:"exit_code"`
	Start      *float64       `json:"start"`
	End        *float64       `json:"end"`
	MTime      float64        `json:"mtime"`
	LastOutput float64        `json:"last_output"`
	LogSize    int64          `json:"log_size"`
	Cmd        string         `json:"cmd"`
	Job        string         `json:"job,omitempty"` // slurm job id or pid
	Tail       []string       `json:"tail"`
	Progress   []Progress     `json:"progress"`
	Sims       []Simulator    `json:"sims"`
	Procs      []Process      `json:"procs"`
	Health     Health         `json:"health"`
	Heartbeat  map[string]any `json:"heartbeat,omitempty"`
}

// Run is one result directory.
type Run struct {
	ID         string             `json:"id"` // host/project/name
	Host       string             `json:"host"`
	Project    string             `json:"project"`
	Name       string             `json:"name"`
	Archived   bool               `json:"archived"`
	MTime      float64            `json:"mtime"`
	Kind       string             `json:"kind"` // single campaign sensitivity
	Fields     map[string]string  `json:"fields"`
	Properties []Property         `json:"properties"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Progress   *Progress          `json:"progress"`
	Failed     *int               `json:"failed"`
	Failures   []string           `json:"failures"`
	ExitCode   *int               `json:"exit_code"`
	Summary    string             `json:"summary,omitempty"`
	Files      []string           `json:"files"`
	Deviation  []string           `json:"deviation"`
}

// HostStatus is a host as the monitor sees it.
type HostStatus struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	OK         bool         `json:"ok"`
	Error      string       `json:"error,omitempty"`
	Now        float64      `json:"now,omitempty"`
	Load       []float64    `json:"load,omitempty"`
	CPUs       int          `json:"cpus,omitempty"`
	MemTotalGB float64      `json:"mem_total_gb,omitempty"`
	MemAvailGB float64      `json:"mem_avail_gb,omitempty"`
	GPUs       []GPU        `json:"gpus,omitempty"`
	FetchS     float64      `json:"fetch_s,omitempty"`
	Queue      *QueueStatus `json:"queue,omitempty"`
}

// Rules are the monitor's hang rules.
type Rules struct {
	StallMinutes     int `json:"stall_minutes"`
	IdleCPU          int `json:"idle_cpu"`
	SimStallMinutes  int `json:"sim_stall_minutes"`
	WorkStallMinutes int `json:"work_stall_minutes"`
}

// Status is the monitor snapshot served at /api/v1/status.
type Status struct {
	Schema   string                    `json:"schema"`
	Updated  float64                   `json:"updated"`
	Interval int                       `json:"interval"`
	Rules    Rules                     `json:"rules"`
	Projects map[string]*ProjectConfig `json:"projects"`
	Hosts    []HostStatus              `json:"hosts"`
	Sessions []Session                 `json:"sessions"`
	Runs     []Run                     `json:"runs"`
	// Activity is what the balance loop of `akita herd web --balance` did,
	// oldest first.
	Activity []string `json:"activity,omitempty"`
}

// AgentInfo is a host agent's agent.json.
type AgentInfo struct {
	PID          int      `json:"pid"`
	StartTick    uint64   `json:"start_tick,omitempty"`
	Host         string   `json:"host"`
	Version      string   `json:"version,omitempty"`
	Started      float64  `json:"started"`
	Heartbeat    float64  `json:"heartbeat"`
	Stopped      float64  `json:"stopped,omitempty"`
	Slots        int      `json:"slots"`
	Own          int      `json:"own"`      // the agent's running tasks
	External     int      `json:"external"` // simulator processes found
	MemAvailGB   float64  `json:"mem_avail_gb"`
	MemReserveGB float64  `json:"mem_reserve_gb"`
	MemPauseGB   float64  `json:"mem_pause_gb"`
	MemResumeGB  float64  `json:"mem_resume_gb"`
	AvoidSignals []string `json:"avoid_signals,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// QueueStatus is the queue of a host root: the agent, the pending tasks
// in order, the running ones, and the most recently ended ones.
type QueueStatus struct {
	Root    string     `json:"root"`
	Now     float64    `json:"now"`
	Agent   *AgentInfo `json:"agent"`
	Alive   bool       `json:"alive"` // the agent holds its lock
	Pending []Task     `json:"pending"`
	Running []Task     `json:"running"`
	Done    []Task     `json:"done"`
}

// HostProbe is what `akita herd probe --json` prints for one machine.
type HostProbe struct {
	Schema     string                    `json:"schema"`
	Now        float64                   `json:"now"`
	Load       []float64                 `json:"load"`
	CPUs       int                       `json:"cpus"`
	MemTotalGB float64                   `json:"mem_total_gb"`
	MemAvailGB float64                   `json:"mem_avail_gb"`
	GPUs       []GPU                     `json:"gpus"`
	Projects   map[string]*ProjectConfig `json:"projects"`
	Sessions   []Session                 `json:"sessions"` // Host empty; health not judged
	Runs       []Run                     `json:"runs"`     // Host empty
	Queue      *QueueStatus              `json:"queue,omitempty"`
}

// Cell is one simulation of a campaign.
type Cell struct {
	Key     map[string]string  `json:"key"`
	Status  string             `json:"status"` // ok failed running missing bad
	Reason  string             `json:"reason,omitempty"`
	Metrics map[string]float64 `json:"metrics"`
	Command []string           `json:"command,omitempty"`
}

// SweepPoint is one point of a sensitivity dimension.
type SweepPoint struct {
	Value     *float64          `json:"value"`
	Label     string            `json:"label"`
	Cell      map[string]string `json:"cell"`
	Reference bool              `json:"reference"`
}

// SweepDimension is one swept parameter.
type SweepDimension struct {
	Name   string       `json:"name"`
	Label  string       `json:"label"`
	Unit   string       `json:"unit,omitempty"`
	Points []SweepPoint `json:"points"`
}

// Sweep describes a sensitivity campaign.
type Sweep struct {
	Dimensions []SweepDimension `json:"dimensions"`
}

// CampaignCells is what a campaign adapter produces.
type CampaignCells struct {
	Schema     string                       `json:"schema"`
	Dimensions []string                     `json:"dimensions"`
	Labels     map[string]map[string]string `json:"labels,omitempty"`
	Order      map[string][]string          `json:"order,omitempty"`
	Cells      []Cell                       `json:"cells"`
	Sweep      *Sweep                       `json:"sweep"`
	Total      int                          `json:"total"`
	Warnings   []string                     `json:"warnings"`
	Binary     string                       `json:"binary,omitempty"`
	Meta       map[string]any               `json:"meta,omitempty"`
}

// RunDetail is one result directory in detail (/api/v1/run).
type RunDetail struct {
	Host       string            `json:"host,omitempty"`
	Project    string            `json:"project"`
	Name       string            `json:"name"`
	Fields     map[string]string `json:"fields"`
	Properties []Property        `json:"properties"`
	Record     *RunRecord        `json:"record"`  // run.json, or null
	Meta       map[string]any    `json:"meta"`    // meta.json, or null
	Summary    *string           `json:"summary"` // summary.csv, or null
	Files      []string          `json:"files"`
	Config     *ConfigUsed       `json:"config,omitempty"`  // config_used.json
	Metrics    *StatsDigest      `json:"metrics,omitempty"` // stats/metrics.csv
	// Finished: the run has ended (meta.json exit_code or an "End Time"
	// property), so the detail will not change.
	Finished bool `json:"finished"`
}

// ConfigUsed digests a config_used.json.
type ConfigUsed struct {
	Files       any      `json:"files"`
	Flags       any      `json:"flags"`
	CommandLine []string `json:"command_line"`
}

// StatsDigest summarises stats/metrics.csv (rows "_,where,what,value"):
// sums per component kind ("GPU[3].CU[1]" -> "GPU.CU") and metric, and a
// fingerprint of all rows to tell identical results apart.
type StatsDigest struct {
	Count       int                `json:"count"`
	Fingerprint string             `json:"fingerprint"`
	Agg         map[string]StatAgg `json:"agg"`
	Nonfinite   map[string]int     `json:"nonfinite"`
}

// StatAgg aggregates one metric over the components of a kind.
type StatAgg struct {
	Sum float64 `json:"sum"`
	N   int     `json:"n"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}
