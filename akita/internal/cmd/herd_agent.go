package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/sarchlab/akita/v5/herd/queue"
	"github.com/spf13/cobra"
)

// agentIntervalEnv overrides the default --interval of agent run (tests).
const agentIntervalEnv = "AKITA_HERD_AGENT_INTERVAL"

// agentStartWait bounds the wait for a started agent's first heartbeat.
const agentStartWait = 15 * time.Second

var herdAgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start, stop, or show the queue agent of a host.",
	Long: `The agent of a host runs its queue (<root>/.akita-herd/): it starts pending
tasks while slots and memory allow, pauses (SIGSTOP) and resumes (SIGCONT)
the process groups of its tasks under memory pressure, and records how
they ended. One agent runs per root. Its settings are the host's "queue"
entry in the user config. Stopping the agent leaves its tasks running; the
next agent adopts them.`,
}

var herdAgentRunCmd = &cobra.Command{
	Use:   "run --root DIR --host NAME [settings]",
	Short: "Run the agent of a root on this machine (agent start runs this).",
	Args:  cobra.NoArgs,
	RunE:  runAgent,
}

var herdAgentStartCmd = &cobra.Command{
	Use:   "start <host>",
	Short: "Start the agent of a host (tmux session herd-agent-<host>).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		env, b, ctx, cancel, err := queueHost(cmd, args[0])
		if err != nil {
			return err
		}
		defer cancel()

		fake, _ := cmd.Flags().GetString("fake-mem-file")

		info, err := ensureAgent(ctx, env, b, fake)
		if err != nil {
			return err
		}

		env.printf(">> agent of %s runs: pid %d, root %s, %d slots\n", args[0], info.PID,
			backend.Root(b), info.Slots)

		return nil
	},
}

var herdAgentStopCmd = &cobra.Command{
	Use:   "stop <host> | --root DIR",
	Short: "Stop the agent of a host; its tasks keep running.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if root, _ := cmd.Flags().GetString("root"); root != "" {
			info, err := queue.Open(root).StopAgent(agentStartWait)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), ">> stopped the agent (pid %d) of %s\n", info.PID, root)

			return nil
		}

		if len(args) != 1 {
			return errors.New("usage: akita herd agent stop <host>")
		}

		env, b, ctx, cancel, err := queueHost(cmd, args[0])
		if err != nil {
			return err
		}
		defer cancel()

		return akitaOut(ctx, b, env.out, "herd", "agent", "stop", "--root", backend.Root(b))
	},
}

var herdAgentStatusCmd = &cobra.Command{
	Use:   "status <host> | --root DIR [--json]",
	Short: "Show the agent of a host.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if root, _ := cmd.Flags().GetString("root"); root != "" {
			info, alive := queue.Open(root).Agent()
			if j, _ := cmd.Flags().GetBool("json"); j {
				return writeJSONTo(cmd.OutOrStdout(), agentState{Agent: info, Alive: alive})
			}

			printAgent(cmd.OutOrStdout(), root, info, alive)

			return nil
		}

		if len(args) != 1 {
			return errors.New("usage: akita herd agent status <host>")
		}

		env, b, ctx, cancel, err := queueHost(cmd, args[0])
		if err != nil {
			return err
		}
		defer cancel()

		st, err := remoteAgent(ctx, b)
		if err != nil {
			return err
		}

		printAgent(env.out, args[0]+":"+backend.Root(b), st.Agent, st.Alive)

		return nil
	},
}

func init() {
	f := herdAgentRunCmd.Flags()
	f.String("root", "", "the root whose queue the agent runs")
	f.String("host", "", "the host's name (attempts, avoid_hosts)")
	f.Int("slots", 0, "concurrent tasks (default: CPUs / 2)")
	f.Float64("mem-reserve-gb", 0, "start a task only if MemAvailable - its mem_gb stays above this")
	f.Float64("mem-pause-gb", 0, "pause a task when MemAvailable is below this")
	f.Float64("mem-resume-gb", 0, "resume a paused task when MemAvailable is above this")
	f.StringSlice("avoid-signals", nil, "signals that make a task retryable on another host")
	f.Duration("interval", queue.DefaultInterval, "time between rounds")
	f.String("fake-mem-file", "", "read MemAvailable (GB) from this file (tests)")
	_ = f.MarkHidden("fake-mem-file")
	_ = herdAgentRunCmd.MarkFlagRequired("root")

	herdAgentStartCmd.Flags().String("fake-mem-file", "", "pass --fake-mem-file to the agent (tests)")
	_ = herdAgentStartCmd.Flags().MarkHidden("fake-mem-file")

	for _, c := range []*cobra.Command{herdAgentStopCmd, herdAgentStatusCmd} {
		c.Flags().String("root", "", "act on the agent of this root on this machine")
	}

	herdAgentStatusCmd.Flags().Bool("json", false, "print JSON (with --root)")
	herdAgentCmd.AddCommand(herdAgentRunCmd, herdAgentStartCmd, herdAgentStopCmd, herdAgentStatusCmd)
	herdCmd.AddCommand(herdAgentCmd)
}

func runAgent(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	root, _ := f.GetString("root")
	host, _ := f.GetString("host")
	interval, _ := f.GetDuration("interval")
	if d, err := time.ParseDuration(os.Getenv(agentIntervalEnv)); err == nil && !f.Changed("interval") {
		interval = d
	}
	fake, _ := f.GetString("fake-mem-file")

	cfg := queue.Config{Host: host, Version: akitaVersion()}
	cfg.Slots, _ = f.GetInt("slots")
	cfg.MemReserveGB, _ = f.GetFloat64("mem-reserve-gb")
	cfg.MemPauseGB, _ = f.GetFloat64("mem-pause-gb")
	cfg.MemResumeGB, _ = f.GetFloat64("mem-resume-gb")
	cfg.AvoidSignals, _ = f.GetStringSlice("avoid-signals")

	if host == "" {
		cfg.Host, _ = os.Hostname()
	}

	q := queue.Open(root)
	if err := os.MkdirAll(filepath.Join(q.Root(), queue.Dir), 0o755); err != nil {
		return err
	}

	logFile, err := os.OpenFile(filepath.Join(q.Root(), queue.Dir, queue.AgentLog),
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	ctx, cancel := commandContext(cmd)
	defer cancel()

	a := queue.NewAgent(root, cfg, queue.HostSystem{FakeMemFile: fake},
		io.MultiWriter(cmd.OutOrStdout(), logFile))

	return a.Run(ctx, interval)
}

// agentArgs are the akita arguments that run a host's agent.
func agentArgs(h herd.HostConfig, root, fakeMem string) []string {
	args := []string{"herd", "agent", "run", "--root", root, "--host", h.Name}

	if q := h.Queue; q != nil {
		num := func(flag string, v float64) {
			if v > 0 {
				args = append(args, flag, strconv.FormatFloat(v, 'g', -1, 64))
			}
		}

		num("--slots", float64(q.Slots))
		num("--mem-reserve-gb", q.MemReserveGB)
		num("--mem-pause-gb", q.MemPauseGB)
		num("--mem-resume-gb", q.MemResumeGB)

		if len(q.AvoidSignals) > 0 {
			args = append(args, "--avoid-signals", strings.Join(q.AvoidSignals, ","))
		}
	}

	if fakeMem != "" {
		args = append(args, "--fake-mem-file", fakeMem)
	}

	return args
}

// agentState is what `agent status --root --json` prints.
type agentState struct {
	Agent *herd.AgentInfo `json:"agent"`
	Alive bool            `json:"alive"`
}

func remoteAgent(ctx context.Context, b backend.Backend) (agentState, error) {
	var st agentState

	data, err := b.Akita(ctx, "herd", "agent", "status", "--root", backend.Root(b), "--json")
	if err != nil {
		return st, err
	}

	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("%s: bad agent status: %w", b.Host().Name, err)
	}

	return st, nil
}

// ensureAgent starts the agent of a host unless it runs, and waits for its
// first heartbeat.
func ensureAgent(ctx context.Context, env *herdEnv, b backend.Backend, fakeMem string,
) (*herd.AgentInfo, error) {
	st, err := remoteAgent(ctx, b)
	if err != nil {
		return nil, err
	}

	if st.Alive {
		return st.Agent, nil
	}

	h := b.Host()
	env.printf(">> starting the agent of %s (tmux session %s)\n", h.Name, agentSession(h.Name))

	if err := b.StartAkita(ctx, agentSession(h.Name), agentArgs(h, backend.Root(b), fakeMem)...); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(agentStartWait)
	for time.Now().Before(deadline) {
		if st, err = remoteAgent(ctx, b); err == nil && st.Alive && st.Agent.Heartbeat > 0 {
			return st.Agent, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return nil, fmt.Errorf("the agent of %s did not start; see %s/%s/%s there", h.Name,
		backend.Root(b), queue.Dir, queue.AgentLog)
}

func agentSession(host string) string { return "herd-agent-" + host }

func printAgent(w io.Writer, where string, info *herd.AgentInfo, alive bool) {
	if info == nil {
		fmt.Fprintf(w, "%s: no agent has run\n", where)
		return
	}

	state := "running"
	if !alive {
		state = "not running"
	}

	age := time.Since(time.Unix(int64(info.Heartbeat), 0)).Round(time.Second)
	fmt.Fprintf(w, "%s: agent %s (pid %d, host %s), heartbeat %s ago\n", where, state, info.PID, info.Host, age)
	fmt.Fprintf(w, "  slots %d/%d (own %d, simulators found %d), MemAvailable %.1f GB "+
		"(reserve %g, pause < %g, resume > %g)\n", max(info.Own, info.External), info.Slots, info.Own,
		info.External, info.MemAvailGB, info.MemReserveGB, info.MemPauseGB, info.MemResumeGB)

	if info.Error != "" {
		fmt.Fprintf(w, "  error: %s\n", info.Error)
	}
}

// queueHost loads the env and the backend of a host that has a queue (not
// a SLURM host), with a command context.
func queueHost(cmd *cobra.Command, host string,
) (*herdEnv, backend.Backend, context.Context, context.CancelFunc, error) {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	b, err := env.backend(host)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if b.Host().Type == herd.HostSlurm {
		return nil, nil, nil, nil, fmt.Errorf("%s is a SLURM host; it has no queue (use run)", host)
	}

	ctx, cancel := commandContext(cmd)

	return env, b, ctx, cancel, nil
}
