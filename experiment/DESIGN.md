# akita exp: experiment runner and monitor

`akita exp` runs simulation experiments on the local machine, on SSH hosts,
or on SLURM clusters, and shows them in a web UI styled like AkitaRTM and
Daisen. It knows nothing about a particular simulator: what a result
directory name means, which processes are simulators, and how to read and
plot a campaign of runs all come from the project's `exp.json`.

## Concepts

- **Project**: a directory with an `exp.json` (optional). `akita exp`
  commands run from its root. The project name is the directory name.
- **Host**: where experiments run. Configured in the user config
  `~/.config/akita/exp.json` (or `$AKITA_EXP_CONFIG`):

  ```json
  {"hosts": [
    {"name": "local", "type": "local"},
    {"name": "server1", "type": "ssh", "address": "server1",
     "queue": {"slots": 16, "mem_reserve_gb": 16, "mem_pause_gb": 8,
               "mem_resume_gb": 24, "avoid_signals": ["SIGBUS"]}},
    {"name": "cluster", "type": "slurm", "address": "login.cluster.example",
     "slurm": {"partition": "cpu", "time": "24:00:00", "cpus": 1, "mem": "16G"}}
  ]}
  ```

  `address` is anything `ssh` accepts (an `~/.ssh/config` alias works).
  A SLURM host without `address` uses the local `sbatch`. `queue` holds
  the settings of the host's agent (see "Queue").
- **Workspace**: per host and project, `<root>/<project>/` where `root`
  defaults to `~/exp` on ssh/slurm hosts. For the local host the
  workspace is the project directory itself. Layout (unchanged from the
  original `exp` tool, so existing workspaces keep working):

  ```text
  <workspace>/
    <synced project files>          (ssh/slurm: rsync of the project)
    logs/<name>.log                 output; "[start] <date>  <host>  <cmd>",
                                    "[exit <code>] <date>", "[stopped] <date>  ..."
    logs/<name>.cmd                 the command
    logs/<name>.group               optional group name
    logs/<name>.heartbeat           optional heartbeat JSON (see below)
    logs/<name>.job                 slurm job id / local pid
    logs/archived/                  archived sessions (same files)
    results/<run>/                  result directories
    .exp_archive.json               {"groups": {...}, "sessions": {...},
                                     "results": {<run>: {"archived_at", "by": [...]}}}
  ```

- **Session**: one launched command (`akita exp run <host> <name> "<cmd>"`).
  Local and ssh sessions run in tmux (`exp-<project>-<name>`, `.` and `:`
  replaced by `_`); without tmux they run under `nohup setsid`. The
  mapping is not one-to-one (`x.y` and `x_y`; project `a` session `b-c`
  and project `a-b` session `c`), so every lookup uses the exact tmux
  session name, a new tmux session records its log in the tmux user
  option `@exp_log`, and `run` refuses a name whose tmux session belongs
  to another session; `stop` never kills another session's tmux session. SLURM
  sessions are batch jobs whose script writes the same log markers.
- **Run**: one result directory `results/<name>/`. Its record is
  `run.json` (below); a run may also hold Akita SQLite outputs
  (`*.sqlite3`), whose `exec_info` table is read as properties.
- **Campaign**: a run directory that holds many cells (one simulation per
  workload × configuration). Cells are produced by a campaign adapter.

## Files

### Project config `exp.json` (schema `exp-project/1`)

```json
{
  "schema": "exp-project/1",
  "sync": {"exclude": ["remote/", "results/", "logs/", "node_modules/"]},
  "naming": {
    "separator": "_",
    "fields": [
      {"name": "date", "label": "Date", "pattern": "\\d{4}"},
      {"name": "category", "label": "Category", "values": {"abl": "ablation"}},
      {"name": "item", "label": "Item", "pattern": "[A-Za-z0-9][A-Za-z0-9.-]*", "not": ["all"]},
      {"name": "config", "label": "Config", "pattern": "...", "variant_separator": "+"},
      {"name": "version", "label": "Version", "pattern": "[0-9a-f]{7}"}
    ],
    "group_by": ["category", "item"],
    "compare_by": "config",
    "legacy": "python3 tools/legacy_names.py"
  },
  "simulators": [
    {"executable": "sim", "label_flag": "-benchmark",
     "output_flag": "-metric-file-name"}
  ],
  "campaigns": {
    "detect": ["EXPERIMENT_METADATA.json"],
    "adapter": {"type": "command", "command": ["python3", "scripts/exp_cells.py"]}
  },
  "plots": [
    {"name": "speedup", "label": "Speedup", "kind": "ratio",
     "metric": "total_time", "lower_is_better": true,
     "x": "workload", "series": "config", "baseline": "baseline",
     "match": ["wgs"], "summary": "gmean"}
  ],
  "build": {"binaries": [{"name": "sim", "dir": "cmd/sim",
              "command": ["go", "build", "-o", "{out}", "."],
              "env": {"CGO_ENABLED": "0"}}]},
  "collect": {"exclude": ["trace.csv"]}
}
```

- `sync.exclude`: rsync excludes in addition to `.git/` and the project's
  `.gitignore`d paths.
- `naming.legacy` (optional): a command that maps old directory names to
  new ones for `akita exp name apply` (reads names on stdin, prints
  `old<TAB>new` lines; `new` empty when unknown).
- `simulators`: how to recognise simulator processes in a session's
  process tree: `executable` is matched against the executable's base
  name (substring); `label_flag` gives the row label (e.g. the benchmark);
  `output_flag` gives an output path whose parent directory names the
  run/campaign it writes to. Akita simulators started with a monitor are
  found from the log line `Monitoring simulation with http://localhost:<port>`.
- `campaigns.detect`: file names whose presence marks a run as a campaign.
  `campaigns.adapter`:
  - `{"type": "runs"}` (default): each subdirectory with a `run.json` is a
    cell; its `fields` are the cell key and its `metrics` the values.
  - `{"type": "command", "command": [...]}`: run in the workspace with the
    campaign directory appended as the last argument, on the host that has
    it; prints a `CampaignCells` JSON document (below).
- `plots`: plot kinds the UI offers for campaigns (see "Plots").
- `build`, `collect`: the binaries `akita exp build` builds and the rsync
  excludes of `akita exp collect` (see "Queue").

### Run record `run.json` (schema `exp-run/1`)

```json
{
  "schema": "exp-run/1",
  "name": "1015_abl_wg32k_base_3e9a41c",
  "fields": {"date": "1015", "category": "abl", "item": "wg32k",
             "config": "base", "version": "3e9a41c"},
  "properties": [{"property": "Command", "value": "..."}],
  "metrics": {"total_time": 1.2e-5}
}
```

Property names follow Akita's `exec_info` where one exists ("Command",
"Start Time", "End Time", "Working Directory"). `akita exp record <dir>
[PROP=VALUE ...]` writes or merges it; fields come from the directory name.

### Heartbeat `logs/<name>.heartbeat` (optional)

A JSON object rewritten by the running program:
`{"pid": 123, "wall": <unix s>, "finished": false, "sim_time_ns": 1.5e6,
  "progress": [ProgressBar...], "args": [...]}`. For compatibility,
`wgs_completed` and `-max-wg` in `args` are read as a "WG" progress bar.

## Campaign cells (schema `exp-cells/1`)

```json
{
  "schema": "exp-cells/1",
  "dimensions": ["workload", "variant", "config"],
  "labels": {"workload": {"fir": "FIR"}},
  "order": {"workload": ["fir", "..."], "config": ["baseline", "optA", "..."]},
  "cells": [
    {"key": {"workload": "fir", "variant": "", "config": "optA"},
     "status": "ok", "reason": "",
     "metrics": {"total_time": 1.2e-5, "wgs": 512},
     "command": ["-benchmark=fir", "..."]}
  ],
  "sweep": null,
  "total": 70, "warnings": [], "binary": "2f6d0c9a8b13", "meta": {}
}
```

- `status`: `ok | failed | running | missing | bad`.
- `sweep` (sensitivity campaigns): `{"dimensions": [{"name": "l2", "label":
  "L2 size", "unit": "MB", "points": [{"value": 2, "label": "2", "cell":
  {"variant": "l2-2"}, "reference": false}]}]}` — each point names the key
  values of its cells.

## Plots

A plot spec from `exp.json` turns cells into numbers:

- `kind: "ratio"`: for each `x` value and each `series` value, value =
  baseline metric / cell metric (`lower_is_better`) or cell / baseline,
  where baseline is the cell with `series == baseline` and the same other
  key values; computed only when every `match` metric is equal. `summary:
  "gmean"` adds a GMEAN over the `x` values every visible series has.
- Comparison of several campaigns: same plot, one series per campaign for
  a chosen `series` value; a campaign without a baseline borrows one from
  another selected campaign.
- `kind: "sweep"`: x = the points of a sweep dimension, series = `series`
  values, value = ratio as above against the reference point or the
  baseline.

The UI options of the original monitor (reference, normalisation, fixed
axis, cap, hidden series, SVG/CSV download, shareable `?plot=` links) apply
to every kind.

## API (`/api/v1`, JSON)

| Route | Returns |
|---|---|
| `GET /api/v1/status` | `Status` (below) |
| `GET /api/v1/run?host=&project=&name=` | `RunDetail` |
| `GET /api/v1/campaign?host=&project=&name=[&refresh=1]` | `CampaignCells` + `{host, project, name, fetched_at, finished}` |

`Status` (`exp-status/1`) is defined in `experiment/types.go`: `hosts`, `projects`
(name → project config), `sessions` (state, progress bars, simulators,
processes, health, log tail) and `runs` (fields, properties, kind,
progress, summary). A host whose root has a queue carries it as `queue`
(`QueueStatus`: `root`, `now`, `agent` (agent.json), `alive`, `pending`
in start order, `running`, and the 100 most recently ended tasks);
`activity` lists what the balance loop of `web --balance` did. Times are
Unix seconds.

## Probing

`akita exp probe --json [--root DIR]` scans the workspaces under a root on
the machine it runs on and prints a `HostProbe` (`experiment/types.go`),
with the root's queue when it has one. The monitor
runs it locally for the local host and via `ssh <address> akita exp probe`
for ssh/slurm hosts. A local host without a root has no root of its own
(its workspace is the project directory), so the monitor probes only the
project directory it was started in (hidden flag `--project <name>`,
relative to the parent directory), never its siblings, so `akita` must be installed on those hosts
(`go install github.com/sarchlab/akita/v5/akita@latest`). Session states:
`running` (tmux session / pid / slurm job alive), `ok` / `failed`
(`[exit N]`), `stopped` (`[stopped]`), `died` (none of these), plus
`stalled` when a running session's CPU stays below `idle_cpu` % for
`stall_minutes`, or its heartbeat/progress does not advance.

## Commands

```text
akita exp init                         write exp.json and the user config skeleton
akita exp sync [host...]               rsync the project to hosts
akita exp run [-g group] [--no-sync] <host> <name> "<cmd>"
akita exp status [host...]             sessions and recent log lines
akita exp log <host> <name> [-f]
akita exp stop <host> <name>
akita exp pull [host...]               logs/ and results/ to remote/<host>/
akita exp group <host> <group> <name>...
akita exp archive [-n] <host> <group|name>... [--results <run>...]
akita exp unarchive [-n] <host> <group|name>... [--results <run>...]
akita exp record <dir> [PROP=VALUE...]
akita exp name check <name>... | plan <results> | apply <results> [--dry-run]
akita exp web [--port 8765] [--no-browser] [--balance] [host...]
akita exp probe --json [--root DIR]    (used by the monitor)
akita exp build [--commit REV] [--target linux/amd64] [name...]
akita exp submit [-g group] [--priority N] [--mem GB] [--host H|--hosts H,H]
                 [--file tasks.jsonl] [--no-sync] <name> "<cmd>"
akita exp queue [host...]              agent, pending, running, paused, ended
akita exp cancel <host> <name>         pending only; a running task: stop
akita exp balance [--watch] [-n] [host...]
akita exp collect [--watch] [host...]
akita exp agent start|stop|status <host>
akita exp agent run --root DIR --host NAME [settings]   (started by agent start)
akita exp agent stop|status --root DIR                  (on the host)
akita exp queue put|take|list|mark --root DIR ...       (on the host; hidden)
```

## Queue

The workflow the queue automates: write code on the laptop, build a
simulator binary for the servers, run many simulations on several
servers (each limited by CPU slots and memory), move waiting work to
whichever server is idle, and bring finished results back to the laptop.

### Principles

- **The laptop is not needed for work to progress.** Each host runs an
  *agent* that owns that host's queue: it starts queued tasks when slots
  and memory allow, pauses and resumes under memory pressure, and records
  results. The laptop only submits, rebalances, and collects.
- **Everything is files under the root**, readable by `probe` and the
  monitor, and safe to inspect by hand. State changes are atomic renames
  under a lock, so the agent and the laptop never both own a task.
- **Only signal what we started.** Every process group the agent starts is
  recorded (pid = pgid, plus the leader's start time to tell a reused pid
  apart); pausing and resuming use those, never a name pattern. The agent
  never kills a task; `akita exp stop` does, by its `.job` file.
- **Coexistence.** Simulations started outside the queue still count
  against the host's slots: the agent counts the processes that match the
  `simulators` of the projects under its root (`SimulatorSpec.Matches`,
  as `probe` matches them) and uses `max(own tasks, simulators found)` as
  the slots in use.

### Host settings (`hosts[].queue`)

| Key | Meaning |
|---|---|
| `slots` | Concurrent tasks (default: CPUs / 2). A paused task holds its slot. |
| `mem_reserve_gb` | A task starts only if `MemAvailable - mem_gb >= mem_reserve_gb` (default 4). Tasks started in the last 60 s count with their `mem_gb` until they have allocated it. |
| `mem_pause_gb` | Below this MemAvailable, the agent pauses (SIGSTOP to the process group) the most recently started task of the lowest priority, one per round (default 2). |
| `mem_resume_gb` | Above this, it resumes (SIGCONT) the paused task that started first, one per round (default 6). No task starts while one is paused. |
| `avoid_signals` | A task killed by one of these signals on this host is *retryable* on another host (e.g. `SIGBUS` on a host with bad memory). |

The agent runs on the host, which does not read the user config: `agent
start` passes the settings as flags of `agent run`.

### Files on a host

```text
<root>/.akita-exp/
  agent.json      AgentInfo: pid, start_tick, host, settings, heartbeat,
                  own/external slots, MemAvailable; stopped when it exits
  agent.lock      flock held by the running agent (one per root)
  agent.log       what the agent did
  queue.lock      flock held by every change of queue/
  seq             the last sequence number
  queue/pending/<prio:04d>-<seq:08d>-<project>--<name>.json
  queue/running/<project>--<name>.json
  queue/done/<project>--<name>.json
  bin/<sha256[:12]>/<file>   uploaded binaries, by content
```

A task (`exp-task/1`, `experiment.Task`):

```json
{"schema": "exp-task/1", "project": "demo", "name": "1019_diag_x_base_a41f7e2",
 "group": "round2", "priority": 2, "mem_gb": 20,
 "cmd": "./run.sh ... {bin:sim}",
 "binaries": {"sim": {"sha": "3f2a...", "file": "sim", "commit": "a41f7e2",
                           "dirty": false, "target": "linux/amd64"}},
 "attempts": [{"host": "server1", "exit": 135, "signal": "SIGBUS", "started": 1760000000, "ended": 1760000100}],
 "avoid_hosts": ["server1"], "submitted": 1760000000, "submitted_by": "me@laptop", "seq": 12}
```

Running adds `host`, `pid`, `pgid`, `start_tick`, `started`, `paused`,
`paused_at`; ended adds `state` (`ok`, `failed`, `stopped`, `died`),
`exit`, `signal`, `ended`, `error` (a task that could not start),
`retryable`, `retried` (the host balance resubmitted it to), and
`collected`. A smaller `priority` starts first (default 5); pending tasks
start in file-name order. Putting a task fails if one of the same project
and name is pending or running there; an ended one is replaced.

### Agent

`akita exp agent run --root <root> --host <name>`, normally started by
`akita exp agent start <host>` in tmux session `exp-agent-<host>` (under
`nohup setsid` without tmux); `submit` starts it when it is not alive
(agent.json's process gone or heartbeat older than 60 s). It holds
`agent.lock` (one agent per root) and every 5 s:

1. Reads MemAvailable and counts the simulator processes.
2. Under `queue.lock`: **reaps** each running task whose process group is
   gone (or whose leader pid now belongs to another process): the exit
   comes from the log markers (`[exit N]`; N > 128 is signal N - 128),
   else from the wait status when this agent started it; `retryable` when
   the signal is in `avoid_signals`. It appends the attempt, adds the
   binaries to `results/<name>/run.json` ("Binary", "Git Revision") when
   that directory exists, and moves the task to `done/`. A running task
   without a pid (the previous agent died while starting it) is adopted
   from its `.job` file, or ends as `died` after 60 s.
3. **Memory guard** (pause or resume one task, as above).
4. **Starts** pending tasks in order, while slots allow, skipping tasks
   whose `avoid_hosts` names this host, and stopping at the first that
   does not fit the memory. A start replaces `{bin:<name>}` by the quoted
   path of the binary on this host (a missing binary fails the task),
   prepares the session's files as `run` does (`logs/<name>.cmd`,
   `.group`, an empty `.log`; refused while a session of that name is
   alive), moves the task to `running/`, starts the wrapper script under
   `setsid` (so pgid = pid) with `EXP_TASK=<project>--<name>`, and writes
   `logs/<name>.job` (`<pid>\npid\n`). A queued task is thus an ordinary
   session: `status`, `log`, `stop`, the monitor and `probe` work
   unchanged.
5. Writes `agent.json` (the heartbeat).

`akita exp agent stop <host>` sends SIGTERM to the pid in agent.json
(after checking its start time) and waits for it to exit; the tasks keep
running and the next agent adopts them.

### build

`akita exp build [--commit REV] [--target linux/amd64] [name...]` checks
the commit (default `HEAD`) out into a temporary git worktree; for `HEAD`
it applies `git diff HEAD --binary` and copies the untracked, not ignored
files up to 10 MB, and marks the build `dirty` with a hash of both. It
runs each binary's command in `<worktree>/<dir>` with `GOOS`/`GOARCH` of
the target, `GOFLAGS=-trimpath` (so the same source gives the same
binary) and the binary's `env`, `{out}` replaced by the output path. The
output is hashed and kept as `.akita-exp/bin/<sha12>/<name>` in the
project (which `.akita-exp/.gitignore` hides from git and `sync` never
copies); `.akita-exp/builds.json` records the latest build of each
binary. The binary is never part of `sync`, so a stale binary cannot be
shipped by accident.

### submit and placement

`submit` checks each name (also against `naming` when the project has
one), adds the latest build of each `{bin:<name>}` of the command, picks
a host per task, and for each chosen host syncs the project (unless
`--no-sync`), uploads the binaries the host lacks (`Backend.Upload`: rsync
to `bin/<sha12>/<file>`, skipped when the file exists), puts the tasks
with `akita exp queue put` there, and starts its agent if needed. With
`--hosts` (default: the hosts with a `queue` entry), a task goes to the
host with the highest score: the free slots (slots - used - pending),
limited by the number of such tasks the memory left after the reserve and
the pending tasks holds; ties go to the host with fewer pending tasks.
Placement and balance planning are pure functions (`queue.Place`,
`queue.PlanBalance`).

### balance

One round, for the project's tasks over the hosts (all by default):

1. Each ended task that is `retryable`, not yet `retried`, and has at
   most 2 attempts is resubmitted (with its host added to `avoid_hosts`)
   to the best other host whose agent runs; the old entry gets
   `retried`.
2. Pending tasks of hosts that cannot start them (free < 0) move, in queue
   order, to hosts whose agent runs, whose `pending/` was empty, and that
   have free slots and memory for them. A move is `queue take` on the
   source (an atomic removal under the lock; it fails if the agent started
   the task meanwhile), then sync, binary upload and `queue put` on the
   target; if that fails, the task is put back on the source.

`--watch` repeats every 60 s; `akita exp web --balance` runs the same
loop and shows its activity in the Queue view.

### collect

For each ended task of the project not yet `collected`, `collect` copies
`results/<name>/` (without `collect.exclude`) and
`logs/<name>.{cmd,group,heartbeat,job,log}` to `remote/<host>/` (as
`pull` does) and marks the task `collected`.

### Monitor

The **Queue** view shows per host: the agent (alive, pid, heartbeat age),
slots used / total (own and simulators found), MemAvailable and the
thresholds; the running tasks (paused ones marked), the pending tasks in
start order, and the recently ended ones (exit, signal, retryable /
retried, collected); and the balance activity of `web --balance`. It is
read-only. The queue comes with each host's probe (no extra ssh).

### Testing

Unit tests run no simulators: the queue files (order, duplicates, a race
of eight takers), placement and balance planning, the agent with a fake
system (slots, external simulators, memory reserve, pause and resume
order, reaping, signals, adoption, pid reuse, binaries and run.json), and
one round with real `true`, `sleep`, and `kill -BUS` processes. CLI tests
use two local roots as two hosts. Remote tests use only a test root
(never `~/exp`), with `sleep`, `true`, and small `python3` commands, and
`agent run --fake-mem-file` (a hidden flag: MemAvailable in GB is read
from that file) for the memory guard.

## Code layout

The Go code is split by responsibility; dependencies only point down the
list, and `akita/internal/cmd` (the CLI) sits on top:

| Package | Owns |
|---|---|
| `experiment/monitor` | the monitor: polling, the detail cache, the hang rules (`health.go`), the HTTP server and `/api/v1`, the embedded UI (`experiment/static`) |
| `experiment/probe` | reading a machine's workspaces without writing: sessions, process trees and simulators, heartbeat and Akita monitor bars, runs, run details, campaign cells, the `akita exp probe` arguments |
| `experiment/queue` | the queue of a root (`<root>/.akita-exp/`): its files and lock, put/take/mark, the agent and its `System` (the machine, replaceable in tests), placement and balance planning, and `build` (worktree builds and the project's build cache) |
| `experiment/backend` | reaching a host: the `Backend` interface, local/ssh/slurm, launch and stop scripts, starting akita in the background, rsync (sync, pull, a run's files, uploads) |
| `experiment/workspace` | the on-disk format: layout and file names, log markers and the wrapper script that writes them, a session's files before it starts, `.job` and heartbeat files, log reading, the liveness rule, `.exp_archive.json`, archive/unarchive/group, renaming result directories |
| `experiment` | the documents: user and project config, naming, `run.json`, tasks (`exp-task/1`), SQLite `exec_info`, and the JSON types of the probe and the API |
| `experiment/internal/hostsys` | the machine: bounded file reads and atomic writes, flock, commands with timeouts, tmux panes, processes (`/proc` on Linux) and process groups (start, signal, start time), SLURM job state, host statistics and MemAvailable |

`experiment/internal/testutil` holds test helpers shared by the packages.
