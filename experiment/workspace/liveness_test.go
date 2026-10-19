package workspace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeLive is a scripted LiveSystem.
type fakeLive struct {
	alive map[int]bool
	slurm map[string]string
	final map[string]string
	calls int
}

func (f *fakeLive) PIDAlive(pid int) bool { return f.alive[pid] }

func (f *fakeLive) SlurmJob(_ context.Context, id string) (state, final string) {
	f.calls++
	return f.slurm[id], f.final[id]
}

func TestLivenessRules(t *testing.T) {
	ctx := context.Background()
	sys := &fakeLive{
		alive: map[int]bool{4242: true},
		slurm: map[string]string{"77": "running"},
		final: map[string]string{"78": "TIMEOUT"},
	}
	pidJob := &Job{ID: "4242", Kind: JobPID}

	cases := []struct {
		name string
		in   LiveInput
		want Live
	}{
		{"tmux", LiveInput{Tmux: true, TmuxPID: 5}, Live{State: "running", PID: 5}},
		{"finished markers win over tmux", LiveInput{Finished: true, Tmux: true, TmuxPID: 5}, Live{}},
		{"finished markers win over a live pid", LiveInput{Finished: true, Job: pidJob}, Live{}},
		{"shadowed archived log", LiveInput{Shadowed: true, Tmux: true, Job: pidJob}, Live{}},
		{"live pid", LiveInput{Job: pidJob}, Live{State: "running", PID: 4242}},
		{"dead pid", LiveInput{Job: &Job{ID: "1", Kind: JobPID}}, Live{}},
		{"slurm running", LiveInput{Job: &Job{ID: "77", Kind: JobSlurm}}, Live{State: "running"}},
		{"slurm final", LiveInput{Job: &Job{ID: "78", Kind: JobSlurm}}, Live{Final: "TIMEOUT"}},
		{"no job", LiveInput{}, Live{}},
	}

	for _, c := range cases {
		sys.calls = 0
		assert.Equal(t, c.want, Liveness(ctx, sys, c.in), c.name)
		assert.LessOrEqual(t, sys.calls, 1, "%s: squeue asked more than once", c.name)
	}
}
