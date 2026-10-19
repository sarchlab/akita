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

This example uses every feature of Herd on one machine (macOS or Linux
with `git`, `go`, `tmux`, `rsync`, and `python3`). Two "hosts",
`server1` and `server2`, are two directories under a temporary
directory, and the "simulator" is a small Go program that sleeps. Paste
the blocks in order into one shell; the text after each block is what
you should see (abbreviated).

**1. Install.** From the root of the akita checkout, install `akita`
into a temporary directory, and give the demo its own user config and
tmux server:

```bash
export T=$(mktemp -d /tmp/herd-demo.XXXXXX)
CGO_ENABLED=0 GOBIN=$T/bin go install ./akita
export PATH=$T/bin:$PATH AKITA_HERD_CONFIG=$T/user.json TMUX_TMPDIR=$T
unset TMUX
```

**2. Make a project.** It has the fake simulator `sim` (it reports
progress in `$HERD_HEARTBEAT` and writes `stats/metrics.csv`,
`summary.csv`, `meta.json`, and `run.json` with the metrics into the
result directory of `-metrics`), `run.sh` (runs `sim` for a result name
`<date>_<workload>_<config>` and records `run.json` with `akita herd
record`), `sweep.sh` (a campaign of 3 workloads × 2 configs), `flaky.sh`
(dies of SIGBUS on `server1`), and `legacy.py` (maps old result names to
new ones):

```bash
mkdir $T/demo && cd $T/demo && git init -q
printf 'module demo\n\ngo 1.22\n' > go.mod
printf 'remote/\n' > .gitignore
mkdir sim && cat > sim/main.go <<'EOF'
// sim is a fake simulator.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	workload := flag.String("workload", "fir", "workload")
	config := flag.String("config", "base", "configuration")
	steps := flag.Int("steps", 5, "seconds to run")
	metrics := flag.String("metrics", "", "results/<run>/stats/metrics.csv")
	flag.Parse()

	start := time.Now().Unix()
	for i := 0; i <= *steps; i++ {
		if hb := os.Getenv("HERD_HEARTBEAT"); hb != "" {
			data, _ := json.Marshal(map[string]any{
				"pid": os.Getpid(), "wall": time.Now().Unix(), "finished": i == *steps,
				"progress": []any{map[string]any{"id": "steps", "name": *workload,
					"start_time": start, "total": *steps, "finished": i}},
			})
			_ = os.WriteFile(hb+".tmp", data, 0o644)
			_ = os.Rename(hb+".tmp", hb)
		}
		fmt.Printf("%s/%s: step %d of %d\n", *workload, *config, i, *steps)
		if i < *steps {
			time.Sleep(time.Second)
		}
	}

	if *metrics == "" {
		return
	}
	t := map[string]float64{"fir": 3, "mm": 8, "bfs": 5}[*workload]
	if *config == "fast" {
		t /= map[string]float64{"fir": 1.2, "mm": 2, "bfs": 1.5}[*workload]
	}
	run := filepath.Dir(filepath.Dir(*metrics))
	_ = os.MkdirAll(filepath.Dir(*metrics), 0o755)
	_ = os.WriteFile(*metrics, fmt.Appendf(nil, ",where,what,value\n0,driver,total_time,%g\n", t), 0o644)
	_ = os.WriteFile(filepath.Join(run, "summary.csv"),
		fmt.Appendf(nil, "workload,config,total_time\n%s,%s,%g\n", *workload, *config, t), 0o644)
	_ = os.WriteFile(filepath.Join(run, "run.json"),
		fmt.Appendf(nil, `{"metrics": {"total_time": %g}}`+"\n", t), 0o644)
	_ = os.WriteFile(filepath.Join(run, "meta.json"), []byte(`{"exit_code": 0}`+"\n"), 0o644)
}
EOF
cat > run.sh <<'EOF'
#!/bin/sh
# run.sh NAME SIM...: run SIM for result NAME (<date>_<workload>_<config>).
name=$1; shift
IFS=_ read -r date workload config <<END
$name
END
"$@" -workload "$workload" -config "$config" -metrics "results/$name/stats/metrics.csv" &&
akita herd record "results/$name" "Command=$* -workload $workload -config $config"
EOF
cat > sweep.sh <<'EOF'
#!/bin/sh
# sweep.sh CAMPAIGN SIM...: every workload x config into results/CAMPAIGN/.
c=results/$1; date=${1%%_*}; shift
mkdir -p "$c" && echo '{"total": 6}' > "$c/campaign.json"
for w in fir mm bfs; do
  for k in base fast; do
    "$@" -steps 2 -workload $w -config $k -metrics "$c/${date}_${w}_$k/stats/metrics.csv" &&
    akita herd record "$c/${date}_${w}_$k" || exit 1
  done
done
EOF
cat > flaky.sh <<'EOF'
#!/bin/sh
# flaky.sh CMD...: server1 has "bad memory": die of SIGBUS there.
case $PWD in */server1/*) kill -BUS $$ ;; esac
exec "$@"
EOF
cat > legacy.py <<'EOF'
import sys
for line in sys.stdin:  # old names are <workload>-<config>-<date>
    old = line.strip()
    parts = old.split("-")
    new = f"{parts[2]}_{parts[0]}_{parts[1]}" if len(parts) == 3 else ""
    print(f"{old}\t{new}")
EOF
chmod +x run.sh sweep.sh flaky.sh
go run ./sim -steps 0
```

```text
fir/base: step 0 of 0
```

**3. `init`, then the configs.** `init` writes skeletons; replace them.
The project config names results `<date>_<workload>_<config>`, says how
to recognise `sim` processes, reads campaigns with the `runs` adapter,
defines a speedup plot, and says how to build `sim`. The user config
makes `server1` and `server2` two local hosts with their own roots and
queues (`server1` has one slot and avoids SIGBUS):

```bash
akita herd init
cat > herd.json <<'EOF'
{
  "schema": "herd-project/1",
  "sync": {"exclude": ["*.tmp"]},
  "naming": {
    "separator": "_",
    "fields": [
      {"name": "date", "pattern": "\\d{4}"},
      {"name": "workload", "pattern": "[a-z0-9]+"},
      {"name": "config", "pattern": "[a-z0-9]+"}
    ],
    "group_by": ["workload"],
    "compare_by": "config",
    "legacy": "python3 legacy.py"
  },
  "simulators": [{"executable": "sim", "label_flag": "-workload", "output_flag": "-metrics"}],
  "campaigns": {"detect": ["campaign.json"], "adapter": {"type": "runs"}},
  "plots": [{"name": "speedup", "kind": "ratio", "metric": "total_time",
             "lower_is_better": true, "x": "workload", "series": "config",
             "baseline": "base", "summary": "gmean"}],
  "build": {"binaries": [{"name": "sim", "dir": "sim",
                          "command": ["go", "build", "-o", "{out}", "."],
                          "env": {"CGO_ENABLED": "0"}}]},
  "collect": {"exclude": ["stats/"]}
}
EOF
cat > $AKITA_HERD_CONFIG <<EOF
{
  "hosts": [
    {"name": "server1", "type": "local", "root": "$T/server1",
     "queue": {"slots": 1, "mem_reserve_gb": 0.5, "mem_pause_gb": 0.2,
               "mem_resume_gb": 0.4, "avoid_signals": ["SIGBUS"]}},
    {"name": "server2", "type": "local", "root": "$T/server2",
     "queue": {"slots": 4, "mem_reserve_gb": 1, "mem_pause_gb": 0.5, "mem_resume_gb": 2}}
  ]
}
EOF
git add -A && git -c user.name=demo -c user.email=demo@example.com commit -qm demo
```

```text
wrote /tmp/herd-demo.XXXXXX/demo/herd.json
wrote /tmp/herd-demo.XXXXXX/user.json
```

**4. `sync`** copies the project to every host:

```bash
akita herd sync
ls $T/server1/demo
```

```text
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
flaky.sh  go.mod  herd.json  legacy.py  run.sh  sim  sweep.sh
```

**5. `run`** starts sessions (it syncs first). `-g` puts a session into a
group. The third session writes a result under an old name:

```bash
akita herd run -g warmup server1 1018_fir_base ./run.sh 1018_fir_base go run ./sim
akita herd run -g sweep server2 1018_all_sweep ./sweep.sh 1018_all_sweep go run ./sim
akita herd run server1 old go run ./sim -workload fir -config fast -metrics results/fir-fast-1017/stats/metrics.csv
```

```text
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> started 1018_fir_base on server1 (tmux session herd-demo-1018_fir_base), group warmup; log: /tmp/herd-demo.XXXXXX/server1/demo/logs/1018_fir_base.log
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> started 1018_all_sweep on server2 (tmux session herd-demo-1018_all_sweep), group sweep; log: ...
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> started old on server1 (tmux session herd-demo-old); log: ...
```

**6. `status`** lists the sessions of every host:

```bash
akita herd status
```

```text
===== server1 =====
STATE    NAME           GROUP   STARTED      DURATION
running  old            -       10-19 14:02  0s
running  1018_fir_base  warmup  10-19 14:02  0s
--- old
[start] Mon Oct 19 02:02:35 PM UTC 2026  server1
--- 1018_fir_base
[start] Mon Oct 19 02:02:35 PM UTC 2026  server1
===== server2 =====
STATE    NAME            GROUP  STARTED      DURATION
running  1018_all_sweep  sweep  10-19 14:02  0s
--- 1018_all_sweep
[start] Mon Oct 19 02:02:35 PM UTC 2026  server2
```

**7. `log`** shows a log; `-f` follows it until you press Ctrl+C (the
session keeps running):

```bash
akita herd log -f server2 1018_all_sweep
```

```text
[start] Mon Oct 19 02:02:35 PM UTC 2026  server2
fir/base: step 0 of 2
fir/base: step 1 of 2
fir/base: step 2 of 2
wrote results/1018_all_sweep/1018_fir_base/run.json
fir/fast: step 0 of 2
...
^C
```

**8. `stop`** kills a running session:

```bash
akita herd run server1 1018_mm_base ./run.sh 1018_mm_base go run ./sim -steps 600
akita herd stop server1 1018_mm_base
akita herd status -n 0 server1
```

```text
>> stopped server1:1018_mm_base
===== server1 =====
STATE    NAME           GROUP   STARTED      DURATION
stopped  1018_mm_base   -       10-19 14:02  0s
ok       1018_fir_base  warmup  10-19 14:02  5s
ok       old            -       10-19 14:02  5s
```

**9. `pull`** copies `logs/` and `results/` of every host to
`remote/<host>/`. Wait for the sweep to finish first (about 15 s):

```bash
sleep 15; akita herd pull
ls remote/*/results
```

```text
>> pulled server1 -> remote/server1/
>> pulled server2 -> remote/server2/
remote/server1/results:
1018_fir_base  fir-fast-1017

remote/server2/results:
1018_all_sweep
```

**10. `group`** adds sessions to a group (`-` removes them):

```bash
akita herd group server1 warmup 1018_mm_base
```

```text
>> 1 sessions put into group warmup
```

**11. `record`** writes or merges `run.json`: the fields come from the
directory name and the naming rule, and the properties from the
arguments; properties it lacks come from `meta.json`. `run.sh` already
ran it after `sim` had written the metrics; add a property by hand:

```bash
(cd $T/server1/demo && akita herd record results/1018_fir_base Note=first)
cat $T/server1/demo/results/1018_fir_base/run.json
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

**12. `name`** checks names against the naming rule, and renames old
result directories with the names that `legacy.py` gives:

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

**13. `archive`** hides finished sessions (here the group `warmup`) and
their results; `--results` names more result directories. With only a
host, it lists what is archived. `unarchive` undoes it:

```bash
akita herd archive server1 warmup --results 1017_fir_fast
akita herd archive server1
akita herd status -a -n 0 server1
akita herd unarchive server1 warmup --results 1017_fir_fast
```

```text
Archiving 2 sessions (logs move to logs/archived/):
  1018_fir_base  [group warmup]
      result directory 1018_fir_base
  1018_mm_base  [group warmup]
  Result directories given with --results:
      result directory 1017_fir_fast
...
Archived sessions (logs/archived/, 2 in total):
  group warmup  2, archived 2026-10-19 14:03: 1018_fir_base, 1018_mm_base
...
STATE    NAME                      GROUP   STARTED      DURATION
stopped  1018_mm_base (archived)   warmup  10-19 14:02  0s
ok       1018_fir_base (archived)  warmup  10-19 14:02  5s
ok       old                       -       10-19 14:02  5s
...
Unmarked 2 result directories as archived: 1017_fir_fast, 1018_fir_base
```

**14. `build`** builds `sim` (the `build` entry of `herd.json`) from a
temporary git worktree of the current commit plus uncommitted changes.
The default target is `linux/amd64`; build for this machine:

```bash
akita herd build --target $(go env GOOS)/$(go env GOARCH)
```

```text
>> sim  6353c7a  darwin/arm64  sha f825b239321d  .akita-herd/bin/f825b239321d/sim
```

**15. `agent`** runs a host's queue (`submit` starts it when needed).
Here `server2`'s agent reads its free memory from a file
(`--fake-mem-file` is a flag for tests) so that step 20 can show the
memory guard without using real memory:

```bash
echo 8 > $T/mem-gb
akita herd agent start server2 --fake-mem-file $T/mem-gb
akita herd agent start server1
akita herd agent status server2
```

```text
>> starting the agent of server2 (tmux session herd-agent-server2)
>> agent of server2 runs: pid 4242, root /tmp/herd-demo.XXXXXX/server2, 4 slots
>> starting the agent of server1 (tmux session herd-agent-server1)
>> agent of server1 runs: pid 4250, root /tmp/herd-demo.XXXXXX/server1, 1 slots
server2:/tmp/herd-demo.XXXXXX/server2: agent running (pid 4242, host server2), heartbeat 3s ago
  slots 0/4 (own 0, simulators found 0), MemAvailable 8.0 GB (reserve 1, pause < 0.5, resume > 2)
```

**16. `submit`** queues tasks. `{bin:sim}` becomes the path of the
latest build of `sim`, which `submit` uploads to the host. `--file`
reads one JSON task per line; with `--hosts`, each task goes to the
host with the most room:

```bash
akita herd submit --host server1 1019_fir_base ./run.sh 1019_fir_base {bin:sim}
cat > tasks.jsonl <<'EOF'
{"name": "1019_fir_fast", "cmd": "./run.sh 1019_fir_fast {bin:sim}"}
{"name": "1019_mm_base", "cmd": "./run.sh 1019_mm_base {bin:sim}"}
{"name": "1019_mm_fast", "cmd": "./run.sh 1019_mm_fast {bin:sim}", "priority": 1}
EOF
akita herd submit --hosts server1,server2 -g batch --file tasks.jsonl
```

```text
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> queued 1 tasks on server1
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> queued 3 tasks on server2
```

**17. `queue`** shows each host's agent, and its pending, running,
paused, and ended tasks. Both hosts are this machine, so each agent also
counts the other's `sim` processes ("simulators") against its slots,
and `1019_fir_base` waits until `server2`'s tasks end:

```bash
sleep 6; akita herd queue
```

```text
===== server1 =====
agent: running (pid 4250), heartbeat 2s ago; slots 3/1 (own 0, simulators 3); MemAvailable 8.0 GB
STATE    PROJECT  NAME           PRIO  MEM  SINCE  INFO
pending  demo     1019_fir_base  5     -    6s
===== server2 =====
agent: running (pid 4242), heartbeat 3s ago; slots 3/4 (own 3, simulators 0); MemAvailable 8.0 GB
STATE    PROJECT  NAME           PRIO  MEM  SINCE  INFO
running  demo     1019_fir_fast  5     -    3s     pid 4310
running  demo     1019_mm_base   5     -    3s     pid 4320
running  demo     1019_mm_fast   1     -    3s     pid 4302
```

**18. `cancel`** removes a task that has not started (this one asks for
more memory than the host has, so it stays pending):

```bash
akita herd submit --host server2 --mem 100 1019_bfs_base ./run.sh 1019_bfs_base {bin:sim}
akita herd cancel server2 1019_bfs_base
```

```text
>> queued 1 tasks on server2
>> cancelled 1019_bfs_base on server2
```

**19. Retry and `balance`.** A task killed by one of its host's
`avoid_signals` is retryable: `balance` resubmits it to another host. It
also moves pending tasks of a busy host to an idle one. `-n` only shows
what it would do. Wait until `server1` is idle first:

```bash
sleep 20
akita herd submit --host server1 1019_bfs_fast ./run.sh 1019_bfs_fast ./flaky.sh {bin:sim}
akita herd submit --host server1 1019_fir_long ./run.sh 1019_fir_long {bin:sim} -steps 15
akita herd submit --host server1 1019_mm_long ./run.sh 1019_mm_long {bin:sim}
sleep 12; akita herd queue server1
akita herd balance -n
akita herd balance
```

```text
...
===== server1 =====
agent: running (pid 4250), heartbeat 4s ago; slots 1/1 (own 1, simulators 0); MemAvailable 8.1 GB
STATE        PROJECT  NAME           PRIO  MEM  SINCE  INFO
pending      demo     1019_mm_long   5     -    12s
running      demo     1019_fir_long  5     -    4s     pid 4500
failed(138)  demo     1019_bfs_fast  5     -    4s     SIGBUS retryable
ok           demo     1019_fir_base  5     -    19s
14:05:38 would retry 1019_bfs_fast (SIGBUS on server1) -> server2
14:05:38 would move 1019_mm_long: server1 -> server2
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
14:05:39 retry 1019_bfs_fast (SIGBUS on server1) -> server2
14:05:39 move 1019_mm_long: server1 -> server2
```

(The exit status of SIGBUS is 138 on macOS and 135 on Linux.)

**20. Memory guard.** Below `mem_pause_gb` of free memory the agent
pauses (SIGSTOP) the newest task of the lowest priority (the largest
`--priority`); above `mem_resume_gb` it resumes it (SIGCONT). Wait for
the tasks of step 19 to end, then lower and raise the fake free memory:

```bash
sleep 10
akita herd submit --host server2 --priority 9 1019_bfs_slow ./run.sh 1019_bfs_slow {bin:sim} -steps 12
sleep 7; echo 0.1 > $T/mem-gb; sleep 7; akita herd queue server2
echo 8 > $T/mem-gb; sleep 7; akita herd queue server2
```

```text
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> queued 1 tasks on server2
===== server2 =====
agent: running (pid 4242), heartbeat 4s ago; slots 1/4 (own 1, simulators 1); MemAvailable 0.1 GB
STATE   PROJECT  NAME           PRIO  MEM  SINCE  INFO
paused  demo     1019_bfs_slow  9     -    9s     pid 4600
ok      demo     1019_bfs_fast  5     -    9s
...
===== server2 =====
agent: running (pid 4242), heartbeat 0s ago; slots 1/4 (own 1, simulators 1); MemAvailable 8.0 GB
STATE    PROJECT  NAME           PRIO  MEM  SINCE  INFO
running  demo     1019_bfs_slow  9     -    16s    pid 4600
ok       demo     1019_bfs_fast  5     -    16s
...
```

**21. `collect`** copies the results and logs of ended tasks to
`remote/<host>/`, once per task and without `collect.exclude` (here
`stats/`):

```bash
sleep 15; akita herd collect
ls remote/server2/results/1019_bfs_fast
```

```text
14:06:25 collected server1:1019_fir_long (ok) -> remote/server1/
14:06:25 collected server1:1019_bfs_fast (failed) -> remote/server1/
14:06:25 collected server1:1019_fir_base (ok) -> remote/server1/
14:06:25 collected server2:1019_bfs_slow (ok) -> remote/server2/
14:06:25 collected server2:1019_bfs_fast (ok) -> remote/server2/
...
meta.json  run.json  summary.csv
```

**22. `web`** serves the monitor on http://127.0.0.1:8765 and opens it
(`--port` to change the port, `--no-browser` to not open it). Start a
slow session first so there is a progress bar to watch:

```bash
akita herd run server2 1019_mm_slow ./run.sh 1019_mm_slow go run ./sim -steps 60
akita herd web
```

```text
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> started 1019_mm_slow on server2 (tmux session herd-demo-1019_mm_slow); log: ...
akita herd web: http://127.0.0.1:8765/ (Ctrl+C to quit)
```

In the page: **Overview** shows both hosts, `1019_mm_slow` with its
progress bar, and the finished campaign `1018_all_sweep` (6/6). Open
**Sessions** and click `1019_mm_slow` for its progress, the `sim`
process, the command, and the log; groups (`warmup`, `sweep`, `batch`)
fold into one row. **Results** lists the result directories grouped by
workload, with `config` as a coloured tag; click `1018_fir_base` for its
properties, `summary.csv`, and files. **Charts** (or "Plot" next to the
campaign): pick `1018_all_sweep` to see the speedup of `fast` over
`base` per workload, with the GMEAN (1.53). **Queue** shows both agents
and the tasks of steps 16 to 20. Press Ctrl+C to stop the server.
`akita herd web --balance` also runs `balance` every minute.

**23. Clean up.** Stop the agents (tasks that still run would keep
running), the demo's tmux server, and remove the directory:

```bash
akita herd agent stop server1; akita herd agent stop server2
tmux -S $T/tmux-$(id -u)/default kill-server 2>/dev/null
cd / && rm -rf $T
```

**Using real servers.** Only the user config changes: give each host
its address (and the queue settings that fit it), and install `akita`
on it (see [Install](#install)):

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

`build` then needs no `--target` (the default is `linux/amd64`), and a
SLURM host has no queue: use `run` there.

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
