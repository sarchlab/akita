// Shapes of /api/v1 (see DESIGN.md). Optional fields are ones older servers may omit.

export interface ProgressBar {
  id: string | number;
  name: string;
  start_time?: number | null;
  total: number | null;
  finished: number;
  in_progress: number;
}

export interface Property {
  property: string;
  value: string;
}

export interface NamingField {
  name: string;
  label?: string;
  pattern?: string;
  values?: Record<string, string>;
  variant_separator?: string;
  not?: string[];
}

export interface Naming {
  separator?: string;
  fields: NamingField[];
  group_by?: string[];
  compare_by?: string;
}

/** A plot kind offered for campaigns (herd.json "plots"). */
export interface PlotSpec {
  name: string;
  label?: string;
  kind: "ratio" | "sweep" | string;
  metric: string;
  lower_is_better?: boolean;
  x: string;
  series: string;
  baseline?: string;
  match?: string[];
  summary?: "gmean" | "" | string;
}

export interface SimulatorSpec { executable: string; label_flag?: string; output_flag?: string }

export interface ProjectConfig {
  schema?: string;
  naming?: Naming | null;
  simulators?: SimulatorSpec[] | null;
  campaigns?: { detect?: string[]; adapter?: { type: string; command?: string[] } } | null;
  plots?: PlotSpec[] | null;
}

// ---------- campaign cells (herd-cells/1) ----------

export interface CellDoc {
  key: Record<string, string>;
  status: string; // ok failed running missing bad
  reason?: string;
  metrics: Record<string, number> | null;
  command?: string[] | null;
}

export interface SweepPoint {
  value: number | null;
  label: string;
  /** Key values of the point's cells; null for a reference this campaign did not run. */
  cell: Record<string, string> | null;
  reference: boolean;
}

export interface SweepDimension { name: string; label: string; unit?: string; points: SweepPoint[] }

export interface CampaignCells {
  schema?: string;
  dimensions: string[];
  labels?: Record<string, Record<string, string>> | null;
  order?: Record<string, string[]> | null;
  cells: CellDoc[];
  sweep: { dimensions: SweepDimension[] } | null;
  total: number;
  warnings: string[] | null;
  binary?: string | null;
  meta?: Record<string, unknown> | null;
  // added by the server
  host?: string;
  project?: string;
  name?: string;
  fetched_at?: number;
  finished?: boolean | number;
  error?: string;
}

export interface Gpu {
  name?: string;
  util: number;
  mem_used_gb: number;
  mem_total_gb: number;
}

export interface Host {
  name: string;
  type?: "local" | "ssh" | "slurm" | string;
  ok: boolean;
  error?: string | null;
  now?: number;
  load?: number[];
  cpus?: number;
  mem_total_gb?: number;
  mem_avail_gb?: number;
  gpus?: Gpu[];
  fetch_s?: number;
  /** The host root's queue; absent when the root has none. */
  queue?: QueueStatus | null;
  /** The user's simulations that no Herd session started. */
  unmanaged?: Unmanaged[] | null;
}

/** A busy process of the user that no Herd session started (started by hand). */
export interface Unmanaged {
  pid: number;
  exe: string;
  args: string;
  cwd?: string;
  start: number;
  /** Average percent of a CPU over its life. */
  cpu: number;
  rss_gb: number;
  /** The command line of the script that started it, when there is one. */
  launcher?: string;
}

// ---------- adding hosts (herd-scan/1) ----------

export interface ScanProject { name: string; sessions: number; running: number; results: number }
export interface ScanRoot { root: string; queue: boolean; projects: ScanProject[] | null }

export interface HostScan {
  schema: string;
  hostname: string;
  user: string;
  os: string;
  arch: string;
  cpus: number;
  mem_total_gb: number;
  mem_avail_gb: number;
  load: number[] | null;
  tmux: boolean;
  roots: ScanRoot[] | null;
  unmanaged: Unmanaged[] | null;
  others_busy: number;
}

/** A host entry of the user config. */
export interface HostConfig {
  name: string;
  type: string;
  address?: string;
  root?: string;
  akita_dir?: string;
  queue?: { slots?: number; mem_reserve_gb?: number; mem_pause_gb?: number; mem_resume_gb?: number; avoid_signals?: string[] } | null;
}

/** What POST /api/v1/hosts/scan returns: the scan and the proposed entry. */
export interface HostScanResponse { scan: HostScan; host: HostConfig; taken: boolean; config: string }

// ---------- queue (herd-task/1) ----------

export interface Attempt { host: string; exit: number | null; signal?: string; started?: number; ended?: number | null }

export interface Task {
  project: string;
  name: string;
  group?: string;
  priority: number;
  mem_gb?: number;
  cmd: string;
  binaries?: Record<string, { sha: string; commit?: string; dirty?: boolean; diff?: string }> | null;
  attempts?: Attempt[] | null;
  avoid_hosts?: string[] | null;
  submitted: number;
  submitted_by?: string;
  host?: string;
  pid?: number;
  pgid?: number;
  started?: number;
  paused?: boolean;
  paused_at?: number;
  state?: string;
  exit?: number | null;
  signal?: string;
  ended?: number;
  error?: string;
  retryable?: boolean;
  retried?: string;
  collected?: number | null;
}

export interface AgentInfo {
  pid: number;
  host: string;
  version?: string;
  started: number;
  heartbeat: number;
  stopped?: number;
  slots: number;
  own: number;
  external: number;
  mem_avail_gb: number;
  mem_reserve_gb: number;
  mem_pause_gb: number;
  mem_resume_gb: number;
  avoid_signals?: string[] | null;
  error?: string;
}

export interface QueueStatus {
  root: string;
  now: number;
  agent: AgentInfo | null;
  alive: boolean;
  pending: Task[] | null;
  running: Task[] | null;
  done: Task[] | null;
}

export interface Sim {
  pid: number;
  bench: string;
  config: string;
  campaign: string;
  paused: boolean;
  etime: number;
  cpu: number;
  rss_mb: number;
  monitor?: string;
}

export interface Proc {
  pid: number;
  etime: number;
  cpu: number;
  rss_mb: number;
  stat: string;
  args: string;
}

export interface Health {
  cpu_now?: number | null;
  low_for?: number | null;
  output_idle?: boolean | null;
  hang_basis?: string | null;
  hang_reason?: string | null;
  sim_rate?: number | null;
  wg_rate?: number | null;
  eta_s?: number | null;
  sim_still_for?: number | null;
  work_still_for?: number | null;
  heartbeat_age?: number | null;
}

export interface Heartbeat {
  pid?: number;
  seq?: number;
  sim_time_ns?: number;
  wgs_completed?: number;
  finished?: boolean;
}

export type SessionState = "running" | "stalled" | "ok" | "failed" | "died" | "stopped" | "queued";

export interface Session {
  id: string;
  host: string;
  project: string;
  name: string;
  group: string | null;
  archived: boolean;
  state: SessionState;
  exit_code: number | null;
  start: number | null;
  end: number | null;
  last_output: number | null;
  mtime?: number | null;
  log_size?: number | null;
  cmd: string | null;
  job?: string | null;
  tail: string[];
  progress: ProgressBar[];
  sims: Sim[];
  procs: Proc[];
  health?: Health;
  heartbeat?: Heartbeat | null;
}

export type RunKind = "single" | "campaign" | "sensitivity";

export interface Run {
  id: string;
  host: string;
  project: string;
  name: string;
  archived: boolean;
  mtime: number | null;
  kind: RunKind;
  fields: Record<string, string> | null;
  properties: Property[];
  metrics?: Record<string, number> | null;
  progress: ProgressBar | null;
  failures: string[];
  failed?: number | null;
  exit_code: number | null;
  summary: string | null;
  files?: string[];
  deviation?: string[];
}

export interface Status {
  schema?: string;
  updated: number | null;
  interval: number | null;
  rules: {
    stall_minutes?: number;
    idle_cpu?: number;
    sim_stall_minutes?: number;
    work_stall_minutes?: number;
  };
  projects: Record<string, ProjectConfig | null> | null;
  hosts: Host[];
  sessions: Session[];
  runs: Run[];
  /** What the balance loop of `akita herd web --balance` did, oldest first. */
  activity?: string[] | null;
}

/** "host|project|name": the key the old page used for results and campaigns. */
export function runKey(r: { host: string; project: string; name: string }): string {
  return `${r.host}|${r.project}|${r.name}`;
}

export function keyParts(key: string): { host: string; project: string; name: string } {
  const [host = "", project = "", name = ""] = key.split("|");
  return { host, project, name };
}
