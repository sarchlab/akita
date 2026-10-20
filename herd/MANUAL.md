# Herd user manual

[中文版](MANUAL.zh-CN.md) · [Command and file reference](README.md) ·
[Design](DESIGN.md)

Herd (`akita herd`; picture the Akita dog herding a flock of simulations
across servers) runs your experiments on your own machine, on servers you
reach over SSH, or on SLURM clusters, and shows them all in one web page.
You keep editing code on your laptop; `akita herd` copies the project to a
host, starts the command in the background, and lets you watch, stop,
archive, and plot what runs there. Closing the laptop stops nothing.

This manual walks through Herd task by task. The [README](README.md)
lists every command, option, and file format in reference form.
To try Herd on one machine first, without writing code, follow the
[five-minute walkthrough](README.md#try-it-a-five-minute-walkthrough) in
the README.

## Contents

1. [Concepts](#1-concepts)
2. [Install](#2-install)
3. [A first experiment on your machine](#3-a-first-experiment-on-your-machine)
4. [Adding SSH servers and SLURM clusters](#4-adding-ssh-servers-and-slurm-clusters)
5. [Everyday work](#5-everyday-work)
6. [The web UI](#6-the-web-ui)
7. [Describing your project in herd.json](#7-describing-your-project-in-herdjson)
8. [Working with Akita simulators](#8-working-with-akita-simulators)
9. [Moving from akita exp or the old exp script](#9-moving-from-akita-exp-or-the-old-exp-script)
10. [Troubleshooting](#10-troubleshooting)

## 1. Concepts

| Term | Meaning |
|---|---|
| **Project** | A directory you work in, usually a git repository. You run `akita herd` from its root. Its name is the directory's name. |
| **Host** | A place where experiments run: `local` (this machine), `ssh` (a server), or `slurm` (a cluster). Hosts are listed in your user config. |
| **Workspace** | The copy of a project on a host: `<root>/<project>`, where `root` is `~/exp` by default. On the local host the workspace is the project directory itself. |
| **Session** | One command you started with `akita herd run`. It has a name, a log (`logs/<name>.log`), and a state such as `running` or `ok`. |
| **Group** | A label that ties sessions together, for example all runs of one study. |
| **Run** | One result directory, `results/<name>/`. It may hold a `run.json` record. |
| **Campaign** | A run that holds many simulations, typically workloads × configurations. The web UI can plot it. |

Herd never edits your code on a host by hand: every change goes
through your local project and `akita herd sync`.

## 2. Install

On your machine:

```bash
go install github.com/sarchlab/akita/v5/akita@latest
```

`akita herd` also needs `rsync` locally. Each SSH or SLURM host needs
`bash` and `rsync`; `tmux` is recommended (without it, sessions run under
`nohup setsid`).

You do not install anything else on the hosts. The monitor and the queue
run `akita herd` there, and the first time a host needs it, `akita herd`
copies the same akita there by itself (built for the host's platform
when it differs, which needs Go on your machine; the build is cached).
To use an akita you installed on a host instead, set the host's
`"akita"` key (section 4).

## 3. A first experiment on your machine

```bash
cd ~/my-simulator          # the project root
akita herd init             # writes ./herd.json and ~/.config/akita/herd.json
akita herd run local hello "echo hello; sleep 5; echo done"
akita herd status           # hello is running
akita herd log local hello  # the output so far
akita herd web              # opens http://127.0.0.1:8765
```

`init` writes a minimal project config and a user config with one host,
`local`. To start from a complete example instead, run `akita herd init
--example` in an empty directory; the README's [Adapting the example to
your project](README.md#adapting-the-example-to-your-project) says what
to replace. Each session's output goes to `logs/<name>.log`, framed by a
start line and an exit line:

```text
[start] Fri Oct 16 01:56:33 AM UTC 2026  local  3e9a41c     (date, host, git revision)
hello
done
[exit 0] Fri Oct 16 01:56:38 AM UTC 2026
```

Results are your program's business: write them under `results/<name>/`
and the web UI lists them.

## 4. Adding SSH servers and SLURM clusters

Hosts live in `~/.config/akita/herd.json` (or the file named by
`$AKITA_HERD_CONFIG`):

```json
{
  "hosts": [
    {"name": "local", "type": "local"},
    {"name": "lab1", "type": "ssh", "address": "lab1"},
    {"name": "big", "type": "ssh", "address": "me@big.example.edu",
     "root": "/scratch/me/exp", "akita": "~/bin/akita"},
    {"name": "cluster", "type": "slurm", "address": "login.cluster.example",
     "slurm": {"partition": "cpu", "account": "mylab", "time": "24:00:00",
               "cpus": 1, "mem": "16G", "extra": ["--gres=gpu:1"]}}
  ]
}
```

- **`address`** is anything `ssh` accepts. An alias from `~/.ssh/config`
  works, and is the easiest way to set user names, ports, and keys. Use
  key-based login: `akita herd` runs `ssh` without a password prompt.
- **`root`** holds the workspaces on that host (default `~/exp`). Put it on
  a disk with room for results.
- **`akita`** is the path of the `akita` program on the host.
- **`tmux: false`** runs sessions under `nohup setsid` even where tmux is
  installed.
- **SLURM hosts** submit each session as a batch job with the `slurm`
  options. A SLURM host without `address` uses the local `sbatch`, for
  when you run `akita herd` on the login node itself. A job waiting in the
  queue shows as `queued`.

Check the setup with `akita herd doctor`: for this machine and each host,
it checks ssh, `bash`, `rsync`, `tmux`, `sbatch`, that `akita` is found
and has the same version as yours, the root, and the agent, and says
what to fix.

## 5. Everyday work

### Starting experiments

```bash
akita herd run lab1 fir-base "./sim -benchmark=fir"
akita herd run -g ablation lab1 fir-opt "./sim -benchmark=fir -opt"
```

- `run` syncs the project first, so the host runs exactly your current
  code. To start many sessions quickly, sync once and skip the rest:
  `akita herd sync lab1`, then `HERD_NO_SYNC=1 akita herd run ...` (or
  `--no-sync`).
- `-g <group>` (or `HERD_GROUP=<group>`) puts the session in a group. Use one
  group per study; the web UI folds a group into one row and notifies you
  when the whole group has finished.
- Put options before the host. Everything after the session name is the
  command; quote it when it contains `;`, `|`, or `&&`.
- `{name}` in the command becomes the session name, so you write it only
  once: `akita herd run lab1 1019_fir_base ./run.sh {name}`. The session
  also gets it as `$HERD_NAME`.
- A name can be used again once the old session has ended. The new run
  overwrites the old log; archive the old session first if you want to
  keep it.

What `sync` copies: everything except `.git/`, `logs/`, `results/`,
`remote/`, `__pycache__/`, `.DS_Store`, `.herd_archive.json`, the paths
your `.gitignore` files ignore, `sync.exclude` in `herd.json`, and the
patterns in `.herdignore`. Those paths are also never deleted on the host,
so results there are safe.

### Watching and stopping

```bash
akita herd status            # every host; add -a for archived sessions
akita herd log lab1 fir-opt -f  # follow; Ctrl+C stops following only
akita herd stop lab1 fir-opt
akita herd wait                 # until nothing of the project runs
akita herd wait lab1 fir-opt    # until this one ends
```

`wait` prints each change of state, and with `--until running` or
`--until paused` waits for that state instead; use it in scripts
instead of `sleep`.

On a host, `tmux attach -t herd-<project>-<name>` shows a live session;
press Ctrl+B, then D, to leave it running.

Session states:

| State | Meaning |
|---|---|
| `running` | Still running. |
| `queued` | A SLURM job waiting to start. |
| `ok` / `failed` | Exited with status 0 / non-zero. |
| `stopped` | Stopped with `akita herd stop`. |
| `died` | Ended without an exit line, e.g. the host rebooted or SLURM hit the time limit. |
| `stalled` (web UI) | Running, but not making progress (see section 6). |

### Collecting results

`akita herd pull` copies `logs/` and `results/` of every host to
`remote/<host>/` in your project, for analysis on your machine.

### Queues: build, submit, balance, collect

When you have more runs than a server has cores or memory, queue them
instead of starting them with `run`. Each host then runs an *agent* that
starts your tasks as slots and memory free up, pauses tasks when memory
runs low, and keeps going while your laptop is closed.

1. Give each host a `queue` entry in the user config:

   ```json
   {"name": "lab1", "type": "ssh", "address": "lab1.example.edu",
    "queue": {"slots": 16, "mem_reserve_gb": 16, "mem_pause_gb": 8,
              "mem_resume_gb": 24, "avoid_signals": ["SIGBUS"]}}
   ```

   `slots` is how many tasks run at once (default: half the CPUs;
   simulators started outside the queue count too). A task starts only
   if MemAvailable minus its `--mem` stays above `mem_reserve_gb`. Below
   `mem_pause_gb` the agent pauses the newest task of the lowest priority
   (SIGSTOP); above `mem_resume_gb` it resumes it. `avoid_signals` marks
   signals that mean "this host is broken, try elsewhere".
2. Describe your binaries in `herd.json` (section 7) and build them:
   `akita herd build`. It builds the commit you are on, with your
   uncommitted changes, for Linux, and remembers the binary by its hash.
   Write `{bin:<name>}` in commands where the binary goes.
3. Submit:

   ```bash
   akita herd submit --host lab1 --mem 20 1019_diag_fir_base_a41f7e2 "./run.sh {name} {bin:sim}"
   akita herd submit --hosts lab1,lab2 -g round2 --each w=fir,mm,bfs --each c=base,opt \
     1019_diag_{w}_{c}_a41f7e2 ./run.sh {name} {bin:sim}
   akita herd submit --hosts lab1,lab2 -g round2 --file tasks.jsonl
   ```

   `--each key=v1,v2` submits one task per combination of the values
   (here 3 × 2), with `{key}` replaced in the name and the command.

   `submit` syncs the project, uploads the binary if the host lacks it,
   queues the task, and starts the host's agent if it is not running.
   With `--hosts`, each task goes to the host with the most room. A file
   has one JSON task per line: `{"name": ..., "cmd": ..., "mem_gb": 20,
   "priority": 3, "group": ...}`. A smaller priority starts first
   (default 5).
4. Watch: `akita herd queue` lists each host's agent, slots, memory,
   pending, running and paused tasks, and what ended. Running tasks are
   ordinary sessions, so `status`, `log`, `stop`, and the web UI work as
   usual. `akita herd cancel lab1 <name>` removes a task that has not
   started yet.
5. Rebalance: `akita herd balance` moves waiting tasks to hosts whose
   queue is empty and that have free slots, and resubmits tasks that died
   of an `avoid_signals` signal to another host (at most twice). `-n`
   only shows what it would do; `--watch` repeats every minute, and so
   does `akita herd web --balance`.
6. Collect: `akita herd collect` copies `results/<name>/` and the logs of
   every finished task to `remote/<host>/` once (leaving out
   `collect.exclude`); `--watch` keeps collecting.

The run's `run.json` gets the binary's hash and commit ("Binary", "Git
Revision"), so you can always tell which code produced a result.
`akita herd agent start|stop|status <host>` manages an agent by hand;
stopping it leaves the running tasks alone, and the next agent takes
them over.

### Tidying up: groups and archives

```bash
akita herd group lab1 ablation fir-base fir-opt  # add sessions to a group
akita herd archive -n lab1 ablation              # show what would happen
akita herd archive lab1 ablation --results 1015_abl_wg32k_base_3e9a41c
akita herd archive lab1                          # list what is archived
akita herd unarchive lab1 ablation               # undo
```

Archiving moves finished sessions' logs to `logs/archived/` and hides them
and their result directories in the web UI. It never moves or deletes
results. A session's result directories are those named in its command;
name others (for example campaigns that a script writes) with
`--results`. `archive` prints a hint with the directories that the logs
mention. Nothing changes while a named session is still running.

### Naming results

If your `herd.json` defines a naming rule (section 7), keep result names
consistent:

```bash
akita herd name check 1015_abl_wg32k_base_3e9a41c   # exit status 1 if wrong
akita herd name plan -H lab1 results                  # old -> new names
akita herd name apply -H lab1 results --dry-run
akita herd record results/1015_abl_wg32k_base_3e9a41c "Command=./sim -benchmark=fir"
```

`record` writes `run.json` in a result directory: the fields read from its
name plus the properties you give. `name apply` renames directories whose
names follow an old convention, using the mapping command you configure
(`naming.legacy`), and fixes the paths recorded inside them.

## 6. The web UI

`akita herd web` polls every host (or the hosts you name) and serves the
page on `http://127.0.0.1:8765` (`--port` to change it, `--no-browser` to
not open a browser). The page is read-only: it never starts, stops, or
changes an experiment. It only listens on this machine; to view it from
elsewhere, use SSH port forwarding.

**Language.** The page is in English or Chinese. It follows your browser's
language; the button at the top right (EN / 中文) switches and remembers
the choice. Add `?lang=en` or `?lang=zh` to a link to fix the language for
whoever opens it.

**Top bar.** The time of the last update (a small amber "stale" tag means
the data is old — a host may be slow or unreachable), one dot per host
with its type, the language button, and "Enable notifications" for
desktop notifications when a session ends, looks stalled, or a whole
group finishes.

**Overview.** Tiles with the number of running sessions and simulators,
sessions that look stalled, failures in the last 24 hours, and the
smallest free memory among the hosts. Below: one card per host (CPU load,
memory, GPUs), the running sessions with progress bars, and the
campaigns, unfinished ones first.

**Sessions.** All sessions with their state, host, start time, duration,
progress, CPU, memory, and last output. Filter by host, state, text,
time (last 24 hours), and archived; groups fold into one row. Click a row
for details: progress bars, the simulator processes, why the session is or
is not judged stalled, the command (with a copy button), and the end of
the log, which keeps scrolling as output arrives.

**Results.** On the left, search and filters built from your naming rule
(for example date, category, config, version), plus host and kind.
Directories are grouped by the fields in `group_by`, and each row shows
the value of `compare_by` as a coloured tag; the same value has the same
colour everywhere. Click a row for its properties (from `run.json`, the
run's SQLite files, and `meta.json`), its `summary.csv`, and its files.
Tick two to four rows to compare them side by side: key results with
ratios, configuration differences, and statistics differences.
"Compare similar" on a group header sends all campaigns of that group to
the charts.

**Charts.** Pick up to four campaigns. Three chart types:

- *Ablation*: each series value (e.g. each configuration) of one campaign
  against its baseline, per workload, with a geometric mean (GMEAN) over
  the workloads that every shown series has.
- *Compare*: one series value across campaigns, each against its own
  baseline (or one borrowed from another selected campaign).
- *Sensitivity*: a swept parameter on the x axis.

Options: which plot (when the project defines several), reference and
normalisation, a fixed baseline, a cap on the y axis, hiding series by
clicking the legend. Download a chart as SVG or its data as CSV, and copy
a link that reopens the same chart.

**Queue.** One card per host with a queue: whether its agent runs (and
its last heartbeat), slots in use (its own tasks, or the simulators found
on the host when there are more), free memory and the thresholds, then
the running tasks (paused ones in amber), the pending tasks in the order
they will start, and the tasks that ended, with their exit code, signal,
and whether they were retried or collected. With `akita herd web
--balance`, the page also lists what balance did.

**When a session counts as stalled.** If the program reports progress (an
Akita monitor or a heartbeat file, section 8), the session is stalled
when simulated time stops advancing for `sim_stall_minutes` (default 10),
the completed work stops growing for `work_stall_minutes` (default 30),
or the heartbeat is not updated for `sim_stall_minutes`. Otherwise, a CPU
use below `idle_cpu` % (default 5) for `stall_minutes` (default 30) means
stalled, and a busy process whose log and output files do not change for
`stall_minutes` is marked "no new output". Set these keys, and the polling
`interval` (default 10 s), in the user config.

## 7. Describing your project in herd.json

Every key is optional. Without `herd.json`, sessions and results are
still run and monitored, but result names are not split into fields,
simulator processes are not recognised, and campaigns are found by the
default markers (`EXPERIMENT_METADATA.json` or `campaign.json`), read with
the `runs` adapter, and plotted with a chart guessed from their cells.

```json
{
  "schema": "herd-project/1",
  "sync": {"exclude": ["data/", "*.ckpt"]},
  "naming": {
    "separator": "_",
    "fields": [
      {"name": "date", "label": "Date", "pattern": "\\d{4}"},
      {"name": "category", "label": "Category",
       "values": {"abl": "ablation", "sens": "sensitivity", "diag": "diagnosis"}},
      {"name": "item", "label": "Item", "pattern": "[A-Za-z0-9][A-Za-z0-9.-]*"},
      {"name": "config", "label": "Config", "pattern": "[a-z0-9][a-z0-9-]*(?:\\+[A-Za-z0-9.-]+)*",
       "variant_separator": "+"},
      {"name": "version", "label": "Version", "pattern": "[0-9a-f]{7}"}
    ],
    "group_by": ["category", "item"],
    "compare_by": "config"
  },
  "simulators": [
    {"executable": "mysim", "label_flag": "-benchmark", "output_flag": "-metric-file-name"}
  ],
  "campaigns": {
    "detect": ["campaign.json"],
    "adapter": {"type": "runs"}
  },
  "plots": [
    {"name": "speedup", "label": "Speedup", "kind": "ratio",
     "metric": "total_time", "lower_is_better": true,
     "x": "workload", "series": "config", "baseline": "baseline",
     "match": ["wgs"], "summary": "gmean"}
  ]
}
```

**`naming`** splits a result directory name into fields. With the rule
above, `1015_abl_wg32k_base+mshr128_3e9a41c` reads as date `1015`,
category `abl`, item `wg32k`, config `base+mshr128` (filtered as
`base`, the part before `variant_separator`), and version `3e9a41c`.
`pattern` is a regular expression for one field (no separator inside);
`values` is a closed list with display names; `not` lists forbidden
values. `group_by` says which fields make "the same experiment";
`compare_by` is the field that differs between members. A four-digit
field that is a valid month and day is shown as `MM-DD`. `legacy` names
a command that maps old names to new ones for `name apply`: it reads
names on stdin and prints `old<TAB>new` lines (empty `new` when unknown).

**`simulators`** tells the monitor which processes in a session are
simulators: `executable` is matched against the program's file name,
`label_flag` gives the label shown in the table (e.g. the benchmark), and
`output_flag` gives an output path whose directory names the campaign the
process writes to. That is how the UI counts running cells of a campaign.

**`campaigns`** marks which result directories are campaigns
(`detect`: file names that must exist in them) and how to read their
cells (`adapter`):

- `{"type": "runs"}`: each subdirectory with a `run.json` is a cell. Its
  `fields` are the cell's key and its `metrics` the numbers, e.g.

  ```json
  {"schema": "herd-run/1", "name": "fir_m1",
   "fields": {"workload": "fir", "config": "optA"},
   "metrics": {"total_time": 1.2e-5, "wgs": 512}, "status": "ok"}
  ```

  An optional `campaign.json` with `{"total": N}` lets the progress bar
  show the planned number of cells.
- `{"type": "command", "command": ["python3", "tools/cells.py"]}`: for
  campaigns in any other format. The command runs in the workspace on the
  host, with the campaign directory as its last argument, and prints the
  cells as JSON (schema `herd-cells/1`, see [DESIGN.md](DESIGN.md)):
  `dimensions`, optional `labels` and `order`, and `cells`, each with a
  `key`, a `status` (`ok`, `failed`, `running`, `missing`, `bad`), and
  `metrics`. A `sweep` section describes sensitivity campaigns.

**`plots`** turns cells into charts. For `kind: "ratio"`, each value is
baseline ÷ cell for a metric where lower is better (`lower_is_better`),
or cell ÷ baseline otherwise. The baseline is the cell whose `series` key
is `baseline` and whose other keys are equal; it is used only when every
metric in `match` is equal (for example, the same amount of work was
simulated). `summary: "gmean"` adds the geometric mean. `kind: "sweep"`
plots the same ratio over the points of a sweep.

**`build`** lists the binaries `akita herd build` makes: `name`, the
`dir` (relative to the project) the `command` runs in, and extra `env`.
`{out}` in the command is the output file; `GOOS`/`GOARCH` come from
`--target` (default `linux/amd64`).

```json
"build": {"binaries": [{"name": "sim", "dir": "cmd/sim",
            "command": ["go", "build", "-o", "{out}", "."],
            "env": {"CGO_ENABLED": "0"}}]},
"collect": {"exclude": ["*_trace.csv"]}
```

**`collect`** lists rsync patterns that `akita herd collect` leaves on the
host (large files you do not need locally).

## 8. Working with Akita simulators

`akita herd` needs nothing from your simulator, but Akita simulators give it
more to show:

- **Live progress.** If the simulator is built with a monitor
  (`sim.MakeBuilder().WithMonitor(monitoring.NewMonitor())`), it prints
  `Monitoring simulation with http://localhost:<port>`. `akita herd` finds
  that line in the session log and reads the monitor's progress bars and
  execution info, so the web UI shows the same bars as AkitaRTM and judges
  stalls by them. The simulator's own monitor page is linked from the
  session details.
- **Run metadata.** Akita writes `akita_sim_<id>.sqlite3` (or the name set
  with `WithOutputFileName`). Write it into the run's result directory and
  its `exec_info` table (command, start and end time, working directory)
  appears as the run's properties. The database is opened read-only.
- **Heartbeat files.** Any program can report progress without a monitor:
  `HERD_HEARTBEAT` holds the path of `logs/<name>.heartbeat`; rewrite it
  with a JSON object such as

  ```json
  {"pid": 12345, "wall": 1792111137.5, "finished": false,
   "sim_time_ns": 1.5e6,
   "progress": [{"id": "wg", "name": "Work-groups", "total": 32768, "finished": 1200, "in_progress": 64}]}
  ```

  `wall` is the Unix time of the update. Set `finished` to `true` at the
  end.

## 9. Moving from akita exp or the old exp script

Herd was called `akita exp` before the rename. Nothing needs migrating:
`akita exp ...` still works as an alias, and Herd reads the old names
when the new ones are missing: `~/.config/akita/exp.json` (or
`$AKITA_EXP_CONFIG`), a project's `exp.json`, schemas `exp-*/1`,
`.exp_archive.json`, `.expignore`, `EXP_GROUP` / `EXP_NO_SYNC`, and
sessions still running in tmux sessions named `exp-<project>-<name>`.
It writes only the new names (`herd.json`, `herd-*/1`,
`.herd_archive.json`, tmux `herd-<project>-<name>`). Rename the files
whenever you like; the [README](README.md#migrating-from-exp-and-akita-exp)
has the full table.

Herd also keeps the on-disk layout of the `exp` script, so existing
workspaces need no migration:

1. Install `akita` locally (section 2); it puts itself on the hosts.
2. List the old hosts in `~/.config/akita/herd.json`; the default root
   `~/exp` is the old one.
3. Use `akita herd` where you used `exp`. The commands and their options
   (`-g`, `-n`, `--results`, `EXP_GROUP` or `HERD_GROUP`, `EXP_NO_SYNC`
   or `HERD_NO_SYNC`) are the same.

Differences: commands without hosts act on every configured host; the
command runs in a subshell, so `exit N` is recorded and single quotes
work; `stop` writes `[stopped] <date>  akita herd stop` and refuses a
session that is not running; the monitor's thresholds are user config
keys; hosts need `akita` instead of `python3`; `logs/<name>.job` is new.

## 10. Troubleshooting

| Problem | What to check |
|---|---|
| A host shows an error in the web UI | Can you `ssh <address> true` without a password? Run `akita herd doctor <host>`: it says what is missing, and puts akita on the host when it can. |
| `run` says the session is already running | The name is in use. Pick another, or `stop` the old one. |
| A command works by hand but fails as a session | Sessions start a login shell (`bash -l`) in the workspace. Check paths relative to the project root and environment set only in interactive shells. |
| `sync` did not copy a file | It is ignored by `.gitignore`, `sync.exclude`, or `.herdignore`, or it is under `logs/`, `results/`, or `remote/`. |
| A session shows `died` | It ended without an exit line: the host rebooted, the process was killed, or SLURM ended the job (`status` shows the SLURM state). |
| The result list shows "unparsed" names | They do not match `naming`. `akita herd name check <name>` explains which field fails; `name plan` / `apply` can rename them. |
| A campaign has no chart | Is it detected (`campaigns.detect`)? Does the adapter print cells (`akita herd probe campaign ...` on the host)? Does `plots` name metrics the cells have? |
| `archive` changes nothing | A named session is still running, or `-n` was given. |
| A queued task does not start | `akita herd queue <host>`: is the agent running? Are the slots full (simulators started outside the queue count too)? Is MemAvailable minus its `mem_gb` below `mem_reserve_gb`, or is a task paused (nothing starts then)? Does the task avoid this host? The agent's log is `<root>/.akita-herd/agent.log` on the host. |
| `submit` says a binary has no build | Run `akita herd build` first; `{bin:<name>}` must name a binary of `herd.json` `build`. |
| `balance` moves nothing | Targets need a running agent, an empty queue, and free slots and memory; only the current project's tasks move. |
