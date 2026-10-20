package cmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/backend"
	"github.com/spf13/cobra"
)

// The states wait reports that are not an end: a task waiting in a
// queue, a SLURM job waiting, and a session or task that runs.
const (
	statePending = "pending"
	stateQueued  = "queued"
	stateRunning = "running"
	statePaused  = "paused"
)

var herdWaitCmd = &cobra.Command{
	Use:   "wait [--until ended|running|paused] [--timeout D] [host [name...]]",
	Short: "Wait until sessions and queued tasks reach a state (default: ended).",
	Long: `Wait until the named sessions and queued tasks of the project on a host
reach a state, printing each change. Without names, wait until nothing of
the project runs or waits on the host (on every host without a host): no
running or queued session, and no pending or running task.

--until running returns once each named one runs (not pending, queued,
or paused) or has ended, and --until paused once each is paused or has
ended. The exit status is 0 once
the state is reached, also when a task failed (status and queue show how
they ended); it is 1 on a timeout or when a name is unknown.`,
	Example: `  akita herd wait                         # until the project is idle on every host
  akita herd wait server1 1019_fir_base    # until it ends
  akita herd wait --until running server2 1019_mm_slow`,
	RunE: runWait,
}

func init() {
	f := herdWaitCmd.Flags()
	f.String("until", "ended", "ended, running, or paused")
	f.Duration("timeout", 0, "give up after this long (0: never)")
	f.Duration("interval", 2*time.Second, "how often to look")
	herdCmd.AddCommand(herdWaitCmd)
}

func runWait(cmd *cobra.Command, args []string) error {
	env, err := loadHerdEnv(cmd)
	if err != nil {
		return err
	}

	until, _ := cmd.Flags().GetString("until")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	interval, _ := cmd.Flags().GetDuration("interval")

	if !slices.Contains([]string{"ended", stateRunning, statePaused}, until) {
		return fmt.Errorf("--until %s: want ended, running, or paused", until)
	}

	var hosts, names []string
	if len(args) > 0 {
		hosts, names = args[:1], args[1:]
	}

	if len(names) == 0 && until != "ended" {
		return fmt.Errorf("--until %s needs a host and names", until)
	}

	bs, err := env.backends(hosts)
	if err != nil {
		return err
	}

	ctx, cancel := commandContext(cmd)
	defer cancel()

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	w := &waiter{env: env, until: until, last: map[string]string{}}

	for {
		done, err := w.poll(ctx, bs, names)
		if err != nil {
			return err
		}

		if done {
			return nil
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("timed out after %v", timeout)
			}

			return nil
		case <-time.After(interval):
		}
	}
}

// waiter remembers the last state of each session and task it reported.
type waiter struct {
	env   *herdEnv
	until string
	polls int
	last  map[string]string // host:name -> state
}

// poll looks at the hosts once, reports changes, and tells whether the
// wait is over.
func (w *waiter) poll(ctx context.Context, bs []backend.Backend, names []string) (bool, error) {
	w.polls++
	done := true

	for _, b := range bs {
		states, err := projectStates(ctx, w.env, b)
		if err != nil {
			return false, err
		}

		if len(names) == 0 {
			for name, st := range states {
				w.report(b, name, st, active(st))

				if active(st) {
					done = false
				}
			}

			continue
		}

		for _, name := range names {
			st, ok := states[name]
			if !ok {
				return false, fmt.Errorf("%s has no session or task named %s", b.Host().Name, name)
			}

			w.report(b, name, st, true)

			if !reached(st, w.until) {
				done = false
			}
		}
	}

	return done, nil
}

// report prints a state when it is new or changed. On the first look,
// only the states worth printing (named, or active) are printed, not the
// project's past sessions.
func (w *waiter) report(b backend.Backend, name, st string, worth bool) {
	key := b.Host().Name + ":" + name
	prev := w.last[key]
	w.last[key] = st

	if prev == st || (w.polls == 1 && !worth) {
		return
	}

	w.env.printf("%s %s %s\n", time.Now().Format("15:04:05"), key, st)
}

// projectStates are the states of the project's sessions and queued
// tasks on a host, by name: pending, queued, running, paused, or how it
// ended (ok, failed(N), stopped, died).
func projectStates(ctx context.Context, env *herdEnv, b backend.Backend) (map[string]string, error) {
	sessions, err := remoteSessions(ctx, b, env.project, false, 0)
	if err != nil {
		return nil, err
	}

	states := map[string]string{}

	for _, s := range sessions {
		if s.State == "stalled" {
			states[s.Name] = stateRunning
		} else {
			states[s.Name] = stateLabel(s)
		}
	}

	if b.Host().Type == herd.HostSlurm {
		return states, nil
	}

	q, err := remoteQueue(ctx, b)
	if err != nil {
		return nil, err
	}

	for _, t := range q.Done {
		if _, ok := states[t.Name]; !ok && t.Project == env.project {
			states[t.Name] = doneState(t)
		}
	}

	for _, t := range q.Running {
		if t.Project == env.project {
			states[t.Name] = stateRunning
			if t.Paused {
				states[t.Name] = statePaused
			}
		}
	}

	for _, t := range q.Pending {
		if t.Project == env.project {
			states[t.Name] = statePending
		}
	}

	return states, nil
}

// active tells whether a state is not an end.
func active(st string) bool {
	return st == statePending || st == stateQueued || st == stateRunning || st == statePaused
}

// reached tells whether a state satisfies --until; an end satisfies all.
func reached(st, until string) bool {
	switch {
	case !active(st):
		return true
	case until == stateRunning:
		return st == stateRunning
	case until == statePaused:
		return st == statePaused
	default:
		return false
	}
}
