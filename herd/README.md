# Herd

New to Herd? Read the [user manual](MANUAL.md) ([中文](MANUAL.zh-CN.md)) first; this page is the reference.

Herd (`akita herd`; the Akita dog herding a flock of simulations across
servers) runs experiments on your machine, on SSH hosts, and on SLURM
clusters, and monitors them in a web UI. You edit code locally, `akita herd
run` copies the project to a host and starts a command there in the
background, and `akita herd pull` brings the logs and results back. Closing
your laptop does not stop anything that runs on a host.

Herd does not depend on any particular simulator. A project's
`herd.json` can say how its result directories are named, which processes
are simulators, and how to read and plot campaigns (see [DESIGN.md](DESIGN.md)).

## Install

```bash
go install github.com/sarchlab/akita/v5/akita@latest
```

Install `akita` on every SSH and SLURM host as well. `status`, `archive`,
`group`, and the web UI run `akita herd ...` there. If `akita` is not on
the host's `PATH` (`~/go/bin` and `~/.local/bin` are searched as well),
set `"akita"` in the host's config. A Linux binary can be cross-compiled
and copied to the host:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o akita-linux github.com/sarchlab/akita/v5/akita
scp akita-linux myhost:bin/akita
```

Hosts need `bash`. `tmux` is recommended; without it, sessions run
under `nohup setsid`. You also need `rsync` locally and on the hosts.

## User config

The user config is `~/.config/akita/herd.json`, or the file that
`$AKITA_HERD_CONFIG` names. `akita herd init` writes a skeleton with the
local host only. A config under the name of the tool before the rename
(`~/.config/akita/exp.json` or `$AKITA_EXP_CONFIG`) is still read when the
new one does not exist.

```json
{
  "hosts": [
    {"name": "local", "type": "local"},
    {"name": "server1", "type": "ssh", "address": "server1"},
    {"name": "lab", "type": "ssh", "address": "me@lab.example.edu",
     "root": "/scratch/me/exp", "akita": "~/bin/akita"},
    {"name": "cluster", "type": "slurm", "address": "login.cluster.example",
     "slurm": {"partition": "cpu", "account": "mylab", "time": "24:00:00",
               "cpus": 1, "mem": "16G", "extra": ["--gres=gpu:1"]}}
  ],
  "stall_minutes": 30, "idle_cpu": 5, "sim_stall_minutes": 10,
  "work_stall_minutes": 30, "interval": 10
}
```

| Key | Meaning |
|---|---|
| `name` | The name you use in commands (`akita herd run server1 ...`). |
| `type` | One of `local`, `ssh`, or `slurm`. |
| `address` | Anything `ssh` accepts; an `~/.ssh/config` alias works. A SLURM host without an address uses the local `sbatch`. |
| `root` | The directory that holds the workspaces: `<root>/<project>`. The default is `~/exp` on SSH and SLURM hosts. On the local host, the workspace is the project directory itself, unless you set a root. Then `sync` copies the project there. |
| `akita` | The path of `akita` on the host. The default is `akita`. A leading `~/` is expanded on the host. |
| `tmux` | `false` runs sessions under `nohup setsid` even where tmux is installed. The default is to use tmux when it is available. |
| `slurm` | `#SBATCH` options. `extra` holds more options, such as `--gres=gpu:1`. |
| `queue` | Settings of the host's queue agent: `slots`, `mem_reserve_gb`, `mem_pause_gb`, `mem_resume_gb`, and `avoid_signals`. See [Queues](#queues). |

The remaining keys are the monitor's hang rules and its polling
interval. A zero value means the default.

## Project config

`herd.json` sits at the project root (a project's `exp.json` from before
the rename is read when there is no `herd.json`). All of its keys are
optional:

```json
{
  "schema": "herd-project/1",
  "sync": {"exclude": ["data/", "*.ckpt"]},
  "naming": {
    "separator": "_",
    "fields": [
      {"name": "date", "pattern": "\\d{4}"},
      {"name": "category", "values": {"abl": "ablation", "diag": "diagnosis"}},
      {"name": "item", "pattern": "[A-Za-z0-9][A-Za-z0-9.-]*"},
      {"name": "config", "pattern": "[a-z0-9+-]+", "variant_separator": "+"},
      {"name": "version", "pattern": "[0-9a-f]{7}"}
    ],
    "group_by": ["category", "item"],
    "compare_by": "config",
    "legacy": "python3 tools/legacy_names.py"
  },
  "simulators": [{"executable": "mysim", "label_flag": "-benchmark",
                  "output_flag": "-metric-file-name"}],
  "campaigns": {"detect": ["EXPERIMENT_METADATA.json"], "adapter": {"type": "runs"}},
  "plots": [{"name": "speedup", "kind": "ratio", "metric": "total_time",
             "lower_is_better": true, "x": "workload", "series": "config",
             "baseline": "baseline", "summary": "gmean"}]
}
```

[DESIGN.md](DESIGN.md) describes every key.

`sync` always leaves out the following paths. It also never deletes them
on the host.

- `.git/`
- `logs/`, `results/`, and `remote/`
- `__pycache__/` and `.DS_Store`
- `.herd_archive.json` and `.exp_archive.json`

It also leaves out the paths that the project's `.gitignore` files
ignore, the patterns in `sync.exclude`, and the lines of `.herdignore`
and `.expignore` (one rsync exclude pattern per line).

## Commands

Run these commands from the project root. Commands that take `[host...]`
act on every configured host when you name none.

| Command | What it does |
|---|---|
| `akita herd init` | Writes `./herd.json` and the user config if they are missing. |
| `akita herd sync [host...]` | Copies the project to `<root>/<project>` with `rsync --delete`, so the copy matches the project. Files you delete locally are deleted on the host too, except for the paths listed above. |
| `akita herd run [-g group] [--no-sync] <host> <name> <command>...` | Syncs, then starts the command in the workspace as session `<name>`. Output goes to `logs/<name>.log`. `HERD_GROUP` sets the default group, and `HERD_NO_SYNC=1` skips the sync. Put the flags before `<host>`. Everything after `<name>` is the command. |
| `akita herd status [-a] [-n lines] [host...]` | Lists sessions with their state, group, start time, and duration, followed by the last log lines of the most recent ones. `-a` also lists archived sessions. |
| `akita herd log <host> <name> [-f] [-n 50]` | Shows the end of a log; archived logs work too. With `-f`, Ctrl+C stops following, not the session. |
| `akita herd stop <host> <name>` | Appends `[stopped] <date>  akita herd stop` to the log, then kills the tmux session or process group, or runs `scancel` for a SLURM job. |
| `akita herd pull [host...]` | Copies `logs/` (including `logs/archived/`) and `results/` to `remote/<host>/`. |
| `akita herd group <host> <group\|-> <name>...` | Puts sessions into a group. `-` removes them from their group. |
| `akita herd archive [-n] <host> [<group\|name>...] [--results <run>...]` | Archives finished sessions; see [Archiving](#archiving). With only `<host>`, lists what is archived there. |
| `akita herd unarchive [-n] <host> <group\|name>... [--results <run>...]` | Undoes `archive`. |
| `akita herd record <dir> [PROPERTY=VALUE...]` | Writes or merges `<dir>/run.json`. Fields come from the directory name. |
| `akita herd name check <name>...` | Checks names against `naming`. Exits with status 1 if any name breaks the rule. |
| `akita herd name plan [-H host] <results>` | Prints `old<TAB>new` for every result directory, or `old<TAB>?` when no new name is known. |
| `akita herd name apply [-H host] <results> [--dry-run]` | Renames result directories; see [Renaming results](#renaming-results). |
| `akita herd web [--port 8765] [--no-browser] [--balance] [host...]` | Serves the monitor on `http://127.0.0.1:8765`. `--balance` also runs `balance` every minute. |
| `akita herd probe --json [--root DIR]` | Prints the workspaces of this machine as JSON. The monitor runs this command, as well as `probe run` and `probe campaign`. |
| `akita herd build [--commit REV] [--target linux/amd64] [name...]` | Builds the binaries of `herd.json` `build` in a temporary git worktree and keeps them in `.akita-herd/bin/<sha12>/`. |
| `akita herd submit [-g group] [--priority N] [--mem GB] [--host H\|--hosts H,H] [--file tasks.jsonl] <name> <command>...` | Queues tasks on hosts' queues; see [Queues](#queues). |
| `akita herd queue [host...]` | Shows each host's agent and its pending, running, paused, and ended tasks. |
| `akita herd cancel <host> <name>` | Removes a pending task. |
| `akita herd balance [--watch] [-n] [host...]` | Moves pending tasks to idle hosts and retries tasks that died of a host's `avoid_signals`. |
| `akita herd collect [--watch] [host...]` | Copies the results and logs of ended tasks to `remote/<host>/`. |
| `akita herd agent start\|stop\|status <host>` | Starts, stops, or shows a host's agent (tmux session `herd-agent-<host>`). Stopping it leaves its tasks running. |

### Sessions

On local and SSH hosts, a session runs in tmux session
`herd-<project>-<name>`, with `.` and `:` replaced by `_`. Use `tmux attach
-t herd-<project>-<name>` on the host to watch it, and press Ctrl+B, then D
to leave it running. Sessions started before the rename run in tmux
sessions named `exp-<project>-<name>`; Herd still finds, reports, and
stops them.

Because `.` and `:` become `_`, and `-` separates the project from the
name, two sessions can have the same tmux session name: `x.y` and `x_y`
in one project, or session `b-c` of project `a` and session `c` of project
`a-b`. Only one of them can run at a time: `run` refuses to start a
session whose tmux session name is in use by another session, and names
the other session's log. Pick a different name in that case.

Without tmux, the session runs under `nohup setsid`, and its pid is
stored in `logs/<name>.job` as `<pid>\npid\n`.

On SLURM hosts, the session is a batch job with the host's `#SBATCH`
options. Its job id is stored in `logs/<name>.job` as `<id>\nslurm\n`.
`status` shows the job as `queued` while it waits.

The log of every session has the same shape:

```text
[start] Fri Oct 16 01:56:33 AM UTC 2026  server2  3e9a41c     (date, host, git revision)
... output ...
[exit 0] Fri Oct 16 01:58:10 AM UTC 2026
```

A session can be in one of these states:

| State | Meaning |
|---|---|
| `running` | The session is still running. |
| `queued` | The SLURM job is waiting to start. |
| `ok` | The command exited with status 0. |
| `failed` | The command exited with a non-zero status. |
| `stopped` | `akita herd stop` stopped it. |
| `died` | It ended without an `[exit]` line, for example because the host rebooted or SLURM hit a time limit. In the time-limit case, `status` adds the final SLURM state. |

The command runs in a subshell, so `exit 3` is recorded as `[exit 3]`, and
you can use single quotes in it. `HERD_HEARTBEAT` (also exported as
`EXP_HEARTBEAT`, its name before the rename) points to
`logs/<name>.heartbeat`, which a program can rewrite with progress
information (see DESIGN.md).

### Queues

Each SSH or local host can have a queue under `<root>/.akita-herd/`, run
by an agent on the host (`akita herd agent start <host>`; `submit` starts
it when needed). The agent starts pending tasks, smallest `priority`
first, while slots are free (`slots`, default CPUs / 2; simulator
processes started outside the queue count too) and `MemAvailable -
mem_gb >= mem_reserve_gb`. When MemAvailable falls below `mem_pause_gb`,
it pauses the newest task of the lowest priority (SIGSTOP to its process
group, one per 5 s); above `mem_resume_gb`, it resumes the oldest paused
task. A running task is an ordinary session: `status`, `log`, `stop`, and
the web UI see it. A task killed by one of the host's `avoid_signals` is
retryable: `balance` resubmits it to another host.

`{bin:<name>}` in a command is replaced on the host by the latest
`akita herd build` of that binary, uploaded by `submit` and recorded with
its commit in the task and in `results/<name>/run.json`. The files, the
agent, and placement are described in [DESIGN.md](DESIGN.md#queue).

### Archiving

Archiving moves a session's files (`.log`, `.cmd`, `.group`,
`.heartbeat`, and `.job`) from `logs/` to `logs/archived/`. It records the
session, its group, and its result directories in `.herd_archive.json`. The
web UI hides archived sessions and results by default. Archiving never
moves or deletes a result directory.

- **Arguments:** each argument names a session or a group. A group name
  archives every session in the group. Quote names that contain spaces.
- **Running sessions:** if any named session is still running (its tmux
  session, process, or SLURM job is alive), `archive` changes nothing.
- **Which results belong to a session:** a result directory belongs to a
  session when its name appears in the session's command.
- **Other results:** directories that scripts write, such as campaigns,
  are not detected. Name them after `--results`. `archive` prints a hint
  listing the result directories that the logs mention, with a ready-made
  `--results ...` to add.
- **Archived groups:** running `archive` again on an archived group
  attaches its `--results` to that group.
- **Results only:** `archive <host> --results <run>...` hides only those
  result directories.
- **Name collisions:** if a new session reuses an archived name and is
  archived too, it becomes `<name>~2` (then `~3`, and so on). `unarchive`
  restores the original name when it is free.
- **Undoing:** `unarchive` moves the logs back. It drops only the marks
  that those sessions or groups put on result directories, so a directory
  that another archived session still marks stays hidden.
- **Dry runs:** `-n` prints what would happen and changes nothing.

### Renaming results

`name apply` renames result directories to the names that the naming
rule and the `naming.legacy` command give. The legacy command reads old
names on stdin and prints `old<TAB>new` lines, with an empty `new` when it
does not know the name.

For each renamed directory, `name apply` does the following:

- Rewrites the absolute paths recorded in the directory's small text
  files: `.json`, `.log`, `.stdout`, `.txt`, `.csv`, and `.tsv` files up to
  64 MB. It skips `metrics.csv` and `*_metrics.csv`.
- Moves the directory's marks in `../.herd_archive.json`.
- Records `Renamed From` in its `run.json`.
- Appends a line to `results/RENAMES.tsv`.

It changes nothing if two directories would get the same name, or if a
running process names one of the directories on its command line. With
`-H <host>`, `<results>` is a path in the project's workspace on that
host, and the renaming runs there.

## Migrating from `exp` and `akita exp`

Herd was called `akita exp` before; the command `akita exp` still works as
an alias of `akita herd`. Herd reads what `akita exp` wrote and writes only
the new names:

| Before | Now | Compatibility |
|---|---|---|
| `~/.config/akita/exp.json`, `$AKITA_EXP_CONFIG` | `~/.config/akita/herd.json`, `$AKITA_HERD_CONFIG` | the old names are read when the new ones are absent |
| `exp.json` | `herd.json` | `exp.json` is read when there is no `herd.json`; `init` leaves it alone |
| schemas `exp-project/1`, `exp-run/1`, `exp-task/1`, `exp-cells/1`, ... | `herd-project/1`, `herd-run/1`, ... | both are accepted on read |
| `.exp_archive.json` | `.herd_archive.json` | the old file is read while the new one is missing; the next archive writes the new one |
| tmux sessions `exp-<project>-<name>` | `herd-<project>-<name>` | sessions under the old name are found by `status`, the web UI, `stop`, `archive`, and `run` |
| `.expignore` | `.herdignore` | both are read |
| `EXP_GROUP`, `EXP_NO_SYNC` | `HERD_GROUP`, `HERD_NO_SYNC` | the old names are read when the new ones are unset |
| `EXP_HEARTBEAT` | `HERD_HEARTBEAT` | both are exported to sessions |

Herd also replaces the `exp` bash script (`~/.local/bin/exp`). It keeps
that script's files on the hosts:

- workspaces in `~/exp/<project>`
- tmux sessions named `exp-<project>-<name>` (recognized; new sessions
  are named `herd-<project>-<name>`)
- the log markers
- `logs/<name>.{log,cmd,group,heartbeat}` and `logs/archived/`
- `.exp_archive.json`, including the `name~2` naming rule (read; Herd
  writes `.herd_archive.json`, which the script does not read)
- `.expignore`

To switch from the script, follow these steps:

1. Install `akita` locally and on each host.
2. Put the old hosts in `~/.config/akita/herd.json`. With the default root
   `~/exp`, the old workspaces are used as they are:

   ```json
   {"hosts": [{"name": "server1", "type": "ssh", "address": "server1"},
              {"name": "server2", "type": "ssh", "address": "server2"}]}
   ```

3. Replace `exp` with `akita herd` in commands. All of the old commands
   exist: `sync`, `run`, `group`, `status`, `log`, `stop`, `pull`, `web`,
   `archive`, and `unarchive`, with the same arguments, `-g`, `-n`,
   `--results`, `EXP_GROUP` (or `HERD_GROUP`), and `EXP_NO_SYNC` (or
   `HERD_NO_SYNC`).

Herd changes the following behavior of the script:

- **Default hosts:** commands without hosts act on every host in the user
  config, instead of the two hosts hard-coded in the script.
- **Command quoting:** the command runs as a subshell `eval`, so single
  quotes work. An `exit N` in the command is recorded as `[exit N]`
  instead of ending the session without an exit line.
- **Stop marker:** `stop` writes `[stopped] <date>  akita herd stop`, and
  it refuses a session that is not running.
- **Hang thresholds:** the `exp web` threshold flags are now user config
  keys: `stall_minutes`, `idle_cpu`, `sim_stall_minutes`, and
  `work_stall_minutes`.
- **Remote `akita`:** `status`, `archive`, `group`, and the web UI need
  `akita` on the host instead of `python3`.
- **New files:** `logs/<name>.job` is new. It holds the pid of a session
  without tmux, or the job id of a SLURM job.

## Developing

The code lives in `herd/` (the library) and `akita/internal/cmd/herd*.go`
(the CLI). [DESIGN.md](DESIGN.md#code-layout) lists the packages and what
each owns; `workspace` is the only package that knows the on-disk format.
Build and test with:

```bash
go test ./herd/... ./akita/...
cd herd/static && npm run build && npm test   # the web UI
```
