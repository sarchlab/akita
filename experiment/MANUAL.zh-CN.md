# akita exp 使用说明

[English](MANUAL.md) · [命令和文件参考](README.md)（英文）·
[设计文档](DESIGN.md)（英文）

`akita exp` 可以在你自己的电脑、通过 SSH 连接的服务器或 SLURM 集群上运行实验，
并把它们都显示在同一个网页上。你照常在笔记本上改代码；`akita exp` 把项目复制到
目标机器，在后台启动命令，让你查看、停止、归档实验并画图。合上笔记本不会中断任何实验。

本说明按任务逐步介绍这个工具。每条命令、选项和文件格式的完整说明见
[README](README.md)。

## 目录

1. [基本概念](#1-基本概念)
2. [安装](#2-安装)
3. [在本机跑第一个实验](#3-在本机跑第一个实验)
4. [添加 SSH 服务器和 SLURM 集群](#4-添加-ssh-服务器和-slurm-集群)
5. [日常使用](#5-日常使用)
6. [网页界面](#6-网页界面)
7. [用 exp.json 描述你的项目](#7-用-expjson-描述你的项目)
8. [配合 Akita 模拟器使用](#8-配合-akita-模拟器使用)
9. [从旧的 exp 脚本迁移](#9-从旧的-exp-脚本迁移)
10. [常见问题](#10-常见问题)

## 1. 基本概念

| 名词 | 含义 |
|---|---|
| **项目（project）** | 你工作的目录，通常是一个 git 仓库。在它的根目录下运行 `akita exp`。项目名就是目录名。 |
| **主机（host）** | 实验运行的地方：`local`（本机）、`ssh`（服务器）或 `slurm`（集群）。主机列在你的用户配置里。 |
| **工作区（workspace）** | 项目在主机上的副本：`<root>/<项目名>`，`root` 默认是 `~/exp`。本机的工作区就是项目目录本身。 |
| **会话（session）** | 用 `akita exp run` 启动的一条命令。它有名字、日志（`logs/<名字>.log`）和状态，如 `running`、`ok`。 |
| **组（group）** | 把多个会话归在一起的标签，比如同一项研究的所有实验。 |
| **结果（run）** | 一个结果目录 `results/<名字>/`，里面可以有一份 `run.json` 记录。 |
| **campaign** | 包含很多次模拟的结果目录，通常是"负载 × 配置"。网页可以为它画图。 |

不要直接在主机上改代码：所有改动都在本地项目里做，再通过 `akita exp sync` 同步过去。

## 2. 安装

在你的电脑上：

```bash
go install github.com/sarchlab/akita/v5/akita@latest
```

本机还需要 `rsync`。每台 SSH 或 SLURM 主机需要 `bash`、`rsync`，以及 `akita`
程序本身，因为监控页面会在那里运行 `akita exp probe`。主机上建议装 `tmux`；没有
的话，会话用 `nohup setsid` 运行。

要从 Mac 或其他机器把 `akita` 放到 Linux 主机上，可以交叉编译后拷过去：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o akita-linux github.com/sarchlab/akita/v5/akita
scp akita-linux myserver:bin/akita
```

如果程序不在主机的 `PATH` 里（也会查找 `~/go/bin` 和 `~/.local/bin`），用主机配置里
的 `"akita"` 告诉 `akita exp` 它在哪（见第 4 节）。

## 3. 在本机跑第一个实验

```bash
cd ~/my-simulator          # 项目根目录
akita exp init             # 生成 ./exp.json 和 ~/.config/akita/exp.json
akita exp run local hello "echo hello; sleep 5; echo done"
akita exp status           # hello 正在运行
akita exp log local hello  # 目前为止的输出
akita exp web              # 打开 http://127.0.0.1:8765
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

主机写在 `~/.config/akita/exp.json`（或环境变量 `$AKITA_EXP_CONFIG` 指定的文件）里：

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
  这是设置用户名、端口和密钥最简单的办法。请用密钥登录：`akita exp` 调用 `ssh`
  时不会弹出密码输入。
- **`root`**：这台主机上存放工作区的目录（默认 `~/exp`）。请放在空间足够存结果的磁盘上。
- **`akita`**：主机上 `akita` 程序的路径。
- **`tmux: false`**：即使装了 tmux，也改用 `nohup setsid` 运行会话。
- **SLURM 主机**：每个会话按 `slurm` 里的选项提交成一个批处理作业。不写 `address`
  的 SLURM 主机直接用本机的 `sbatch`，适合在登录节点上运行 `akita exp` 的情况。
  在队列里等待的作业显示为 `queued`。

可以用 `akita exp sync lab1` 和 `akita exp run lab1 hello "hostname"` 检查配置是否正确。

## 5. 日常使用

### 启动实验

```bash
akita exp run lab1 fir-base "./sim -benchmark=fir"
akita exp run -g ablation lab1 fir-opt "./sim -benchmark=fir -opt"
```

- `run` 会先同步项目，所以主机上跑的正是你当前的代码。要一次启动很多会话，先同步
  一次，后面跳过同步：`akita exp sync lab1`，然后 `EXP_NO_SYNC=1 akita exp run ...`
  （或加 `--no-sync`）。
- `-g <组名>`（或环境变量 `EXP_GROUP=<组名>`）把会话放进一个组。建议每项研究一个组；
  网页会把一组折叠成一行，并在整组跑完时通知你。
- 选项要写在主机名前面。会话名后面的所有内容都是命令；命令里有 `;`、`|`、`&&`
  时要加引号。
- 旧会话结束后，名字可以重复使用。新会话会覆盖旧日志；想保留的话先把旧会话归档。

`sync` 会复制项目里的所有内容，但以下这些除外：`.git/`、`logs/`、`results/`、
`remote/`、`__pycache__/`、`.DS_Store`、`.exp_archive.json`、`.gitignore` 忽略的路径、
`exp.json` 里的 `sync.exclude`，以及 `.expignore` 里的规则。这些路径在主机上也永远
不会被删除，所以那里的结果是安全的。

### 查看和停止

```bash
akita exp status            # 所有主机；加 -a 包括已归档的会话
akita exp log lab1 fir-opt -f  # 持续跟踪；Ctrl+C 只停止跟踪，不停实验
akita exp stop lab1 fir-opt
```

在主机上，`tmux attach -t exp-<项目名>-<会话名>` 可以实时看会话；按 Ctrl+B 再按 D
退出，实验继续运行。

会话状态：

| 状态 | 含义 |
|---|---|
| `running` | 正在运行。 |
| `queued` | SLURM 作业在排队等待。 |
| `ok` / `failed` | 退出码为 0 / 非 0。 |
| `stopped` | 被 `akita exp stop` 停止。 |
| `died` | 没有退出标记就结束了，比如主机重启或 SLURM 到了时间上限。 |
| `stalled`（网页） | 还在运行，但没有进展（见第 6 节）。 |

### 收集结果

`akita exp pull` 把每台主机的 `logs/` 和 `results/` 复制到你项目里的
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
2. 在 `exp.json` 里描述你的二进制（见第 7 节），然后构建：`akita exp build`。它为 Linux
   构建当前 commit 加上未提交的改动，并按内容哈希记住这个二进制。在命令里需要这个
   二进制的地方写 `{bin:<名字>}`。
3. 提交：

   ```bash
   akita exp submit --host lab1 --mem 20 1019_diag_fir_base_a41f7e2 "./run.sh {bin:sim} fir"
   akita exp submit --hosts lab1,lab2 -g round2 --file tasks.jsonl
   ```

   `submit` 会同步项目，主机上没有这个二进制时上传它，把任务放进队列，agent 没在运行
   时启动它。用 `--hosts` 时，每个任务分给最空闲的主机。文件里每行一个 JSON 任务：
   `{"name": ..., "cmd": ..., "mem_gb": 20, "priority": 3, "group": ...}`。priority 数字
   越小越先启动（默认 5）。
4. 查看：`akita exp queue` 列出每台主机的 agent、槽位、内存，排队、运行和暂停的任务，
   以及已结束的任务。运行中的任务就是普通会话，`status`、`log`、`stop` 和网页照常可用。
   `akita exp cancel lab1 <名字>` 删除还没启动的任务。
5. 调度：`akita exp balance` 把排队的任务移到队列为空、还有空闲槽位的主机上，并把因
   `avoid_signals` 里的信号死掉的任务重新提交到别的主机（最多两次）。`-n` 只显示会做
   什么；`--watch` 每分钟重复一次，`akita exp web --balance` 也一样。
6. 取回：`akita exp collect` 把每个已结束任务的 `results/<名字>/` 和日志复制到
   `remote/<主机名>/`，每个任务只取一次（跳过 `collect.exclude`）；`--watch` 持续取回。

结果的 `run.json` 会记下二进制的哈希和 commit（"Binary"、"Git Revision"），所以总能
知道结果是哪份代码跑出来的。`akita exp agent start|stop|status <主机>` 手动管理 agent；
停止 agent 不影响正在运行的任务，下一个 agent 会接管它们。

### 整理：分组和归档

```bash
akita exp group lab1 ablation fir-base fir-opt  # 把会话加进一个组
akita exp archive -n lab1 ablation              # 先看看会做什么
akita exp archive lab1 ablation --results 1015_abl_wg32k_base_3e9a41c
akita exp archive lab1                          # 列出已归档的内容
akita exp unarchive lab1 ablation               # 撤销归档
```

归档会把已结束会话的日志移到 `logs/archived/`，并在网页上默认隐藏它们和它们的
结果目录。归档从不移动或删除结果。会话的结果目录是指名字出现在它命令里的那些；
其他目录（比如脚本写出的 campaign）用 `--results` 指定，`archive` 也会列出日志里
提到的目录作为提示。只要指定的会话中有一个还在运行，就什么都不会改。

### 结果命名

如果你的 `exp.json` 定义了命名规则（见第 7 节），可以用这些命令保持结果名一致：

```bash
akita exp name check 1015_abl_wg32k_base_3e9a41c   # 不符合规则时退出码为 1
akita exp name plan -H lab1 results                  # 旧名 -> 新名
akita exp name apply -H lab1 results --dry-run
akita exp record results/1015_abl_wg32k_base_3e9a41c "Command=./sim -benchmark=fir"
```

`record` 在结果目录里写 `run.json`：从目录名解析出的字段，加上你给出的属性。
`name apply` 用你配置的映射命令（`naming.legacy`）把旧命名的目录改成新名，并修正
目录里记录的路径。

## 6. 网页界面

`akita exp web` 轮询所有主机（或你指定的主机），在 `http://127.0.0.1:8765` 提供页面
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
退出码、信号、是否已重试或已取回。用 `akita exp web --balance` 启动时，页面还会列出调度
做了什么。

**什么情况算"疑似卡住"。**如果程序会报告进度（Akita monitor 或心跳文件，见第 8 节）：
模拟时间 `sim_stall_minutes`（默认 10）分钟不推进，或完成的工作量 `work_stall_minutes`
（默认 30）分钟不增加，或心跳 `sim_stall_minutes` 分钟没有更新，就判定为疑似卡住。
否则，CPU 使用率低于 `idle_cpu`%（默认 5）持续 `stall_minutes`（默认 30）分钟判定为
疑似卡住；CPU 在忙但日志和输出文件 `stall_minutes` 分钟没有变化时，标为"无新输出"。
这些参数以及轮询间隔 `interval`（默认 10 秒）都在用户配置里设置。

## 7. 用 exp.json 描述你的项目

所有字段都是可选的。没有 `exp.json` 时，会话和结果照样可以运行和监控，但结果名不会被
拆成字段，模拟器进程不会被识别；campaign 按默认标记（`EXPERIMENT_METADATA.json` 或
`campaign.json`）识别，用 `runs` 方式读取，并根据数据自动推测一种图。

```json
{
  "schema": "exp-project/1",
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
  {"schema": "exp-run/1", "name": "fir_m1",
   "fields": {"workload": "fir", "config": "optA"},
   "metrics": {"total_time": 1.2e-5, "wgs": 512}, "status": "ok"}
  ```

  可以再放一个 `campaign.json`，写上 `{"total": N}`，进度条就能显示计划的格子总数。
- `{"type": "command", "command": ["python3", "tools/cells.py"]}`：用于其他任何格式的
  campaign。这个命令在主机的工作区里运行，campaign 目录作为最后一个参数，输出格子的
  JSON（格式 `exp-cells/1`，见 [DESIGN.md](DESIGN.md)）：`dimensions`、可选的 `labels`
  和 `order`，以及 `cells`，每个格子有 `key`、`status`（`ok`、`failed`、`running`、
  `missing`、`bad`）和 `metrics`。`sweep` 部分用来描述敏感性 campaign。

**`plots`** 把格子变成图。`kind: "ratio"` 时，对越小越好的指标（`lower_is_better`），
每个值 = baseline ÷ 该格子，否则 = 该格子 ÷ baseline。baseline 是 `series` 键等于
`baseline`、其余键都相同的那个格子；只有 `match` 里的每个指标都相等时才计算（比如
模拟的工作量相同）。`summary: "gmean"` 会加上几何平均。`kind: "sweep"` 在一次扫描的
各个点上画同样的比值。

**`build`** 列出 `akita exp build` 构建的二进制：`name`、`command` 运行的目录 `dir`
（相对于项目），以及额外的 `env`。命令里的 `{out}` 是输出文件；`GOOS`/`GOARCH` 来自
`--target`（默认 `linux/amd64`）。

```json
"build": {"binaries": [{"name": "sim", "dir": "cmd/sim",
            "command": ["go", "build", "-o", "{out}", "."],
            "env": {"CGO_ENABLED": "0"}}]},
"collect": {"exclude": ["*_trace.csv"]}
```

**`collect`** 列出 `akita exp collect` 不取回的 rsync 规则（本机用不到的大文件）。

## 8. 配合 Akita 模拟器使用

`akita exp` 对模拟器没有任何要求，但 Akita 模拟器能让它显示更多信息：

- **实时进度。**如果模拟器打开了 monitor
  （`sim.MakeBuilder().WithMonitor(monitoring.NewMonitor())`），它会输出
  `Monitoring simulation with http://localhost:<port>`。`akita exp` 在会话日志里找到这一
  行，读取 monitor 的进度条和运行信息，所以网页上显示的进度条和 AkitaRTM 一样，卡住
  判断也依据它。会话详情里有指向模拟器自己 monitor 页面的链接。
- **运行元数据。**Akita 会写出 `akita_sim_<id>.sqlite3`（或用 `WithOutputFileName`
  指定的名字）。把它写进结果目录，它的 `exec_info` 表（命令、开始和结束时间、工作目录）
  就会显示为这个结果的属性。数据库以只读方式打开。
- **心跳文件。**任何程序不用 monitor 也能报告进度：环境变量 `EXP_HEARTBEAT` 是
  `logs/<名字>.heartbeat` 的路径；程序定期重写这个文件，内容是一个 JSON 对象，例如

  ```json
  {"pid": 12345, "wall": 1792111137.5, "finished": false,
   "sim_time_ns": 1.5e6,
   "progress": [{"id": "wg", "name": "Work-groups", "total": 32768, "finished": 1200, "in_progress": 64}]}
  ```

  `wall` 是这次更新的 Unix 时间。结束时把 `finished` 设为 `true`。

## 9. 从旧的 exp 脚本迁移

`akita exp` 沿用 `exp` 脚本在磁盘上的目录结构，已有的工作区不需要任何迁移：

1. 在本机和每台主机上安装 `akita`（见第 2 节）。
2. 把原来的主机写进 `~/.config/akita/exp.json`；默认的 root `~/exp` 就是原来的目录。
3. 把 `exp` 换成 `akita exp`。命令和选项（`-g`、`-n`、`--results`、`EXP_GROUP`、
   `EXP_NO_SYNC`）都一样。

不同之处：不指定主机时，命令作用于配置里的所有主机；命令在子 shell 里运行，所以
`exit N` 会被记录，命令里也可以用单引号；`stop` 写入 `[stopped] <日期>  akita exp stop`，
并拒绝停止没在运行的会话；监控的判定阈值改成用户配置里的字段；主机上需要的是 `akita`，
不再是 `python3`；新增了 `logs/<名字>.job` 文件。

## 10. 常见问题

| 问题 | 检查什么 |
|---|---|
| 网页上某台主机显示错误 | 能否不输密码执行 `ssh <地址> true`？主机上有没有 `akita`（`ssh <地址> akita exp probe --json`）？如果在别的路径，在主机配置里设置 `"akita"`。 |
| `run` 提示会话已在运行 | 这个名字正被使用。换个名字，或先 `stop` 旧的。 |
| 手动能跑，作为会话就失败 | 会话在工作区里用登录 shell（`bash -l`）启动。检查相对于项目根目录的路径，以及只在交互式 shell 里设置的环境变量。 |
| `sync` 没有复制某个文件 | 它被 `.gitignore`、`sync.exclude` 或 `.expignore` 忽略了，或者位于 `logs/`、`results/`、`remote/` 下。 |
| 会话显示 `died` | 它没有退出标记就结束了：主机重启、进程被杀，或 SLURM 结束了作业（`status` 会显示 SLURM 的状态）。 |
| 结果列表里出现"未解析"的名字 | 它们不符合 `naming`。`akita exp name check <名字>` 会说明哪个字段不符合；`name plan` / `apply` 可以批量改名。 |
| campaign 没有图 | 它被识别了吗（`campaigns.detect`）？适配器能输出格子吗（在主机上运行 `akita exp probe campaign ...`）？`plots` 里写的指标，格子里有吗？ |
| `archive` 什么都没改 | 指定的会话中有一个还在运行，或者加了 `-n`。 |
| 排队的任务不启动 | 运行 `akita exp queue <主机>`：agent 在运行吗？槽位满了吗（队列之外启动的模拟器也算）？MemAvailable 减去它的 `mem_gb` 是否低于 `mem_reserve_gb`，或者有任务被暂停（这时不会启动新任务）？任务是否避开了这台主机？agent 的日志在主机上的 `<root>/.akita-exp/agent.log`。 |
| `submit` 提示某个二进制没有构建 | 先运行 `akita exp build`；`{bin:<名字>}` 必须是 `exp.json` 里 `build` 列出的二进制。 |
| `balance` 什么都没移动 | 目标主机需要 agent 在运行、队列为空，并且有空闲的槽位和内存；只移动当前项目的任务。 |
