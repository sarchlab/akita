package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/backend"
	"github.com/sarchlab/akita/v5/experiment/workspace"
	"github.com/spf13/cobra"
)

var expRunCmd = &cobra.Command{
	Use:   "run [-g group] [--no-sync] <host> <name> <command>...",
	Short: "Sync the project and start a command as a session on a host.",
	Long: `Sync the project to the host, then start the command in the project's
workspace as session <name>: in tmux (session exp-<project>-<name>), under
nohup setsid when tmux is missing, or as a batch job on SLURM hosts.
Output goes to logs/<name>.log, framed by "[start] ..." and "[exit N] ..."
lines; EXP_HEARTBEAT points to logs/<name>.heartbeat.

Quote the command, or put flags of akita exp before <host>: everything
after <name> is the command. EXP_GROUP sets the default group and
EXP_NO_SYNC=1 skips the sync (useful after one akita exp sync when
starting many sessions).`,
	Example: `  akita exp run server1 seed0 "python train.py --seed 0"
  akita exp run -g ablation cluster fir "./sim -benchmark=fir"`,
	Args: cobra.MinimumNArgs(3),
	RunE: runExpRun,
}

var expStopCmd = &cobra.Command{
	Use:   "stop <host> <name>",
	Short: "Stop a session (its log gets a \"[stopped]\" line).",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadExpEnv(cmd)
		if err != nil {
			return err
		}

		b, err := env.backend(args[0])
		if err != nil {
			return err
		}

		ctx, cancel := commandContext(cmd)
		defer cancel()

		if err := b.Stop(ctx, env.project, args[1]); err != nil {
			return err
		}

		env.printf(">> stopped %s:%s\n", args[0], args[1])

		return nil
	},
}

var expLogCmd = &cobra.Command{
	Use:   "log <host> <name> [-f]",
	Short: "Show the end of a session's log (archived sessions too).",
	Args:  cobra.ExactArgs(2),
	RunE:  runExpLog,
}

var expStatusCmd = &cobra.Command{
	Use:   "status [host...]",
	Short: "List the project's sessions on hosts with their last log lines.",
	Long: `List the project's sessions on each host (all hosts by default): state,
group, start time, and the last lines of the most recent logs. States:
running, queued (SLURM), ok, failed (non-zero exit), stopped, and died
(ended without an exit line). Needs akita on the host; without it, the
tmux sessions and log tails are shown.`,
	RunE: runExpStatus,
}

var expGroupCmd = &cobra.Command{
	Use:   "group <host> <group|-> <name>...",
	Short: "Put sessions into a group (\"-\" removes them from their group).",
	Args:  cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadExpEnv(cmd)
		if err != nil {
			return err
		}

		b, err := env.backend(args[0])
		if err != nil {
			return err
		}

		ctx, cancel := commandContext(cmd)
		defer cancel()

		return akitaOut(ctx, b, env.out, append([]string{"exp", "_local", "group",
			b.Workspace(env.project), args[1], "--"}, args[2:]...)...)
	},
}

func init() {
	expRunCmd.Flags().StringP("group", "g", "", "put the session into a group (default $EXP_GROUP)")
	expRunCmd.Flags().Bool("no-sync", false, "do not sync the project first (or EXP_NO_SYNC=1)")
	expRunCmd.Flags().SetInterspersed(false)
	expLogCmd.Flags().BoolP("follow", "f", false, "follow the log (Ctrl+C stops following, not the session)")
	expLogCmd.Flags().IntP("lines", "n", 50, "number of lines")
	expStatusCmd.Flags().IntP("lines", "n", 3, "log lines per session")
	expStatusCmd.Flags().BoolP("all", "a", false, "list archived sessions too")
	expCmd.AddCommand(expRunCmd, expStopCmd, expLogCmd, expStatusCmd, expGroupCmd)
}

func runExpRun(cmd *cobra.Command, args []string) error {
	env, err := loadExpEnv(cmd)
	if err != nil {
		return err
	}

	group, _ := cmd.Flags().GetString("group")
	if !cmd.Flags().Changed("group") {
		group = os.Getenv("EXP_GROUP")
	}

	noSync, _ := cmd.Flags().GetBool("no-sync")
	noSync = noSync || os.Getenv("EXP_NO_SYNC") == "1"

	b, err := env.backend(args[0])
	if err != nil {
		return err
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	if !noSync {
		if err := syncTo(ctx, env, b); err != nil {
			return err
		}
	}

	spec := backend.LaunchSpec{
		Project: env.project, Name: args[1], Group: group, Command: strings.Join(args[2:], " "),
	}

	job, err := b.Launch(ctx, spec)
	if err != nil {
		return err
	}

	env.printf(">> started %s on %s (%s)%s; log: %s/%s\n", spec.Name, b.Host().Name,
		launchedAs(b, spec, job), groupSuffix(group), b.Workspace(env.project),
		workspace.ScriptFile(spec.Name, workspace.LogExt))

	return nil
}

// launchedAs tells how a session runs: its tmux session, or its SLURM job.
func launchedAs(b backend.Backend, spec backend.LaunchSpec, job string) string {
	switch {
	case b.Host().Type != experiment.HostSlurm:
		return "tmux session " + workspace.TmuxSessionName(spec.Project, spec.Name)
	case job == "":
		return "SLURM job"
	default:
		return "SLURM job " + job
	}
}

func groupSuffix(g string) string {
	if g == "" {
		return ""
	}

	return ", group " + g
}

func runExpLog(cmd *cobra.Command, args []string) error {
	env, err := loadExpEnv(cmd)
	if err != nil {
		return err
	}

	if err := workspace.ValidateSessionName(args[1]); err != nil {
		return err
	}

	b, err := env.backend(args[0])
	if err != nil {
		return err
	}

	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")

	tail := "tail -n " + strconv.Itoa(lines)
	if follow {
		tail += " -f"
	}

	active := workspace.ShellQuote(workspace.ScriptFile(args[1], workspace.LogExt))
	archived := workspace.ShellQuote(workspace.ScriptFile(workspace.ArchivedDir+"/"+args[1], workspace.LogExt))
	script := "if [ -f " + active + " ]; then lf=" + active + "; " +
		"elif [ -f " + archived + " ]; then lf=" + archived + "; " +
		"else echo " + workspace.ShellQuote("no log for session "+args[1]) + " >&2; exit 1; fi\n" +
		tail + " \"$lf\"\n"

	ctx, cancel := commandContext(cmd)
	defer cancel()

	err = b.Stream(ctx, env.project, script, env.out, env.errOut)
	if follow && ctx.Err() != nil {
		return nil
	}

	return err
}

func runExpStatus(cmd *cobra.Command, args []string) error {
	env, err := loadExpEnv(cmd)
	if err != nil {
		return err
	}

	bs, err := env.backends(args)
	if err != nil {
		return err
	}

	lines, _ := cmd.Flags().GetInt("lines")
	all, _ := cmd.Flags().GetBool("all")

	ctx, cancel := commandContext(cmd)
	defer cancel()

	for _, b := range bs {
		env.printf("===== %s =====\n", b.Host().Name)

		sessions, err := remoteSessions(ctx, b, env.project, all, lines)
		if err != nil {
			env.printf("(akita exp on %s: %v; showing raw status)\n", b.Host().Name, err)
			rawStatus(ctx, env, b)

			continue
		}

		printSessions(env, sessions, lines)
	}

	return nil
}

func remoteSessions(ctx context.Context, b backend.Backend, project string, archived bool, lines int,
) ([]experiment.Session, error) {
	args := []string{
		"exp", "_local", "sessions", b.Workspace(project), "--project", project,
		"--tail", strconv.Itoa(lines),
	}
	if archived {
		args = append(args, "--archived")
	}

	data, err := b.Akita(ctx, args...)
	if err != nil {
		return nil, err
	}

	var sessions []experiment.Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("bad output: %w", err)
	}

	return sessions, nil
}

// tailedSessions is how many of the most recent sessions status shows the
// log tails of.
const tailedSessions = 5

// printSessions prints a table of sessions, then the tails of the five
// most recent ones.
func printSessions(env *expEnv, sessions []experiment.Session, lines int) {
	if len(sessions) == 0 {
		env.printf("(no sessions)\n")
		return
	}

	tw := tabwriter.NewWriter(env.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tNAME\tGROUP\tSTARTED\tDURATION")

	for _, s := range sessions {
		name := s.Name
		if s.Archived {
			name += " (archived)"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", stateLabel(s), name, dash(s.Group),
			fmtTime(s.Start), duration(s))
	}

	_ = tw.Flush()

	for i, s := range sessions {
		if i >= tailedSessions || lines <= 0 {
			break
		}

		env.printf("--- %s\n", s.Name)

		for _, l := range s.Tail {
			env.printf("%s\n", l)
		}
	}
}

func stateLabel(s experiment.Session) string {
	if s.ExitCode != nil && s.State == "failed" {
		return fmt.Sprintf("failed(%d)", *s.ExitCode)
	}

	return s.State
}

func dash(s string) string {
	if s == "" {
		return "-"
	}

	return s
}

func fmtTime(t *float64) string {
	if t == nil {
		return "-"
	}

	return time.Unix(int64(*t), 0).Format("01-02 15:04")
}

func duration(s experiment.Session) string {
	if s.Start == nil {
		return "-"
	}

	end := float64(time.Now().Unix())
	if s.End != nil {
		end = *s.End
	} else if s.State != "running" && s.State != "queued" {
		end = s.MTime
	}

	return (time.Duration(end-*s.Start) * time.Second).String()
}

// rawStatus is the status without akita on the host: the tmux sessions
// of the project's sessions and the tails of the five newest logs.
func rawStatus(ctx context.Context, env *expEnv, b backend.Backend) {
	out, err := b.Shell(ctx, env.project, rawStatusScript(env.project))
	env.printf("%s", out)

	if err != nil {
		env.printf("(%v)\n", err)
	}
}

// rawStatusScript lists the `tmux ls` lines whose session name is exactly
// the tmux session name of a log in logs/ (a prefix would also match other
// projects whose names start with this one's), then the newest log tails.
func rawStatusScript(project string) string {
	return "names=$(cd logs 2>/dev/null && for f in *.log; do [ -e \"$f\" ] && " +
		"printf '%s\\n' \"${f%.log}\"; done | tr .: __ | " +
		"while IFS= read -r n; do printf '%s%s\\n' " +
		workspace.ShellQuote(workspace.TmuxSessionName(project, "")) + " \"$n\"; done)\n" +
		"tmux ls 2>/dev/null | while IFS= read -r l; do " +
		"printf '%s\\n' \"$names\" | grep -Fxq -- \"${l%%:*}\" && printf '%s\\n' \"$l\"; done | " +
		"grep . || echo '(no running tmux sessions)'\n" +
		"cd logs 2>/dev/null && for f in $(ls -t *.log 2>/dev/null | head -5); do " +
		"echo \"--- $f\"; tail -n 3 \"$f\"; done; exit 0\n"
}
