package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/sarchlab/akita/v5/herd/workspace"
	"github.com/spf13/cobra"
)

// Defaults of submit and balance.
const (
	defaultPriority = 5
	balanceEvery    = 60 * time.Second
	maxTaskLine     = 1 << 20
)

// binRe finds the {bin:<name>} references of a command.
var binRe = regexp.MustCompile(`\{bin:([^}]+)\}`)

var herdSubmitCmd = &cobra.Command{
	Use:   "submit [-g group] [--priority N] [--mem GB] [--host H|--hosts H,H] [--file tasks.jsonl] <name> <command>...",
	Short: "Queue a command (or a file of them) on a host's queue.",
	Long: `Queue tasks on hosts. Each task runs as session <name> in the project's
workspace when the host's agent has a free slot and memory for it
(MemAvailable - mem_gb >= mem_reserve_gb); a smaller priority starts
first. {bin:<name>} in the command is replaced by the path of the latest
build of that binary (akita herd build), which is uploaded to the host if
it does not have it.

With --hosts (or without a host: the hosts with a "queue" entry in the
user config), each task goes to the host that can start the most tasks
like it now (free slots, limited by the memory left after the pending
tasks), ties to the one with fewer pending tasks. submit syncs the project
to each chosen host (unless --no-sync) and starts its agent if it does not
run. --file reads one JSON task per line: {"name", "cmd", "group",
"priority", "mem_gb"}; flags give the defaults.`,
	Example: `  akita herd submit --host server1 --mem 20 1019_diag_x_base_a41f7e2 "./run.sh {bin:sim} x"
  akita herd submit --hosts server1,server2 -g round2 --file tasks.jsonl`,
	RunE: runSubmit,
}

var herdCancelCmd = &cobra.Command{
	Use:   "cancel <host> <name>",
	Short: "Remove a pending task from a host's queue (a running one: stop).",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		env, b, ctx, cancel, err := queueHost(cmd, args[0])
		if err != nil {
			return err
		}
		defer cancel()

		if _, err := takeTask(ctx, b, env.project, args[1]); err != nil {
			if strings.Contains(err.Error(), queue.ErrNotPending.Error()) {
				return fmt.Errorf("%s is not pending on %s; if it runs, use akita herd stop %s %s",
					args[1], args[0], args[0], args[1])
			}

			return err
		}

		env.printf(">> cancelled %s on %s\n", args[1], args[0])

		return nil
	},
}

var herdBalanceCmd = &cobra.Command{
	Use:   "balance [--watch] [--dry-run] [host...]",
	Short: "Move pending tasks to idle hosts and retry tasks that died of a host's signal.",
	Long: `Balance the project's tasks over the hosts' queues (all SSH and local hosts
by default). First, a task that ended with one of its host's
avoid_signals (e.g. SIGBUS on a host with bad memory) is resubmitted, at
most 2 times, to another host whose agent runs and that the task does not
avoid; the host it failed on is added to its avoid_hosts. Then, while a
host has pending tasks it cannot start and another host's agent runs with
free slots and an empty queue, pending tasks move there one by one: taken
from the source (atomically; a task the agent started meanwhile stays),
put on the target after syncing the project and uploading binaries, and
put back on the source if that fails. --watch repeats every minute.`,
	RunE: runBalance,
}

func init() {
	f := herdSubmitCmd.Flags()
	f.StringP("group", "g", "", "group of the tasks (default $HERD_GROUP)")
	f.Int("priority", defaultPriority, "priority; a smaller number starts first (0-9999)")
	f.Float64("mem", 0, "memory the task needs, in GB")
	f.String("host", "", "queue on this host")
	f.StringSlice("hosts", nil, "place each task on the best of these hosts")
	f.String("file", "", "read tasks from a JSON-lines file")
	f.Bool("no-sync", false, "do not sync the project first")
	f.SetInterspersed(false)
	herdBalanceCmd.Flags().Bool("watch", false, "repeat every minute")
	herdBalanceCmd.Flags().BoolP("dry-run", "n", false, "show what would move")
	herdCmd.AddCommand(herdSubmitCmd, herdCancelCmd, herdBalanceCmd)
}

func runSubmit(cmd *cobra.Command, args []string) error {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return err
	}

	tasks, err := submitTasks(cmd, env, args)
	if err != nil {
		return err
	}

	bs, err := submitHosts(cmd, env)
	if err != nil {
		return err
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	loads := make([]queue.HostLoad, len(bs))
	for i, b := range bs {
		s, err := remoteQueue(ctx, b)
		if err != nil {
			return err
		}

		loads[i] = queue.LoadOf(b.Host().Name, s, hostQueueConfig(b))
	}

	perHost := map[int][]*herd.Task{}

	for _, t := range tasks {
		i := queue.Place(loads, t)
		if i < 0 {
			return fmt.Errorf("task %s avoids every host", t.Name)
		}

		perHost[i] = append(perHost[i], t)
	}

	noSync, _ := cmd.Flags().GetBool("no-sync")
	s := &sender{env: env, noSync: noSync, synced: map[string]bool{}}

	for i, b := range bs {
		if len(perHost[i]) == 0 {
			continue
		}

		if err := s.send(ctx, b, perHost[i]); err != nil {
			return err
		}

		env.printf(">> queued %d tasks on %s\n", len(perHost[i]), b.Host().Name)

		if _, err := ensureAgent(ctx, env, b, ""); err != nil {
			return err
		}
	}

	return nil
}

// submitTasks are the tasks of the arguments or of --file, with the
// defaults of the flags, checked and with their binaries.
func submitTasks(cmd *cobra.Command, env *herdEnv, args []string) ([]*herd.Task, error) {
	f := cmd.Flags()
	file, _ := f.GetString("file")

	def := herd.Task{Project: env.project, Submitted: herd.UnixSeconds(time.Now()), SubmittedBy: whoAmI()}
	def.Group, _ = f.GetString("group")
	def.Priority, _ = f.GetInt("priority")
	def.MemGB, _ = f.GetFloat64("mem")

	if !f.Changed("group") {
		def.Group = getenvLegacy("HERD_GROUP", "EXP_GROUP")
	}

	var tasks []*herd.Task

	switch {
	case file != "" && len(args) > 0:
		return nil, errors.New("give either --file or <name> <command>")
	case file != "":
		var err error
		if tasks, err = readTaskFile(file, def); err != nil {
			return nil, err
		}
	case len(args) < 2:
		return nil, errors.New("usage: akita herd submit [flags] <name> <command>...")
	default:
		t := def
		t.Name, t.Cmd = args[0], strings.Join(args[1:], " ")
		tasks = []*herd.Task{&t}
	}

	builds, err := queue.LoadBuilds(env.projectDir)
	if err != nil {
		return nil, err
	}

	for _, t := range tasks {
		if err := checkTask(env, t, builds); err != nil {
			return nil, err
		}
	}

	return tasks, nil
}

func readTaskFile(path string, def herd.Task) ([]*herd.Task, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tasks []*herd.Task

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, maxTaskLine), maxTaskLine)

	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		t := def
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}

		t.Project = def.Project
		tasks = append(tasks, &t)
	}

	return tasks, sc.Err()
}

// checkTask checks a task's name (also against the project's naming rule)
// and adds the latest builds of the binaries its command names.
func checkTask(env *herdEnv, t *herd.Task, builds map[string]herd.Binary) error {
	if err := queue.ValidateTask(t); err != nil {
		return err
	}

	if env.projectCfg != nil && env.projectCfg.Naming != nil {
		if err := herd.CheckName(t.Name, env.projectCfg.Naming); err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
	}

	for _, m := range binRe.FindAllStringSubmatch(t.Cmd, -1) {
		b, ok := builds[m[1]]
		if !ok {
			return fmt.Errorf("%s: no build of binary %q; run akita herd build", t.Name, m[1])
		}

		if t.Binaries == nil {
			t.Binaries = map[string]herd.Binary{}
		}

		t.Binaries[m[1]] = b
	}

	return nil
}

// submitHosts are the hosts of --host or --hosts, or those with a queue
// entry in the user config.
func submitHosts(cmd *cobra.Command, env *herdEnv) ([]backend.Backend, error) {
	host, _ := cmd.Flags().GetString("host")
	hosts, _ := cmd.Flags().GetStringSlice("hosts")

	switch {
	case host != "" && len(hosts) > 0:
		return nil, errors.New("give either --host or --hosts")
	case host != "":
		hosts = []string{host}
	case len(hosts) == 0:
		for _, h := range env.user.Hosts {
			if h.Queue != nil && h.Type != herd.HostSlurm {
				hosts = append(hosts, h.Name)
			}
		}

		if len(hosts) == 0 {
			return nil, errors.New("name a host with --host or --hosts (no host has a \"queue\" entry)")
		}
	}

	return queueBackends(env, hosts)
}

func hostQueueConfig(b backend.Backend) herd.QueueConfig {
	if q := b.Host().Queue; q != nil {
		return *q
	}

	return herd.QueueConfig{}
}

func whoAmI() string {
	name := "?"
	if u, err := user.Current(); err == nil {
		name = u.Username
	}

	host, _ := os.Hostname()

	return name + "@" + host
}

// sender puts tasks on hosts: it syncs the project to a host once, and
// uploads the binaries the tasks name.
type sender struct {
	env    *herdEnv
	noSync bool
	synced map[string]bool
}

func (s *sender) send(ctx context.Context, b backend.Backend, tasks []*herd.Task) error {
	name := b.Host().Name
	if !s.noSync && !s.synced[name] {
		if err := syncTo(ctx, s.env, b); err != nil {
			return err
		}

		s.synced[name] = true
	}

	uploaded := map[string]bool{}

	for _, t := range tasks {
		for bn, bin := range t.Binaries {
			if uploaded[bin.SHA] {
				continue
			}

			local := queue.LocalBinary(s.env.projectDir, bin)
			if _, err := os.Stat(local); err != nil {
				return fmt.Errorf("binary %s (%s) of %s is not in %s; rebuild it", bn, bin.ShortSHA(), t.Name,
					queue.Dir+"/"+queue.BinDir)
			}

			if err := b.Upload(ctx, local, queue.HostBinaryPath(backend.Root(b), bin)); err != nil {
				return err
			}

			uploaded[bin.SHA] = true
		}
	}

	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}

	_, err = b.Akita(ctx, "herd", "queue", "put", "--root", backend.Root(b), string(data))

	return err
}

func takeTask(ctx context.Context, b backend.Backend, project, name string) (*herd.Task, error) {
	if err := workspace.ValidateSessionName(name); err != nil {
		return nil, err
	}

	data, err := b.Akita(ctx, "herd", "queue", "take", "--root", backend.Root(b), project, name)
	if err != nil {
		return nil, err
	}

	t := &herd.Task{}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("%s: bad task: %w", b.Host().Name, err)
	}

	return t, nil
}

func runBalance(cmd *cobra.Command, args []string) error {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return err
	}

	bs, err := queueBackends(env, args)
	if err != nil {
		return err
	}

	watch, _ := cmd.Flags().GetBool("watch")
	dry, _ := cmd.Flags().GetBool("dry-run")

	ctx, cancel := commandContext(cmd)
	defer cancel()

	report := func(line string) { env.printf("%s %s\n", time.Now().Format("15:04:05"), line) }

	for {
		if n := balanceOnce(ctx, env, bs, dry, report); n == 0 && !watch {
			report("nothing to move")
		}

		if !watch {
			return nil
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(balanceEvery):
		}
	}
}

// balanceOnce runs one round of balance and reports each step; it returns
// the number of planned retries and moves.
func balanceOnce(ctx context.Context, env *herdEnv, bs []backend.Backend, dry bool, report func(string)) int {
	byName := map[string]backend.Backend{}
	statuses := make([]*herd.QueueStatus, len(bs))
	loads := make([]queue.HostLoad, len(bs))

	for i, b := range bs {
		byName[b.Host().Name] = b

		s, err := remoteQueue(ctx, b)
		if err != nil {
			report(fmt.Sprintf("%s: %v", b.Host().Name, err))
		}

		statuses[i], loads[i] = s, queue.LoadOf(b.Host().Name, s, hostQueueConfig(b))
	}

	retries, moves := queue.PlanBalance(loads, statuses, env.project)
	s := &sender{env: env, synced: map[string]bool{}}

	for _, r := range retries {
		what := fmt.Sprintf("retry %s (%s on %s) -> %s", r.Task.Name, r.Task.Signal, r.From, r.To)
		if dry {
			report("would " + what)
			continue
		}

		t := r.Task
		if err := s.send(ctx, byName[r.To], []*herd.Task{&t}); err != nil {
			report(what + ": " + err.Error())
			continue
		}

		src := byName[r.From]
		if _, err := src.Akita(ctx, "herd", "queue", "mark", "--root", backend.Root(src), t.Project, t.Name,
			"--retried", r.To); err != nil {
			report(what + ": marking the old task: " + err.Error())
		}

		report(what)
	}

	for _, m := range moves {
		what := fmt.Sprintf("move %s: %s -> %s", m.Task.Name, m.From, m.To)
		if dry {
			report("would " + what)
			continue
		}

		report(moveTask(ctx, s, byName[m.From], byName[m.To], m.Task.Project, m.Task.Name, what))
	}

	return len(retries) + len(moves)
}

// moveTask takes a pending task from src and puts it on dst, putting it
// back on src if that fails; it returns the line to report.
func moveTask(ctx context.Context, s *sender, src, dst backend.Backend, project, name, what string) string {
	t, err := takeTask(ctx, src, project, name)
	if err != nil {
		return what + ": skipped: " + err.Error()
	}

	if err := s.send(ctx, dst, []*herd.Task{t}); err != nil {
		if perr := s.sendNoSync(ctx, src, t); perr != nil {
			return fmt.Sprintf("%s: FAILED (%v), and putting it back on %s failed: %v; the task is:\n%s",
				what, err, src.Host().Name, perr, taskJSON(t))
		}

		return what + ": failed, put back: " + err.Error()
	}

	return what
}

func (s *sender) sendNoSync(ctx context.Context, b backend.Backend, t *herd.Task) error {
	back := &sender{env: s.env, noSync: true, synced: map[string]bool{}}
	return back.send(ctx, b, []*herd.Task{t})
}

func taskJSON(t *herd.Task) string {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Sprintf("(task %s: %v)", t.Name, err)
	}

	return string(data)
}
