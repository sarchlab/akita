# Herd 使用说明

[English](MANUAL.md) · [命令和文件参考](README.md)（英文）·
[设计文档](DESIGN.md)（英文）

Herd（akita herd；可以想象成一只秋田犬把一群模拟实验赶到各台服务器上）可以在你自己的电脑、通过 SSH 连接的服务器或 SLURM 集群上运行实验，
并把它们都显示在同一个网页上。你照常在笔记本上改代码；`akita herd` 把项目复制到
目标机器，在后台启动命令，让你查看、停止、归档实验并画图。合上笔记本不会中断任何实验。

本说明按任务逐步介绍 Herd。每条命令、选项和文件格式的完整说明见
[README](README.md)。

想先在一台机器上把所有功能试一遍，见下面的
[五分钟上手：完整演练](#五分钟上手完整演练)（与 README 中的演练相同）。

## 目录

1. [基本概念](#1-基本概念)
2. [安装](#2-安装)
3. [在本机跑第一个实验](#3-在本机跑第一个实验)
4. [添加 SSH 服务器和 SLURM 集群](#4-添加-ssh-服务器和-slurm-集群)
5. [日常使用](#5-日常使用)
6. [网页界面](#6-网页界面)
7. [用 herd.json 描述你的项目](#7-用-herdjson-描述你的项目)
8. [配合 Akita 模拟器使用](#8-配合-akita-模拟器使用)
9. [从 akita exp 或旧的 exp 脚本迁移](#9-从-akita-exp-或旧的-exp-脚本迁移)
10. [常见问题](#10-常见问题)

## 五分钟上手：完整演练

下面这个例子在一台机器上（装有 `git`、`go`、`tmux`、`rsync` 和 `python3` 的 macOS
或 Linux）用到 Herd 的全部功能。两台"主机" `server1` 和 `server2` 其实是临时目录
下的两个目录，"模拟器"是一个只会 sleep 的小 Go 程序。按顺序把代码块粘贴到同一个
shell 里；每个代码块后面是你应该看到的输出（有删节）。

**1. 安装。** 在 akita 源码的根目录下，把 `akita` 装到一个临时目录，并给这次演练
单独的用户配置和 tmux 服务器：

```bash
export T=$(mktemp -d /tmp/herd-demo.XXXXXX)
CGO_ENABLED=0 GOBIN=$T/bin go install ./akita
export PATH=$T/bin:$PATH AKITA_HERD_CONFIG=$T/user.json TMUX_TMPDIR=$T
unset TMUX
```
**2. 建一个项目。** 里面有假模拟器 `sim`（它把进度写到 `$HERD_HEARTBEAT`，并在
`-metrics` 所在的结果目录里写 `stats/metrics.csv`、`summary.csv`、`meta.json` 和
带指标的 `run.json`）、`run.sh`（按 `<date>_<workload>_<config>` 形式的结果名运行
`sim`，再用 `akita herd record` 记录 `run.json`）、`sweep.sh`（3 个负载 × 2 个配置
的 campaign）、`flaky.sh`（在 `server1` 上因 SIGBUS 退出）和 `legacy.py`（把旧结果
名映射成新名字）：

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
**3. `init`，然后写配置。** `init` 写出配置骨架，再用下面的内容替换。项目配置把
结果命名为 `<date>_<workload>_<config>`，说明怎样识别 `sim` 进程，用 `runs` 适配器
读取 campaign，定义一张加速比图，并说明怎样构建 `sim`。用户配置把 `server1` 和
`server2` 设为两台各有自己 root 和队列的本地主机（`server1` 只有一个槽位，并避开
SIGBUS）：

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
**4. `sync`** 把项目复制到每台主机：

```bash
akita herd sync
ls $T/server1/demo
```

```text
>> sync demo -> server1:/tmp/herd-demo.XXXXXX/server1/demo
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
flaky.sh  go.mod  herd.json  legacy.py  run.sh  sim  sweep.sh
```
**5. `run`** 启动会话（会先同步）。`-g` 把会话放进一个组。第三个会话用旧的命名
方式写一个结果：

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
**6. `status`** 列出每台主机上的会话：

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
**7. `log`** 显示日志；`-f` 持续跟踪，直到你按 Ctrl+C（会话继续运行）：

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
**8. `stop`** 停止一个正在运行的会话：

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
**9. `pull`** 把每台主机的 `logs/` 和 `results/` 复制到 `remote/<主机>/`。先等
sweep 跑完（大约 15 秒）：

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
**10. `group`** 把会话加入一个组（`-` 表示移出所在的组）：

```bash
akita herd group server1 warmup 1018_mm_base
```

```text
>> 1 sessions put into group warmup
```
**11. `record`** 写入或合并 `run.json`：字段从目录名按命名规则解析，属性来自命令
参数，缺少的属性从 `meta.json` 补上。`run.sh` 已经在 `sim` 写完指标后运行过它；
下面手动加一个属性：

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
**12. `name`** 按命名规则检查名字，并用 `legacy.py` 给出的新名字重命名旧的结果
目录：

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
**13. `archive`** 隐藏已结束的会话（这里是组 `warmup`）和它们的结果；`--results`
指定更多结果目录。只给主机时，列出已归档的内容。`unarchive` 撤销归档：

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
**14. `build`** 在当前提交（加上未提交的改动）的临时 git worktree 里构建 `sim`
（`herd.json` 的 `build` 项）。默认目标是 `linux/amd64`；这里为本机构建：

```bash
akita herd build --target $(go env GOOS)/$(go env GOARCH)
```

```text
>> sim  6353c7a  darwin/arm64  sha f825b239321d  .akita-herd/bin/f825b239321d/sim
```
**15. `agent`** 运行一台主机的队列（`submit` 会在需要时启动它）。这里 `server2`
的 agent 从一个文件读取可用内存（`--fake-mem-file` 是测试用的参数），这样第 20 步
不必真的占用内存就能演示内存保护：

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
**16. `submit`** 把任务放进队列。`{bin:sim}` 会换成 `sim` 最近一次构建的路径，
`submit` 会把它上传到主机。`--file` 每行读一个 JSON 任务；用 `--hosts` 时，每个任务
分到最空闲的主机：

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
**17. `queue`** 显示每台主机的 agent，以及排队、运行、暂停和已结束的任务。两台
主机其实是同一台机器，所以每个 agent 也会把另一台的 `sim` 进程（"simulators"）算进
自己的槽位，`1019_fir_base` 要等 `server2` 的任务结束：

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
**18. `cancel`** 删除还没开始的任务（这个任务要的内存比主机多，所以一直在排队）：

```bash
akita herd submit --host server2 --mem 100 1019_bfs_base ./run.sh 1019_bfs_base {bin:sim}
akita herd cancel server2 1019_bfs_base
```

```text
>> queued 1 tasks on server2
>> cancelled 1019_bfs_base on server2
```
**19. 重试和 `balance`。** 被主机 `avoid_signals` 中的信号杀死的任务可以重试：
`balance` 把它重新提交到另一台主机。它还会把忙碌主机上排队的任务移到空闲主机。
`-n` 只显示会做什么。先等 `server1` 空闲：

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
（SIGBUS 的退出码在 macOS 上是 138，在 Linux 上是 135。）

**20. 内存保护。** 可用内存低于 `mem_pause_gb` 时，agent 暂停（SIGSTOP）优先级最低
（`--priority` 最大）的任务中最新的一个；高于 `mem_resume_gb` 时恢复它（SIGCONT）。
先等第 19 步的任务结束，再调低、调高假的可用内存：

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
**21. `collect`** 把已结束任务的结果和日志复制到 `remote/<主机>/`，每个任务只复制
一次，不含 `collect.exclude`（这里是 `stats/`）：

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
**22. `web`** 在 http://127.0.0.1:8765 提供监控页面并打开浏览器（`--port` 换端口，
`--no-browser` 不打开浏览器）。先启动一个慢会话，好看到进度条：

```bash
akita herd run server2 1019_mm_slow ./run.sh 1019_mm_slow go run ./sim -steps 60
akita herd web
```

```text
>> sync demo -> server2:/tmp/herd-demo.XXXXXX/server2/demo
>> started 1019_mm_slow on server2 (tmux session herd-demo-1019_mm_slow); log: ...
akita herd web: http://127.0.0.1:8765/ (Ctrl+C to quit)
```
在页面上：**概览** 显示两台主机、带进度条的 `1019_mm_slow`，以及已完成的
campaign `1018_all_sweep`（6/6）。打开 **实验**，点 `1019_mm_slow` 查看进度、`sim`
进程、命令和日志；组（`warmup`、`sweep`、`batch`）折叠成一行。**结果** 按 workload
分组列出结果目录，`config` 显示为彩色标签；点 `1018_fir_base` 查看属性、
`summary.csv` 和文件。**图表**（或 campaign 旁的"画图"）：选 `1018_all_sweep`，看
每个 workload 上 `fast` 相对 `base` 的加速比和 GMEAN（1.53）。**队列** 显示两个
agent 和第 16 到 20 步的任务。按 Ctrl+C 停止服务。`akita herd web --balance` 还会
每分钟运行一次 `balance`。

**23. 清理。** 停止 agent（还在运行的任务会继续运行）和这次演练的 tmux 服务器，然后
删除目录：

```bash
akita herd agent stop server1; akita herd agent stop server2
tmux -S $T/tmux-$(id -u)/default kill-server 2>/dev/null
cd / && rm -rf $T
```
**使用真实服务器。** 只需改用户配置：给每台主机写上地址（以及合适的队列设置），
并在上面安装 `akita`（见[第 2 节](#2-安装)）：

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
这时 `build` 不需要 `--target`（默认就是 `linux/amd64`）；SLURM 主机没有队列，
在那里用 `run`。

## 1. 基本概念

| 名词 | 含义 |
|---|---|
| **项目（project）** | 你工作的目录，通常是一个 git 仓库。在它的根目录下运行 `akita herd`。项目名就是目录名。 |
| **主机（host）** | 实验运行的地方：`local`（本机）、`ssh`（服务器）或 `slurm`（集群）。主机列在你的用户配置里。 |
| **工作区（workspace）** | 项目在主机上的副本：`<root>/<项目名>`，`root` 默认是 `~/exp`。本机的工作区就是项目目录本身。 |
| **会话（session）** | 用 `akita herd run` 启动的一条命令。它有名字、日志（`logs/<名字>.log`）和状态，如 `running`、`ok`。 |
| **组（group）** | 把多个会话归在一起的标签，比如同一项研究的所有实验。 |
| **结果（run）** | 一个结果目录 `results/<名字>/`，里面可以有一份 `run.json` 记录。 |
| **campaign** | 包含很多次模拟的结果目录，通常是"负载 × 配置"。网页可以为它画图。 |

不要直接在主机上改代码：所有改动都在本地项目里做，再通过 `akita herd sync` 同步过去。

## 2. 安装

在你的电脑上：

```bash
go install github.com/sarchlab/akita/v5/akita@latest
```

本机还需要 `rsync`。每台 SSH 或 SLURM 主机需要 `bash`、`rsync`，以及 `akita`
程序本身，因为监控页面会在那里运行 `akita herd probe`。主机上建议装 `tmux`；没有
的话，会话用 `nohup setsid` 运行。

要从 Mac 或其他机器把 `akita` 放到 Linux 主机上，可以交叉编译后拷过去：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o akita-linux github.com/sarchlab/akita/v5/akita
scp akita-linux myserver:bin/akita
```

如果程序不在主机的 `PATH` 里（也会查找 `~/go/bin` 和 `~/.local/bin`），用主机配置里
的 `"akita"` 告诉 `akita herd` 它在哪（见第 4 节）。

## 3. 在本机跑第一个实验

```bash
cd ~/my-simulator          # 项目根目录
akita herd init             # 生成 ./herd.json 和 ~/.config/akita/herd.json
akita herd run local hello "echo hello; sleep 5; echo done"
akita herd status           # hello 正在运行
akita herd log local hello  # 目前为止的输出
akita herd web              # 打开 http://127.0.0.1:8765
```

`init` 会生成一份最简的项目配置，以及只含一台主机 `local` 的用户配置。每个会话的
输出写在 `logs/<名字>.log`，首尾各有一行标记：

```text
[start] Fri Oct 16 01:56:33 AM UTC 2026  local  3e9a41c     （日期、主机、git 版本）
hello
done
[exit 0] Fri Oct 16 01:56:38 AM UTC 2026
```

结果由你的程序自己写：写到 `results/<名字>/` 下，网页就会列出来。

## 4. 添加 SSH 服务器和 SLURM 集群

主机写在 `~/.config/akita/herd.json`（或环境变量 `$AKITA_HERD_CONFIG` 指定的文件）里：

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

- **`address`**：任何 `ssh` 能接受的地址都可以。`~/.ssh/config` 里的别名也行，
  这是设置用户名、端口和密钥最简单的办法。请用密钥登录：`akita herd` 调用 `ssh`
  时不会弹出密码输入。
- **`root`**：这台主机上存放工作区的目录（默认 `~/exp`）。请放在空间足够存结果的磁盘上。
- **`akita`**：主机上 `akita` 程序的路径。
- **`tmux: false`**：即使装了 tmux，也改用 `nohup setsid` 运行会话。
- **SLURM 主机**：每个会话按 `slurm` 里的选项提交成一个批处理作业。不写 `address`
  的 SLURM 主机直接用本机的 `sbatch`，适合在登录节点上运行 `akita herd` 的情况。
  在队列里等待的作业显示为 `queued`。

可以用 `akita herd sync lab1` 和 `akita herd run lab1 hello "hostname"` 检查配置是否正确。

## 5. 日常使用

### 启动实验

```bash
akita herd run lab1 fir-base "./sim -benchmark=fir"
akita herd run -g ablation lab1 fir-opt "./sim -benchmark=fir -opt"
```

- `run` 会先同步项目，所以主机上跑的正是你当前的代码。要一次启动很多会话，先同步
  一次，后面跳过同步：`akita herd sync lab1`，然后 `HERD_NO_SYNC=1 akita herd run ...`
  （或加 `--no-sync`）。
- `-g <组名>`（或环境变量 `HERD_GROUP=<组名>`）把会话放进一个组。建议每项研究一个组；
  网页会把一组折叠成一行，并在整组跑完时通知你。
- 选项要写在主机名前面。会话名后面的所有内容都是命令；命令里有 `;`、`|`、`&&`
  时要加引号。
- 旧会话结束后，名字可以重复使用。新会话会覆盖旧日志；想保留的话先把旧会话归档。

`sync` 会复制项目里的所有内容，但以下这些除外：`.git/`、`logs/`、`results/`、
`remote/`、`__pycache__/`、`.DS_Store`、`.herd_archive.json`、`.gitignore` 忽略的路径、
`herd.json` 里的 `sync.exclude`，以及 `.herdignore` 里的规则。这些路径在主机上也永远
不会被删除，所以那里的结果是安全的。

### 查看和停止

```bash
akita herd status            # 所有主机；加 -a 包括已归档的会话
akita herd log lab1 fir-opt -f  # 持续跟踪；Ctrl+C 只停止跟踪，不停实验
akita herd stop lab1 fir-opt
```

在主机上，`tmux attach -t herd-<项目名>-<会话名>` 可以实时看会话；按 Ctrl+B 再按 D
退出，实验继续运行。

会话状态：

| 状态 | 含义 |
|---|---|
| `running` | 正在运行。 |
| `queued` | SLURM 作业在排队等待。 |
| `ok` / `failed` | 退出码为 0 / 非 0。 |
| `stopped` | 被 `akita herd stop` 停止。 |
| `died` | 没有退出标记就结束了，比如主机重启或 SLURM 到了时间上限。 |
| `stalled`（网页） | 还在运行，但没有进展（见第 6 节）。 |

### 收集结果

`akita herd pull` 把每台主机的 `logs/` 和 `results/` 复制到你项目里的
`remote/<主机名>/`，方便在本机分析。

### 队列：构建、提交、调度、取回

实验数量超过服务器的核数或内存时，用队列提交，而不是用 `run` 直接启动。每台主机上会运行
一个 *agent*：有空闲槽位和内存时启动你的任务，内存不足时暂停任务，笔记本合上后照常工作。

1. 在用户配置里给每台主机加一个 `queue`：

   ```json
   {"name": "lab1", "type": "ssh", "address": "lab1.example.edu",
    "queue": {"slots": 16, "mem_reserve_gb": 16, "mem_pause_gb": 8,
              "mem_resume_gb": 24, "avoid_signals": ["SIGBUS"]}}
   ```

   `slots` 是同时运行的任务数（默认 CPU 数的一半；队列之外启动的模拟器也算在内）。
   只有 MemAvailable 减去任务的 `--mem` 后仍不低于 `mem_reserve_gb` 时，任务才会启动。
   低于 `mem_pause_gb` 时，agent 暂停（SIGSTOP）优先级最低的任务中最新启动的那个；
   高于 `mem_resume_gb` 时再恢复。`avoid_signals` 列出表示"这台机器有问题，换一台试试"
   的信号。
2. 在 `herd.json` 里描述你的二进制（见第 7 节），然后构建：`akita herd build`。它为 Linux
   构建当前 commit 加上未提交的改动，并按内容哈希记住这个二进制。在命令里需要这个
   二进制的地方写 `{bin:<名字>}`。
3. 提交：

   ```bash
   akita herd submit --host lab1 --mem 20 1019_diag_fir_base_a41f7e2 "./run.sh {bin:sim} fir"
   akita herd submit --hosts lab1,lab2 -g round2 --file tasks.jsonl
   ```

   `submit` 会同步项目，主机上没有这个二进制时上传它，把任务放进队列，agent 没在运行
   时启动它。用 `--hosts` 时，每个任务分给最空闲的主机。文件里每行一个 JSON 任务：
   `{"name": ..., "cmd": ..., "mem_gb": 20, "priority": 3, "group": ...}`。priority 数字
   越小越先启动（默认 5）。
4. 查看：`akita herd queue` 列出每台主机的 agent、槽位、内存，排队、运行和暂停的任务，
   以及已结束的任务。运行中的任务就是普通会话，`status`、`log`、`stop` 和网页照常可用。
   `akita herd cancel lab1 <名字>` 删除还没启动的任务。
5. 调度：`akita herd balance` 把排队的任务移到队列为空、还有空闲槽位的主机上，并把因
   `avoid_signals` 里的信号死掉的任务重新提交到别的主机（最多两次）。`-n` 只显示会做
   什么；`--watch` 每分钟重复一次，`akita herd web --balance` 也一样。
6. 取回：`akita herd collect` 把每个已结束任务的 `results/<名字>/` 和日志复制到
   `remote/<主机名>/`，每个任务只取一次（跳过 `collect.exclude`）；`--watch` 持续取回。

结果的 `run.json` 会记下二进制的哈希和 commit（"Binary"、"Git Revision"），所以总能
知道结果是哪份代码跑出来的。`akita herd agent start|stop|status <主机>` 手动管理 agent；
停止 agent 不影响正在运行的任务，下一个 agent 会接管它们。

### 整理：分组和归档

```bash
akita herd group lab1 ablation fir-base fir-opt  # 把会话加进一个组
akita herd archive -n lab1 ablation              # 先看看会做什么
akita herd archive lab1 ablation --results 1015_abl_wg32k_base_3e9a41c
akita herd archive lab1                          # 列出已归档的内容
akita herd unarchive lab1 ablation               # 撤销归档
```

归档会把已结束会话的日志移到 `logs/archived/`，并在网页上默认隐藏它们和它们的
结果目录。归档从不移动或删除结果。会话的结果目录是指名字出现在它命令里的那些；
其他目录（比如脚本写出的 campaign）用 `--results` 指定，`archive` 也会列出日志里
提到的目录作为提示。只要指定的会话中有一个还在运行，就什么都不会改。

### 结果命名

如果你的 `herd.json` 定义了命名规则（见第 7 节），可以用这些命令保持结果名一致：

```bash
akita herd name check 1015_abl_wg32k_base_3e9a41c   # 不符合规则时退出码为 1
akita herd name plan -H lab1 results                  # 旧名 -> 新名
akita herd name apply -H lab1 results --dry-run
akita herd record results/1015_abl_wg32k_base_3e9a41c "Command=./sim -benchmark=fir"
```

`record` 在结果目录里写 `run.json`：从目录名解析出的字段，加上你给出的属性。
`name apply` 用你配置的映射命令（`naming.legacy`）把旧命名的目录改成新名，并修正
目录里记录的路径。

## 6. 网页界面

`akita herd web` 轮询所有主机（或你指定的主机），在 `http://127.0.0.1:8765` 提供页面
（`--port` 改端口，`--no-browser` 不自动打开浏览器）。页面是只读的：不会启动、停止或
修改任何实验。它只在本机监听；要从别的电脑看，请用 SSH 端口转发。

**语言。**页面有英文和中文两种，默认跟随浏览器的语言设置；右上角的按钮（EN / 中文）
可以切换，选择会被记住。在链接后加 `?lang=en` 或 `?lang=zh`，打开链接的人看到的
就是指定的语言。

**顶栏。**上次更新的时间（出现琥珀色的"数据过旧"标签表示数据旧了，可能有主机很慢或
连不上）、每台主机一个圆点并标出类型、语言按钮，以及"开启通知"：开启后，会话结束、
疑似卡住或一整组跑完时会发桌面通知。

**概览。**几块数字：运行中的实验和模拟数、疑似卡住的实验、24 小时内的失败数、各主机中
最少的可用内存。下面是每台主机一张卡片（CPU 负载、内存、GPU）、运行中的会话及其
进度条，以及各个 campaign，未完成的排在前面。

**实验。**列出所有会话的状态、主机、开始时间、用时、进度、CPU、内存和最后输出时间。
可以按主机、状态、关键字、时间（最近 24 小时）和是否归档筛选；同一组的会话折叠成一行。
点一行看详情：进度条、模拟器进程、是否判定为卡住及原因、命令（带复制按钮），以及日志
末尾（有新输出时自动滚动）。

**结果。**左边是搜索和按命名规则生成的筛选项（如日期、类别、配置、版本），以及主机和
类型。结果目录按 `group_by` 里的字段分组，每行用彩色标签显示 `compare_by` 的值；同一个
值在所有地方颜色相同。点一行看它的属性（来自 `run.json`、结果里的 SQLite 文件和
`meta.json`）、`summary.csv` 和文件列表。勾选 2–4 行可以并排对比：关键结果及其比值、
配置差异、统计差异。组标题上的"同类对比"会把这一组的所有 campaign 送到图表页。

**图表。**最多选 4 个 campaign。三种图：

- *消融*：一个 campaign 里每个系列值（如每种配置）相对它的 baseline，按负载画，并给出
  所有显示的系列都有结果的负载上的几何平均（GMEAN）。
- *对比*：同一个系列值在多个 campaign 之间比较，各自除以自己的 baseline（没有的话借用
  另一个已选 campaign 的 baseline）。
- *敏感性*：横轴是扫描的参数。

选项：选用哪个图（项目定义了多个时）、参考点和归一化方式、固定 baseline、纵轴上限、
点图例隐藏系列。可以把图下载成 SVG、把数据下载成 CSV，也可以复制一个能重新打开同一张图
的链接。

**队列。**每台有队列的主机一张卡片：agent 是否在运行（以及最近的心跳）、已用槽位
（自己的任务数，或者机器上检测到的模拟器数，取较大者）、可用内存和各个阈值，然后是
运行中的任务（暂停的标成琥珀色）、按启动顺序排列的排队任务，以及已结束的任务和它们的
退出码、信号、是否已重试或已取回。用 `akita herd web --balance` 启动时，页面还会列出调度
做了什么。

**什么情况算"疑似卡住"。**如果程序会报告进度（Akita monitor 或心跳文件，见第 8 节）：
模拟时间 `sim_stall_minutes`（默认 10）分钟不推进，或完成的工作量 `work_stall_minutes`
（默认 30）分钟不增加，或心跳 `sim_stall_minutes` 分钟没有更新，就判定为疑似卡住。
否则，CPU 使用率低于 `idle_cpu`%（默认 5）持续 `stall_minutes`（默认 30）分钟判定为
疑似卡住；CPU 在忙但日志和输出文件 `stall_minutes` 分钟没有变化时，标为"无新输出"。
这些参数以及轮询间隔 `interval`（默认 10 秒）都在用户配置里设置。

## 7. 用 herd.json 描述你的项目

所有字段都是可选的。没有 `herd.json` 时，会话和结果照样可以运行和监控，但结果名不会被
拆成字段，模拟器进程不会被识别；campaign 按默认标记（`EXPERIMENT_METADATA.json` 或
`campaign.json`）识别，用 `runs` 方式读取，并根据数据自动推测一种图。

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

**`naming`** 把结果目录名拆成字段。按上面的规则，`1015_abl_wg32k_base+mshr128_3e9a41c`
会被读成：日期 `1015`、类别 `abl`、项目 `wg32k`、配置 `base+mshr128`（筛选时按
`variant_separator` 前面的 `base` 归类）、版本 `3e9a41c`。`pattern` 是单个字段的正则
表达式（字段内不能有分隔符）；`values` 是固定的取值表及其显示名；`not` 列出禁止的取值。
`group_by` 指定哪些字段相同就算"同一个实验"；`compare_by` 是同一实验里互相对比的那个
字段。四位数字且是合法月日的字段会显示成 `MM-DD`。`legacy` 指定一个把旧名映射成新名的
命令，供 `name apply` 使用：它从标准输入读名字，输出 `旧名<TAB>新名` 行（不认识的旧名，
新名留空）。

**`simulators`** 告诉监控哪些进程是模拟器：`executable` 和程序文件名做匹配，
`label_flag` 指定表格里显示的标签（如 benchmark），`output_flag` 指定一个输出路径，
它所在的目录就是这个进程写入的 campaign。网页就是靠它统计 campaign 中正在运行的格子数。

**`campaigns`** 指定哪些结果目录是 campaign（`detect`：目录里必须存在的文件名），以及
怎样读取它的格子（`adapter`）：

- `{"type": "runs"}`：每个含 `run.json` 的子目录是一个格子。它的 `fields` 是格子的键，
  `metrics` 是数值，例如

  ```json
  {"schema": "herd-run/1", "name": "fir_m1",
   "fields": {"workload": "fir", "config": "optA"},
   "metrics": {"total_time": 1.2e-5, "wgs": 512}, "status": "ok"}
  ```

  可以再放一个 `campaign.json`，写上 `{"total": N}`，进度条就能显示计划的格子总数。
- `{"type": "command", "command": ["python3", "tools/cells.py"]}`：用于其他任何格式的
  campaign。这个命令在主机的工作区里运行，campaign 目录作为最后一个参数，输出格子的
  JSON（格式 `herd-cells/1`，见 [DESIGN.md](DESIGN.md)）：`dimensions`、可选的 `labels`
  和 `order`，以及 `cells`，每个格子有 `key`、`status`（`ok`、`failed`、`running`、
  `missing`、`bad`）和 `metrics`。`sweep` 部分用来描述敏感性 campaign。

**`plots`** 把格子变成图。`kind: "ratio"` 时，对越小越好的指标（`lower_is_better`），
每个值 = baseline ÷ 该格子，否则 = 该格子 ÷ baseline。baseline 是 `series` 键等于
`baseline`、其余键都相同的那个格子；只有 `match` 里的每个指标都相等时才计算（比如
模拟的工作量相同）。`summary: "gmean"` 会加上几何平均。`kind: "sweep"` 在一次扫描的
各个点上画同样的比值。

**`build`** 列出 `akita herd build` 构建的二进制：`name`、`command` 运行的目录 `dir`
（相对于项目），以及额外的 `env`。命令里的 `{out}` 是输出文件；`GOOS`/`GOARCH` 来自
`--target`（默认 `linux/amd64`）。

```json
"build": {"binaries": [{"name": "sim", "dir": "cmd/sim",
            "command": ["go", "build", "-o", "{out}", "."],
            "env": {"CGO_ENABLED": "0"}}]},
"collect": {"exclude": ["*_trace.csv"]}
```

**`collect`** 列出 `akita herd collect` 不取回的 rsync 规则（本机用不到的大文件）。

## 8. 配合 Akita 模拟器使用

`akita herd` 对模拟器没有任何要求，但 Akita 模拟器能让它显示更多信息：

- **实时进度。**如果模拟器打开了 monitor
  （`sim.MakeBuilder().WithMonitor(monitoring.NewMonitor())`），它会输出
  `Monitoring simulation with http://localhost:<port>`。`akita herd` 在会话日志里找到这一
  行，读取 monitor 的进度条和运行信息，所以网页上显示的进度条和 AkitaRTM 一样，卡住
  判断也依据它。会话详情里有指向模拟器自己 monitor 页面的链接。
- **运行元数据。**Akita 会写出 `akita_sim_<id>.sqlite3`（或用 `WithOutputFileName`
  指定的名字）。把它写进结果目录，它的 `exec_info` 表（命令、开始和结束时间、工作目录）
  就会显示为这个结果的属性。数据库以只读方式打开。
- **心跳文件。**任何程序不用 monitor 也能报告进度：环境变量 `HERD_HEARTBEAT` 是
  `logs/<名字>.heartbeat` 的路径；程序定期重写这个文件，内容是一个 JSON 对象，例如

  ```json
  {"pid": 12345, "wall": 1792111137.5, "finished": false,
   "sim_time_ns": 1.5e6,
   "progress": [{"id": "wg", "name": "Work-groups", "total": 32768, "finished": 1200, "in_progress": 64}]}
  ```

  `wall` 是这次更新的 Unix 时间。结束时把 `finished` 设为 `true`。

## 9. 从 akita exp 或旧的 exp 脚本迁移

Herd 改名之前叫 `akita exp`。不需要做任何迁移：`akita exp ...` 仍可作为别名使用；
新名字不存在时，Herd 会读取旧名字：`~/.config/akita/exp.json`（或
`$AKITA_EXP_CONFIG`）、项目里的 `exp.json`、`exp-*/1` 格式、`.exp_archive.json`、
`.expignore`、`EXP_GROUP` / `EXP_NO_SYNC`，以及仍在名为 `exp-<项目名>-<会话名>` 的
tmux 会话里运行的实验。写入时只用新名字（`herd.json`、`herd-*/1`、
`.herd_archive.json`、tmux 会话 `herd-<项目名>-<会话名>`）。旧文件可以随时改名；
完整对照表见 [README](README.md#migrating-from-exp-and-akita-exp)。

Herd 还沿用 `exp` 脚本在磁盘上的目录结构，已有的工作区不需要任何迁移：

1. 在本机和每台主机上安装 `akita`（见第 2 节）。
2. 把原来的主机写进 `~/.config/akita/herd.json`；默认的 root `~/exp` 就是原来的目录。
3. 把 `exp` 换成 `akita herd`。命令和选项（`-g`、`-n`、`--results`、`EXP_GROUP` 或
   `HERD_GROUP`、`EXP_NO_SYNC` 或 `HERD_NO_SYNC`）都一样。

不同之处：不指定主机时，命令作用于配置里的所有主机；命令在子 shell 里运行，所以
`exit N` 会被记录，命令里也可以用单引号；`stop` 写入 `[stopped] <日期>  akita herd stop`，
并拒绝停止没在运行的会话；监控的判定阈值改成用户配置里的字段；主机上需要的是 `akita`，
不再是 `python3`；新增了 `logs/<名字>.job` 文件。

## 10. 常见问题

| 问题 | 检查什么 |
|---|---|
| 网页上某台主机显示错误 | 能否不输密码执行 `ssh <地址> true`？主机上有没有 `akita`（`ssh <地址> akita herd probe --json`）？如果在别的路径，在主机配置里设置 `"akita"`。 |
| `run` 提示会话已在运行 | 这个名字正被使用。换个名字，或先 `stop` 旧的。 |
| 手动能跑，作为会话就失败 | 会话在工作区里用登录 shell（`bash -l`）启动。检查相对于项目根目录的路径，以及只在交互式 shell 里设置的环境变量。 |
| `sync` 没有复制某个文件 | 它被 `.gitignore`、`sync.exclude` 或 `.herdignore` 忽略了，或者位于 `logs/`、`results/`、`remote/` 下。 |
| 会话显示 `died` | 它没有退出标记就结束了：主机重启、进程被杀，或 SLURM 结束了作业（`status` 会显示 SLURM 的状态）。 |
| 结果列表里出现"未解析"的名字 | 它们不符合 `naming`。`akita herd name check <名字>` 会说明哪个字段不符合；`name plan` / `apply` 可以批量改名。 |
| campaign 没有图 | 它被识别了吗（`campaigns.detect`）？适配器能输出格子吗（在主机上运行 `akita herd probe campaign ...`）？`plots` 里写的指标，格子里有吗？ |
| `archive` 什么都没改 | 指定的会话中有一个还在运行，或者加了 `-n`。 |
| 排队的任务不启动 | 运行 `akita herd queue <主机>`：agent 在运行吗？槽位满了吗（队列之外启动的模拟器也算）？MemAvailable 减去它的 `mem_gb` 是否低于 `mem_reserve_gb`，或者有任务被暂停（这时不会启动新任务）？任务是否避开了这台主机？agent 的日志在主机上的 `<root>/.akita-herd/agent.log`。 |
| `submit` 提示某个二进制没有构建 | 先运行 `akita herd build`；`{bin:<名字>}` 必须是 `herd.json` 里 `build` 列出的二进制。 |
| `balance` 什么都没移动 | 目标主机需要 agent 在运行、队列为空，并且有空闲的槽位和内存；只移动当前项目的任务。 |
