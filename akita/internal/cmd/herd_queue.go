package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/spf13/cobra"
)

// queueDoneShown is how many ended tasks `queue` lists per host.
const queueDoneShown = 10

var herdQueueCmd = &cobra.Command{
	Use:   "queue [host...]",
	Short: "Show the queues of hosts: agent, pending, running, paused, and done tasks.",
	Long: `Show the queue of each host (all SSH and local hosts by default): whether its
agent runs, slots in use (its own tasks, or the simulator processes found
when more), MemAvailable, the pending tasks in the order they will start,
the running and paused tasks, and the most recently ended ones. All
projects of the host's root are listed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := loadHerdEnv(cmd)
		if err != nil {
			return err
		}

		bs, err := queueBackends(env, args)
		if err != nil {
			return err
		}

		ctx, cancel := commandContext(cmd)
		defer cancel()

		for _, b := range bs {
			env.printf("===== %s =====\n", b.Host().Name)

			s, err := remoteQueue(ctx, b)
			if err != nil {
				env.printf("(%v)\n", err)
				continue
			}

			printQueue(env, s)
		}

		return nil
	},
}

func init() {
	herdCmd.AddCommand(herdQueueCmd)
}

// queueBackends are the named hosts, or all hosts that can have a queue.
func queueBackends(env *herdEnv, names []string) ([]backend.Backend, error) {
	bs, err := env.backends(names)
	if err != nil {
		return nil, err
	}

	out := bs[:0]

	for _, b := range bs {
		switch {
		case b.Host().Type != herd.HostSlurm:
			out = append(out, b)
		case len(names) > 0:
			return nil, fmt.Errorf("%s is a SLURM host; it has no queue", b.Host().Name)
		}
	}

	return out, nil
}

func remoteQueue(ctx context.Context, b backend.Backend) (*herd.QueueStatus, error) {
	data, err := b.Akita(ctx, "herd", "queue", "list", "--root", backend.Root(b))
	if err != nil {
		return nil, err
	}

	s := &herd.QueueStatus{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("%s: bad queue: %w", b.Host().Name, err)
	}

	return s, nil
}

func printQueue(env *herdEnv, s *herd.QueueStatus) {
	if s.Agent == nil {
		env.printf("agent: never ran\n")
	} else {
		a := s.Agent

		state := "running"
		if !s.Alive {
			state = "NOT RUNNING"
		}

		env.printf("agent: %s (pid %d), heartbeat %s ago; slots %d/%d (own %d, simulators %d); "+
			"MemAvailable %.1f GB\n", state, a.PID, ago(s.Now, a.Heartbeat), max(a.Own, a.External), a.Slots,
			a.Own, a.External, a.MemAvailGB)
	}

	tw := tabwriter.NewWriter(env.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATE\tPROJECT\tNAME\tPRIO\tMEM\tSINCE\tINFO")

	for _, t := range s.Pending {
		fmt.Fprintf(tw, "pending\t%s\t%s\t%d\t%s\t%s\t%s\n", t.Project, t.Name, t.Priority, memText(t.MemGB),
			ago(s.Now, t.Submitted), avoidText(t))
	}

	for _, t := range s.Running {
		state := "running"
		if t.Paused {
			state = "paused"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\tpid %d\n", state, t.Project, t.Name, t.Priority,
			memText(t.MemGB), ago(s.Now, t.Started), t.PGID)
	}

	for i, t := range s.Done {
		if i >= queueDoneShown {
			fmt.Fprintf(tw, "...\t%d more ended tasks\t\t\t\t\t\n", len(s.Done)-i)
			break
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", doneState(t), t.Project, t.Name, t.Priority,
			memText(t.MemGB), ago(s.Now, t.Ended), doneInfo(t))
	}

	_ = tw.Flush()
}

func ago(now, t float64) string {
	if t == 0 {
		return "-"
	}

	return (time.Duration(now-t) * time.Second).String()
}

func memText(gb float64) string {
	if gb == 0 {
		return "-"
	}

	return fmt.Sprintf("%gG", gb)
}

func avoidText(t herd.Task) string {
	if len(t.AvoidHosts) == 0 {
		return ""
	}

	return fmt.Sprintf("avoid %v", t.AvoidHosts)
}

func doneState(t herd.Task) string {
	if t.State == "failed" && t.Exit != nil {
		return fmt.Sprintf("failed(%d)", *t.Exit)
	}

	return t.State
}

func doneInfo(t herd.Task) string {
	info := t.Signal

	switch {
	case t.Error != "":
		info += " " + t.Error
	case t.Retried != "":
		info += " retried on " + t.Retried
	case t.Retryable:
		info += " retryable"
	}

	if t.Collected != nil {
		info += " collected"
	}

	return info
}
