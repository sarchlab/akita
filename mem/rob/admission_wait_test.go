package rob

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
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

func setupAdmissionWait(t *testing.T) (*admissionClock, *Comp, *milestoneRecorder, uint64) {
	t.Helper()
	clock := &admissionClock{Engine: timing.NewSerialEngine()}
	sim := testsim.New(clock, true)
	spec := DefaultSpec()
	spec.BufferSize = 1
	spec.Bottom = "Remote"
	c := modeling.MakeBuilder[*Comp](spec).WithSimulation(sim).Build("Rob")
	for _, name := range []string{"Top", "Bottom", "Control"} {
		p := twowaybuffered.NewPort(2, 1)
		c.BindPort(name, p)
		(&noopConn{}).BindPort(p)
	}
	require.NoError(t, sim.Initialize())
	rec := &milestoneRecorder{}
	sim.TaskTracker().Subscribe(rec, c.Ports.Top)
	req := messaging.Msg{ID: c.NewID(), Src: "Agent", Dst: c.Ports.Top.AsRemote(),
		Payload: memprotocol.ReadReq{Address: 0, AccessByteSize: 4}}
	c.Ports.Top.Deliver(req)
	bufferID := rec.taskID(tasks.KindIncomingBuffer)
	c.Ports.Bottom.Send(messaging.Msg{ID: c.NewID(), Src: c.Ports.Bottom.AsRemote(), Dst: "Remote"})
	c.State.Transactions = []transactionState{{}}
	return clock, c, rec, bufferID
}

func TestAdmissionWaitTransitionsAcrossCheckpoint(t *testing.T) {
	clock, c, rec, bufferID := setupAdmissionWait(t)
	clock.now = 10
	require.False(t, c.Middlewares.Pipeline.topDown())
	require.Empty(t, rec.milestonesOn(bufferID))
	c.State.Transactions = nil
	clock.now = 20
	require.False(t, c.Middlewares.Pipeline.topDown())
	clock.now = 25
	require.False(t, c.Middlewares.Pipeline.topDown())
	require.Len(t, rec.milestonesOn(bufferID), 1, "polling must not repeat the first wait phase")
	var saved bytes.Buffer
	require.NoError(t, c.SaveCheckpoint(&saved))
	c.State.WaitingForBottom = false
	c.State.AdmissionMsgID = 0
	require.NoError(t, c.LoadCheckpoint(&saved))
	clock.now = 30
	require.False(t, c.Middlewares.Pipeline.topDown())
	clock.now = 35
	require.False(t, c.Middlewares.Pipeline.topDown())
	c.Ports.Bottom.RetrieveOutgoing()
	clock.now = 40
	require.True(t, c.Middlewares.Pipeline.topDown())
	require.Len(t, c.State.Transactions, 1)
	require.Zero(t, c.Ports.Top.NumIncoming())
	ms := rec.milestonesOn(bufferID)
	require.Len(t, ms, 2)
	for i, want := range []struct {
		kind tasks.MilestoneKind
		what string
		time timing.VTimeInPicoSec
	}{
		{tasks.MilestoneKindHardwareResource, "Rob.buffer", 20},
		{tasks.MilestoneKindNetworkBusy, "Rob.Bottom", 40},
	} {
		require.Equal(t, want.kind, ms[i].Kind)
		require.Equal(t, want.what, ms[i].What)
		require.Equal(t, want.time, ms[i].Time)
	}
	t.Log("ROB preserved slot/network wait transitions at 20/40 across checkpoint; " +
		"no polling duplicates or early admission")
}
