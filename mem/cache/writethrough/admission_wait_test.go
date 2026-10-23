package writethrough

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type admissionClock struct {
	timing.Engine
	now timing.VTimeInPicoSec
}

func (c *admissionClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

type admissionConnection struct{ hooking.HookableBase }

func (*admissionConnection) Name() string                   { return "Link" }
func (*admissionConnection) NotifySend(_ messaging.Port)    {}
func (*admissionConnection) NotifyAvailable(messaging.Port) {}
func (c *admissionConnection) BindPort(p messaging.Port)    { p.BindConnection(c) }

type admissionRecorder struct {
	tasks.NopObserver
	starts     []tasks.Task
	milestones []tasks.Milestone
}

func (r *admissionRecorder) TaskStarted(task tasks.Task)      { r.starts = append(r.starts, task) }
func (r *admissionRecorder) MilestoneAdded(m tasks.Milestone) { r.milestones = append(r.milestones, m) }
func (r *admissionRecorder) firstStart(kind string) (tasks.Task, bool) {
	for _, task := range r.starts {
		if task.Kind == kind {
			return task, true
		}
	}
	return tasks.Task{}, false
}
func (r *admissionRecorder) milestonesOn(id uint64) []tasks.Milestone {
	var out []tasks.Milestone
	for _, m := range r.milestones {
		if m.TaskID == id {
			out = append(out, m)
		}
	}
	return out
}

func setupAdmissionWait(t *testing.T) (*admissionClock, *Comp, *admissionRecorder, uint64) {
	t.Helper()
	clock := &admissionClock{Engine: timing.NewSerialEngine()}
	sim := testsim.New(clock, true)
	spec := DefaultSpec()
	spec.Bottom.Destinations = []messaging.RemotePort{"Memory.Top"}
	spec.MaxInFlightRequests, spec.NumReqPerCycle = 1, 1

	spec.DirectoryWidth = 1
	spec.BankWidth = 1

	c := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).
		WithResources(Resources{Storage: mem.NewStorage(spec.ByteSize())}).Build("Cache")
	for _, name := range []string{"Top", "Bottom", "Control"} {
		p := twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 2, OutgoingCapacity: 2}).
			Build()
		c.BindPort(name, p)
		(&admissionConnection{}).BindPort(p)
	}
	require.NoError(t, sim.Initialize())
	rec := &admissionRecorder{}
	sim.TaskTracker().Subscribe(rec, c.Ports.Top)
	req := messaging.Msg{ID: c.NewID(), Src: "Agent", Dst: c.Ports.Top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4}}
	c.Ports.Top.Deliver(req)
	bufferTask, ok := rec.firstStart(tasks.KindIncomingBuffer)
	require.True(t, ok)
	c.State.DirBuf.Push(0)
	c.State.Transactions = []transactionState{{}}
	return clock, c, rec, bufferTask.ID
}

func TestAdmissionWaitTransitionsAcrossCheckpoint(t *testing.T) {
	clock, c, rec, bufferID := setupAdmissionWait(t)

	tick := c.Middlewares.Pipeline.intakeStage.Tick
	clock.now = 10
	require.False(t, tick())
	require.Empty(t, rec.milestonesOn(bufferID))
	c.State.Transactions = nil
	clock.now = 20
	require.False(t, tick())
	clock.now = 25
	require.False(t, tick())
	require.Len(t, rec.milestonesOn(bufferID), 1)
	var saved bytes.Buffer
	require.NoError(t, c.SaveCheckpoint(&saved))
	c.State.WaitingForDir = false
	c.State.AdmissionMsgID = 0
	require.NoError(t, c.LoadCheckpoint(&saved))
	clock.now = 30
	require.False(t, tick())
	clock.now = 35
	require.False(t, tick())
	c.State.DirBuf.Pop()
	clock.now = 40
	require.True(t, tick())
	require.Len(t, c.State.Transactions, 1)
	require.Zero(t, c.Ports.Top.IncomingBuf().Size())
	ms := rec.milestonesOn(bufferID)
	require.Len(t, ms, 2)
	for i, want := range []struct {
		what string
		time timing.VTimeInPicoSec
	}{
		{"Cache.trans", 20}, {"Cache.dir_buf", 40},
	} {
		require.Equal(t, tasks.MilestoneKindHardwareResource, ms[i].Kind)
		require.Equal(t, want.what, ms[i].What)
		require.Equal(t, want.time, ms[i].Time)
	}
	t.Log("Cache preserved transaction/directory wait transitions at 20/40 across checkpoint; " +
		"no polling duplicates or early admission")
}
