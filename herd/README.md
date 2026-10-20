# Herd

New to Herd? Try the [five-minute walkthrough](#try-it-a-five-minute-walkthrough)
below, then read the [user manual](MANUAL.md) ([中文](MANUAL.zh-CN.md));
the rest of this page is the reference.

Herd (`akita herd`; the Akita dog herding a flock of simulations across
servers) runs experiments on your machine, on SSH hosts, and on SLURM
clusters, and monitors them in a web UI. You edit code locally, `akita herd
run` copies the project to a host and starts a command there in the
background, and `akita herd pull` brings the logs and results back. Closing
your laptop does not stop anything that runs on a host.

Herd does not depend on any particular simulator. A project's
`herd.json` can say how its result directories are named, which processes
are simulators, and how to read and plot campaigns (see [DESIGN.md](DESIGN.md)).

## Try it: a five-minute walkthrough

Everything runs on one machine (macOS or Linux with `git`, `go`, `tmux`,
`rsync`, and `python3`): the two "hosts" `server1` and `server2` are two
directories, and the "simulator" is a small Go program that sleeps. You
write no code: `akita herd init --example` writes the project. Paste the
blocks in order into one shell, at your own pace: `akita herd wait`
waits wherever something has to finish first. The text after each block
is what you should see (abbreviated). [Part 1](#part-1-run-and-watch) is
the daily loop; [Part 2](#part-2-queues) adds queues across hosts. Then
[Adapting the example to your project](#adapting-the-example-to-your-project)
lists what to replace.

### Part 1: run and watch

**1. Install.** From the root of the akita checkout, install `akita`
into a temporary directory, and give the demo its own user config and
tmux server, so your own are not touched:

```bash
export T=$(mktemp -d /tmp/herd-demo.XXXXXX)
CGO_ENABLED=0 GOBIN=$T/bin go install ./akita
export PATH=$T/bin:$PATH AKITA_HERD_CONFIG=$T/user.json TMUX_TMPDIR=$T
unset TMUX
```

**2. Write the example project, and check the setup.** Each line of
`init` says what the file stands for in a real project. `doctor` checks
this machine and every host; run it whenever something does not work:

```bash
mkdir $T/demo && cd $T/demo
akita herd init --example
akita herd doctor
```

```text
wrote herd.json    project config: naming, simulators, campaigns, plots, build (edit for your project)
wrote sim/main.go  the fake simulator (your simulator)
wrote run.sh       runs one simulation (how you start yours)
wrote sweep.sh     runs a campaign of 6 simulations (your sweep script, if any)
wrote legacy.py    maps old result names (only if you have old names)
wrote flaky.sh     dies of SIGBUS on server1 (demo only)
wrote go.mod       the Go module of sim (demo only)
wrote .gitignore   ignores remote/, where pull and collect put results
wrote /tmp/herd-demo.XXXXXX/user.json
      hosts server1 and server2: ../server1 and ../server2 (your servers)
===== this machine =====
ok    akita v5...
ok    rsync
...
===== server1 (local) =====
ok    root /tmp/herd-demo.XXXXXX/server1 does not exist yet; the first sync creates it
ok    akita v5... (same as here)
ok    agent not running (submit starts it)
...
```

**3. `run`** copies the project to a host and starts a command there in
the background (a tmux session). `{name}` in the command becomes the
session's name: `run.sh NAME SIM` runs one simulation into
`results/NAME/`, and `sweep.sh` runs a campaign of 3 workloads × 2
configs. `-g` puts a session into a group:

```bash
akita herd run server1 1018_fir_base ./run.sh {name} go run ./sim
akita herd run -g sweep server2 1018_all_sweep ./sweep.sh {name} go run ./sim
```

```text
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> started 1018_fir_base on server1 (tmux session herd-demo-1018_fir_base); log: /tmp/herd-demo.XXXXXX/server1/demo/logs/1018_fir_base.log
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> started 1018_all_sweep on server2 (tmux session herd-demo-1018_all_sweep), group sweep; log: ...
```

**4. `status` and `log`.** `status` lists the sessions of every host;
`log -f` follows a log until you press Ctrl+C (the session keeps
running):

```bash
akita herd status
akita herd log -f server2 1018_all_sweep
```

```text
===== server1 =====
STATE    NAME           GROUP  STARTED      DURATION
running  1018_fir_base  -      10-19 14:02  0s
--- 1018_fir_base
[start] Mon Oct 19 02:02:35 PM UTC 2026  server1
===== server2 =====
STATE    NAME            GROUP  STARTED      DURATION
running  1018_all_sweep  sweep  10-19 14:02  0s
--- 1018_all_sweep
[start] Mon Oct 19 02:02:35 PM UTC 2026  server2
[start] Mon Oct 19 02:02:35 PM UTC 2026  server2
fir/base: step 0 of 2
fir/base: step 1 of 2
fir/base: step 2 of 2
wrote results/1018_all_sweep/1018_fir_base/run.json
fir/fast: step 0 of 2
...
^C
```

**5. `wait` and `pull`.** `wait` returns when nothing of the project
runs any more, printing each change; `pull` then copies `logs/` and
`results/` of every host to `remote/<host>/`:

```bash
akita herd wait
akita herd pull
ls remote/*/results
```

```text
14:02:41 server1:1018_fir_base ok
14:02:49 server2:1018_all_sweep ok
>> pulled server1 -> remote/server1/
>> pulled server2 -> remote/server2/
remote/server1/results:
1018_fir_base

remote/server2/results:
1018_all_sweep
```

**6. `web`** serves the monitor on http://127.0.0.1:8765 and opens it.
Start a slow session first so there is a progress bar to watch:

```bash
akita herd run server2 1018_mm_slow ./run.sh {name} go run ./sim -steps 60
akita herd web
```

In the page: **Overview** shows both hosts, `1018_mm_slow` with its
progress bar, and the finished campaign `1018_all_sweep` (6/6). Click a
session in **Sessions** for its progress, the `sim` process, the
command, and the log. **Results** lists the result directories grouped
by workload, with `config` as a coloured tag; click one for its
`run.json` properties, `summary.csv`, and files. **Charts** (or "Plot"
next to the campaign) shows the speedup of `fast` over `base` per
workload, with the GMEAN (1.53). Press Ctrl+C to stop the server, and
stop the slow session:

```bash
akita herd stop server2 1018_mm_slow
```

That is the daily loop. `group` regroups sessions, `archive` hides
finished ones, `record` adds properties to a result, and `name` checks
and renames result directories (see [Commands](#commands)).

### Part 2: queues

A queue lets each host start tasks by itself as slots and memory allow,
and lets Herd move waiting tasks to an idle host. Continue in the same
shell.

**7. `build`** builds `sim` (the `build` entry of `herd.json`) from the
current git commit plus uncommitted changes, so every task records the
exact code it ran. The default target is `linux/amd64`; here, build for
this machine:

```bash
git init -q && git add -A && git -c user.name=demo -c user.email=demo@example.com commit -qm demo
akita herd build --target $(go env GOOS)/$(go env GOARCH)
```

```text
>> sim  6353c7a  darwin/arm64  sha f825b239321d  .akita-herd/bin/f825b239321d/sim
```

**8. `submit` a sweep.** `--each key=v1,v2` submits one task per
combination: here 3 workloads × 2 configs, named by the template
`1019_{workload}_{config}`. `{bin:sim}` becomes the path of the build,
which `submit` uploads. Each task goes to the host with the most room
(`server1` has one slot, `server2` four), each host is synced once, and
`submit` starts the host's agent, the process that runs its queue:

```bash
akita herd submit -g batch --each workload=fir,mm,bfs --each config=base,fast \
  1019_{workload}_{config} ./run.sh {name} {bin:sim}
```

```text
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> queued 2 tasks on server1
>> starting the agent of server1 (tmux session herd-agent-server1)
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> queued 4 tasks on server2
>> starting the agent of server2 (tmux session herd-agent-server2)
```

**9. `queue`** shows each host's agent and its tasks (what you see
depends on the moment); `wait` waits for them all:

```bash
akita herd queue
akita herd wait
```

```text
===== server1 =====
agent: running (pid 4250), heartbeat 2s ago; slots 1/1 (own 1, simulators 0); MemAvailable 7.1 GB
STATE    PROJECT  NAME           PRIO  MEM  SINCE  INFO
pending  demo     1019_bfs_fast  5     -    2s
running  demo     1019_mm_fast   5     -    2s     pid 4255
===== server2 =====
agent: running (pid 4242), heartbeat 0s ago; slots 3/4 (own 3, simulators 1); MemAvailable 7.1 GB
STATE    PROJECT  NAME           PRIO  MEM  SINCE  INFO
pending  demo     1019_bfs_base  5     -    2s
running  demo     1019_fir_base  5     -    0s     pid 4310
running  demo     1019_fir_fast  5     -    0s     pid 4320
running  demo     1019_mm_base   5     -    0s     pid 4330
14:03:16 server1:1019_mm_fast running
14:03:16 server1:1019_bfs_fast pending
...
14:03:42 server1:1019_bfs_fast ok
```

Both hosts are this machine, so each agent also counts the other's `sim`
processes ("simulators") against its slots.

**10. Retry and `balance`.** A task killed by one of its host's
`avoid_signals` is retryable (`server1` has "bad memory": `flaky.sh`
dies of SIGBUS there), and `balance` resubmits it to another host. It
also moves pending tasks of a busy host to an idle one. Queue three
tasks on `server1`, and wait until the second runs (the first has then
died):

```bash
akita herd submit --host server1 1020_bfs_fast ./run.sh {name} ./flaky.sh {bin:sim}
akita herd submit --host server1 1020_fir_long ./run.sh {name} {bin:sim} -steps 15
akita herd submit --host server1 1020_mm_long ./run.sh {name} {bin:sim}
akita herd wait --until running server1 1020_fir_long
akita herd queue server1
akita herd balance -n
akita herd balance
```

```text
...
14:03:50 server1:1020_fir_long running
===== server1 =====
agent: running (pid 4250), heartbeat 0s ago; slots 1/1 (own 1, simulators 0); MemAvailable 7.2 GB
STATE        PROJECT  NAME           PRIO  MEM  SINCE  INFO
pending      demo     1020_mm_long   5     -    8s
running      demo     1020_fir_long  5     -    0s     pid 4500
failed(138)  demo     1020_bfs_fast  5     -    0s     SIGBUS retryable
...
14:03:51 would retry 1020_bfs_fast (SIGBUS on server1) -> server2
14:03:51 would move 1020_mm_long: server1 -> server2
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
14:03:51 retry 1020_bfs_fast (SIGBUS on server1) -> server2
14:03:51 move 1020_mm_long: server1 -> server2
```

(The exit status of SIGBUS is 138 on macOS and 135 on Linux.)
`akita herd web --balance` runs `balance` every minute, and its **Queue**
view shows all of this.

**11. `collect`** copies the results and logs of ended tasks to
`remote/<host>/`, once per task and without `collect.exclude` (here
`stats/`):

```bash
akita herd wait
akita herd collect
ls remote/server2/results/1020_bfs_fast
```

```text
...
14:04:06 collected server1:1020_fir_long (ok) -> remote/server1/
14:04:06 collected server1:1020_bfs_fast (failed) -> remote/server1/
...
14:04:06 collected server2:1020_bfs_fast (ok) -> remote/server2/
...
meta.json  run.json  summary.csv
```

**12. Optional: the memory guard.** Below `mem_pause_gb` of free memory,
an agent pauses (SIGSTOP) the newest task of the lowest priority (the
largest `--priority`); above `mem_resume_gb`, it resumes it (SIGCONT).
To show it without using real memory, restart `server2`'s agent so that
it reads its free memory from a file (`--fake-mem-file` is a flag for
tests), then lower and raise the number:

```bash
echo 8 > $T/mem-gb
akita herd agent stop server2; akita herd agent start server2 --fake-mem-file $T/mem-gb
akita herd submit --host server2 --priority 9 1021_bfs_slow ./run.sh {name} {bin:sim} -steps 30
akita herd wait --until running server2 1021_bfs_slow
echo 0.1 > $T/mem-gb; akita herd wait --until paused server2 1021_bfs_slow
echo 8 > $T/mem-gb; akita herd wait --until running server2 1021_bfs_slow
akita herd stop server2 1021_bfs_slow
```

```text
...
14:04:12 server2:1021_bfs_slow running
14:04:12 server2:1021_bfs_slow running
14:04:18 server2:1021_bfs_slow paused
14:04:18 server2:1021_bfs_slow paused
14:04:22 server2:1021_bfs_slow running
>> stopped server2:1021_bfs_slow
```

**13. Clean up.** Stop the agents (tasks that still run would keep
running), the demo's tmux server, and remove the directory:

```bash
akita herd agent stop server1; akita herd agent stop server2
tmux -S $T/tmux-$(id -u)/default kill-server 2>/dev/null
cd / && rm -rf $T
```

## Adapting the example to your project

The example has three parts, and each can be replaced on its own. Keep
what you need and delete the rest; nothing else depends on it.

**Hosts** (the user config, `~/.config/akita/herd.json`). Replace the
two directories with your servers: `name` is what you type in commands,
`address` is anything `ssh` accepts, and `queue` fits the machine.
Nothing has to be installed there: `akita herd` puts its akita on a host
by itself ([Install](#install)), and `akita herd doctor` checks the
setup. A SLURM host has no queue; use `run` there.

```json
{
  "hosts": [
    {"name": "server1", "type": "ssh", "address": "server1",
     "queue": {"slots": 16, "mem_reserve_gb": 16, "avoid_signals": ["SIGBUS"]}},
    {"name": "server2", "type": "ssh", "address": "me@server2.example.edu",
     "queue": {"slots": 32}},
    {"name": "cluster", "type": "slurm", "address": "login.cluster.example",
     "slurm": {"partition": "cpu", "time": "24:00:00", "cpus": 1, "mem": "16G"}}
  ]
}
```

**The project config** (`herd.json`). Every key is optional and
independent; `{"schema": "herd-project/1"}` is enough to run sessions.
Each key of the example, and what to put there:

| Key | In the example | Replace with | Without it |
|---|---|---|---|
| `sync.exclude` | `*.tmp` | big inputs or outputs that should not be copied to hosts | `.gitignore`d paths are still left out |
| `naming` | results are named `<date>_<workload>_<config>` | the fields of your result names; `group_by` groups them in the web UI, `compare_by` colours them | any name is accepted |
| `naming.legacy` | `legacy.py` | a script that maps your old names | `name apply` renames nothing |
| `simulators` | processes named `sim`; `-workload` labels them, `-metrics` names their result | your simulator's executable name and flags | sessions show no simulator rows |
| `campaigns` | a result holding `campaign.json` is a campaign; its cells are the subdirectories with `run.json` | the file your sweep script writes | no campaign view |
| `plots` | speedup of each `config` over `base`, per `workload` | your metric and dimensions | no charts |
| `build` | `go build` in `sim/` | the command that builds your simulator | `{bin:...}` cannot be used; commands run what was synced |
| `collect.exclude` | `stats/` | large files `collect` should not copy back | everything is copied |

**The scripts.** Herd only needs a command to run; how it runs your
simulator is up to you:

- `sim/main.go` stands in for your simulator. Nothing in it is required;
  the two optional parts (a progress bar via `$HERD_HEARTBEAT`, results
  under `results/<run>/`) are marked in its header.
- `run.sh` turns a result name into simulator flags. Replace its
  simulator line; keep `akita herd record` if you want `run.json`.
- `sweep.sh` is a campaign; replace it with your sweep script, or delete
  it.
- `flaky.sh`, `go.mod`, and `legacy.py` exist only for the demo.

In commands, replace `go run ./sim` (sessions) or `{bin:sim}` (queued
tasks) with your simulator, and the result names and `--each` values
with yours; keep `{name}` so that each run writes under its own name.

## Install

```bash
go install github.com/sarchlab/akita/v5/akita@latest
```

Nothing needs to be installed on SSH and SLURM hosts by hand: `status`,
`archive`, the queue, and the web UI run `akita herd ...` there, and the
first time a host needs it, `akita herd` puts the same akita there
(`~/.cache/akita-herd/<version>-<hash>/akita`, or under the host's
`akita_dir`). For another platform (a Linux server from a Mac), it is
built from the source this akita was built from, which needs Go here;
the build is cached. When that fails, the `akita` on the host's `PATH`
(`~/go/bin` and `~/.local/bin` are searched too) is used. Set `"akita"`
in the host's config to always use an akita you installed there.

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
| `akita` | The path of an `akita` you installed on the host. A leading `~/` is expanded on the host. Unset, `akita herd` puts its own there (see [Install](#install)). |
| `akita_dir` | Where `akita herd` puts its akita on the host. The default is `~/.cache/akita-herd`. |
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
| `akita herd init [--example]` | Writes `./herd.json` and the user config if they are missing. `--example` writes the walkthrough's project instead, and its hosts only to a new `$AKITA_HERD_CONFIG`. |
| `akita herd sync [host...]` | Copies the project to `<root>/<project>` with `rsync --delete`, so the copy matches the project. Files you delete locally are deleted on the host too, except for the paths listed above. |
| `akita herd run [-g group] [--no-sync] <host> <name> <command>...` | Syncs, then starts the command in the workspace as session `<name>`; `{name}` in the command becomes `<name>`. Output goes to `logs/<name>.log`. `HERD_GROUP` sets the default group, and `HERD_NO_SYNC=1` skips the sync. Put the flags before `<host>`. Everything after `<name>` is the command. |
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
| `akita herd submit [-g group] [--priority N] [--mem GB] [--host H\|--hosts H,H] [--each key=v1,v2]... [--file tasks.jsonl] <name> <command>...` | Queues tasks on hosts' queues; see [Queues](#queues). `--each` submits one task per combination of the values, with `{key}` replaced in the name template and the command. |
| `akita herd queue [host...]` | Shows each host's agent and its pending, running, paused, and ended tasks. |
| `akita herd cancel <host> <name>` | Removes a pending task. |
| `akita herd balance [--watch] [-n] [host...]` | Moves pending tasks to idle hosts and retries tasks that died of a host's `avoid_signals`. |
| `akita herd collect [--watch] [host...]` | Copies the results and logs of ended tasks to `remote/<host>/`. |
| `akita herd wait [--until ended\|running\|paused] [--timeout D] [host [name...]]` | Waits until the named sessions and tasks reach the state (default: ended), printing each change; without names, until nothing of the project runs or waits. Exits 0 also when a task failed. |
| `akita herd doctor [host...]` | Checks this machine and the hosts: ssh, `bash`, `rsync`, `tmux`, `sbatch`, `akita` and whether its version matches, the root, and the agent. Exits 1 when a check fails. |
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
you can use single quotes in it. `{name}` in the command is replaced by
the session's name when it has only letters, digits, and `._+=,@%:-`
(it is not quoted); the session also gets it as `HERD_NAME`. `HERD_HEARTBEAT` (also exported as
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

### Recording results

`record` writes or merges `<dir>/run.json`, which the web UI shows. The
fields come from the directory name and the naming rule, the properties
from the arguments, and missing properties from `meta.json`:

```bash
akita herd record results/1018_fir_base Note=first
```

```text
wrote results/1018_fir_base/run.json
{
  "fields": {"config": "base", "date": "1018", "workload": "fir"},
  "metrics": {"total_time": 3},
  "name": "1018_fir_base",
  "properties": [
    {"property": "Command", "value": "go run ./sim -workload fir -config base"},
    {"property": "Exit Code", "value": "0"},
    {"property": "Note", "value": "first"}
  ],
  "schema": "herd-run/1"
}
```

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

For example, archive the group `warmup` and one more result directory,
list what is archived, and undo it:

```bash
akita herd archive server1 warmup --results 1017_fir_fast
akita herd archive server1
akita herd unarchive server1 warmup --results 1017_fir_fast
```

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

With the example's naming rule and `legacy.py` (old names are
`<workload>-<config>-<date>`):

```bash
akita herd name check 1018_fir_base fir-fast-1017
akita herd name plan -H server1 results
akita herd name apply -H server1 results
```

```text
"fir-fast-1017": want 3 fields (date, workload, config) separated by "_", got 1
Error: 1 of 2 names break the naming rule
1018_fir_base	1018_fir_base
fir-fast-1017	1017_fir_fast
fir-fast-1017	1017_fir_fast
```

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

1. Install `akita` locally (it puts itself on the hosts).
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
