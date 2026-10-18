package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/stretchr/testify/require"
)

// cmdTracer is a Tracer that counts the command-issue milestones recorded on
// sub-transaction tasks. It only observes — it must not change results.
type cmdTracer struct {
	tasks.NopObserver
	count    int
	byKind   map[string]int
	subTasks int
}

func newCmdTracer() *cmdTracer {
	return &cmdTracer{byKind: map[string]int{}}
}

func (subject *cmdTracer) TaskStarted(ts tasks.Task) {
	if ts.Kind == "sub-trans" {
		subject.subTasks++
	}
}

func (subject *cmdTracer) MilestoneAdded(m tasks.Milestone) {
	subject.count++
	subject.byKind[m.What]++
}

func TestP1StrategySelection(t *testing.T) {
	t.Run("selects the default strategies from a default spec", func(t *testing.T) {
		spec := DefaultSpec()
		ctrl := newController(&spec)

		require.Equal(t, "FRFCFS", ctrl.scheduler.Name())
		require.Equal(t, "default", ctrl.addrMapper.Name())
	})

	t.Run("derives the row policy from PagePolicy", func(t *testing.T) {
		open := DefaultSpec()
		open.PagePolicy = PagePolicyOpen
		require.Equal(t, "open", newController(&open).rowPolicy.Name())

		closed := DefaultSpec()
		closed.PagePolicy = PagePolicyClose
		require.Equal(t, "close", newController(&closed).rowPolicy.Name())
	})

	t.Run("selects a scheduler by its Spec registry key", func(t *testing.T) {
		spec := DefaultSpec()
		spec.Scheduler = "FRFCFS"
		require.Equal(t, "FRFCFS", newController(&spec).scheduler.Name())
	})

	t.Run("panics on an unknown registry key", func(t *testing.T) {
		spec := DefaultSpec()
		spec.Scheduler = "does-not-exist"
		require.Panics(t, func() { newController(&spec) })
	})
}

func TestP1CommandTracing(t *testing.T) {
	t.Run("records command milestones without changing results", func(t *testing.T) {
		spec := DefaultSpec()
		tracer := newCmdTracer()

		// Same workload, with and without the tracer.
		traced := newP0Harness(t, spec, tracer)
		traced.src.Send(traced.write(0x40, []byte{1, 2, 3, 4}))
		require.NoError(t, traced.engine.Run())
		traced.src.Send(traced.read(0x40))
		require.NoError(t, traced.engine.Run())
		tracedReads, tracedWrites := traced.collect()

		plain := newP0Harness(t, spec)
		plain.src.Send(plain.write(0x40, []byte{1, 2, 3, 4}))
		require.NoError(t, plain.engine.Run())
		plain.src.Send(plain.read(0x40))
		require.NoError(t, plain.engine.Run())
		plainReads, plainWrites := plain.collect()

		// The tracer saw sub-transaction tasks and command milestones (at least
		// an ACT plus column commands) on them.
		require.Greater(t, tracer.subTasks, 0)
		require.Greater(t, tracer.count, 0)
		require.Greater(t, tracer.byKind["ACT"], 0)

		// Results are identical to the untraced run.
		require.Len(t, tracedWrites, len(plainWrites))
		require.Len(t, tracedReads, len(plainReads))
		require.Len(t, tracedReads, 1)
		require.Equal(t, []byte{1, 2, 3, 4}, tracedReads[0].Payload.(memprotocol.DataReadyRsp).Data[:4])
	})
}
