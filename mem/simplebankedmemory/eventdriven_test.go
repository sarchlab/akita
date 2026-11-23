package simplebankedmemory

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/internal/tracingtest"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// driverEvent runs a test action at a chosen time on the real engine.
type driverEvent struct {
	timing.EventBase
	action func()
}

type driverHandler struct{}

func (driverHandler) Handle(e timing.Event) { e.(driverEvent).action() }

// wakeRig runs the memory on the serial engine and records the cycle of every
// tick it receives and every response it sends.
type wakeRig struct {
	sim     *akitasim.Simulation
	engine  *timing.SerialEngine
	storage *mem.Storage
	comp    *Comp
	top     messaging.Port
	ctrl    messaging.Port
	ticks   []uint64
	sends   []uint64
	rspTo   []uint64

	// maxResident is the most requests any bank held after an event.
	maxResident int
}

func (r *wakeRig) Func(ctx hooking.HookCtx) {
	switch ctx.Pos {
	case timing.HookPosBeforeEvent:
		if e, ok := ctx.Item.(ticking.Event); ok && e.HandlerID() == r.comp.Name() {
			r.ticks = append(r.ticks, r.comp.Spec.Freq.Cycle(e.Time()))
		}
	case timing.HookPosAfterEvent:
		for i := range r.comp.State.Banks {
			r.maxResident = max(r.maxResident, r.comp.State.Banks[i].Pipeline.Size())
		}
	case messaging.HookPosPortMsgSend:
		r.sends = append(r.sends, r.comp.Spec.Freq.Cycle(r.engine.CurrentTime()))
		r.rspTo = append(r.rspTo, ctx.Item.(messaging.Msg).RspTo)
	}
}

func newWakeRig(t *testing.T, spec Spec, outgoing int) *wakeRig {
	t.Helper()

	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	storage := mem.NewStorage(1 * mem.MB)
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{Storage: storage}).
		Build("Mem")
	comp.BindPort("Top", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 32, OutgoingCapacity: outgoing}).
		Build())
	comp.BindPort("Control", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
		Build())
	for _, p := range []messaging.Port{comp.Ports.Top, comp.Ports.Control} {
		(&noopConn{}).BindPort(p)
	}
	sim.Engine().RegisterHandler("Driver", driverHandler{})
	require.NoError(t, sim.Initialize())

	r := &wakeRig{
		sim: sim, engine: sim.Engine().(*timing.SerialEngine), storage: storage,
		comp: comp, top: comp.Ports.Top, ctrl: comp.Ports.Control,
	}
	r.engine.AcceptHook(r)
	r.top.AcceptHook(r)

	return r
}

func (r *wakeRig) at(cycle uint64, action func()) {
	r.engine.Schedule(driverEvent{
		EventBase: timing.MakeEventBase(r.sim.NewID(), cycleEdge(r.comp.Spec, cycle), "Driver"),
		action:    action,
	})
}

func (r *wakeRig) read(index int) messaging.Msg {
	return makeReadReq(r.sim, messaging.RemotePort("Agent"), r.top.AsRemote(), index)
}

func (r *wakeRig) deliverReads(cycle uint64, indices ...int) {
	r.at(cycle, func() {
		for _, index := range indices {
			r.top.Deliver(r.read(index))
		}
	})
}

func (r *wakeRig) control(cycle uint64, cmd memcontrolprotocol.Command) {
	r.at(cycle, func() {
		r.ctrl.Deliver(messaging.Msg{
			Payload: memcontrolprotocol.Req{Command: cmd},
			ID:      r.sim.NewID(),
			Src:     messaging.RemotePort("Ctrl"),
			Dst:     r.ctrl.AsRemote(),
		})
	})
}

func (r *wakeRig) quiescent() bool {
	for i := range r.comp.State.Banks {
		if !bankIsQuiescent(&r.comp.State.Banks[i]) {
			return false
		}
	}

	return true
}

func wakeSpec(latency int) Spec {
	spec := DefaultSpec()
	spec.StageLatency = latency

	return spec
}

// TestSparseBankSkipsIntermediateEdges is the measurable effect: one request
// through a 100000-cycle bank costs four ticks rather than one per cycle.
func TestSparseBankSkipsIntermediateEdges(t *testing.T) {
	const latency = 100_000
	r := newWakeRig(t, wakeSpec(latency), 4)
	r.deliverReads(5, 0)

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{latency + 6}, r.sends)
	require.Equal(t, []uint64{6, 7, latency + 6, latency + 7}, r.ticks)
	t.Logf("%d ticks for a request spanning %d cycles", len(r.ticks), latency+1)
}

func TestWakeUsesExactEdgesAtFractionalPeriod(t *testing.T) {
	spec := wakeSpec(1234)
	spec.Freq = 2100 * timing.MHz
	r := newWakeRig(t, spec, 4)
	r.deliverReads(5, 0)
	var sentAt timing.VTimeInPicoSec
	r.at(1240, func() { sentAt = r.engine.CurrentTime() })

	require.NoError(t, r.engine.Run())

	require.Equal(t, timing.VTimeInPicoSec(476), spec.Freq.Period())
	require.Equal(t, timing.VTimeInPicoSec(1240*476), sentAt)
	require.Equal(t, []uint64{1240}, r.sends)
	require.Equal(t, []uint64{6, 7, 1240, 1241}, r.ticks)
}

func TestIdleMemoryNeverTicks(t *testing.T) {
	r := newWakeRig(t, DefaultSpec(), 4)

	require.NoError(t, r.engine.Run())

	require.Empty(t, r.ticks)
}

func TestEarlierArrivalWakesBeforePendingCompletion(t *testing.T) {
	r := newWakeRig(t, wakeSpec(1000), 4)
	r.deliverReads(5, 0)
	r.deliverReads(100, 1)

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{1006, 1101}, r.sends)
	require.Equal(t, []uint64{6, 7, 101, 102, 1006, 1007, 1101, 1102}, r.ticks)
}

func TestQueuedInputEntersAsBankFrees(t *testing.T) {
	r := newWakeRig(t, wakeSpec(2), 8)
	// All map to bank 0, whose single lane holds two requests.
	r.deliverReads(5, 0, 4, 8, 12, 16)

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{8, 9, 10, 11, 12}, r.sends)
	require.True(t, r.quiescent())
}

// mi300xShapeSpec is one lane of five one-cycle stages.
func mi300xShapeSpec() Spec {
	spec := DefaultSpec()
	spec.NumBanks = 1
	spec.BankPipelineWidth = 1
	spec.BankPipelineDepth = 5
	spec.StageLatency = 1
	spec.PostPipelineBufSize = 128

	return spec
}

func TestFullBlockedLaneHoldsExactlyItsStages(t *testing.T) {
	r := newWakeRig(t, mi300xShapeSpec(), 1)
	r.deliverReads(5, 0, 1, 2, 3, 4, 5, 6, 7)

	// The first response fills the port at 11. Five more requests then fill
	// the five stages, and only the head has committed.
	r.at(50, func() {
		stages := r.comp.State.Banks[0].Pipeline.Stages()
		require.Len(t, stages, 5)
		for i, s := range stages {
			require.Equal(t, 4-i, s.Stage)
			require.Equal(t, i == 0, s.Item.Committed)
		}
		require.False(t, r.comp.State.Banks[0].Pipeline.CanAccept())
		require.Equal(t, 2, r.top.IncomingBuf().Size())
		require.Equal(t, []uint64{6, 7, 8, 9, 10, 11, 12}, r.ticks, "a blocked lane must sleep")
	})
	for release := uint64(50); release <= 110; release += 10 {
		r.at(release, func() { r.top.RetrieveOutgoing() })
	}

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{11, 51, 61, 71, 81, 91, 101, 111}, r.sends)
	require.Equal(t, 5, r.maxResident)
	require.True(t, r.quiescent())
}

func TestUncontendedLaneLatencyAndThroughput(t *testing.T) {
	r := newWakeRig(t, mi300xShapeSpec(), 32)
	indices := make([]int, 20)
	for i := range indices {
		indices[i] = i
	}
	r.deliverReads(5, indices...)

	require.NoError(t, r.engine.Run())

	want := make([]uint64, 20)
	for i := range want {
		want[i] = uint64(11 + i) // accepted at 6+i, five cycles of latency
	}
	require.Equal(t, want, r.sends)
	require.Equal(t, 5, r.maxResident)
}

func TestLoadRejectsLegacyPushPipelineCheckpoint(t *testing.T) {
	r := newWakeRig(t, DefaultSpec(), 4)
	legacy := `{"state":{"banks":[{` +
		`"pipeline":{"width":1,"num_stages":10,"stages":[{"lane":0,"stage":3,"item":{"is_read":true}}]},` +
		`"post_pipeline_buf":{"cap":1,"elements":[{"is_read":true}]}}]},"pending_event_guard":{}}`

	err := r.comp.LoadCheckpoint(strings.NewReader(legacy))

	require.ErrorContains(t, err, "push Pipeline snapshot")
	require.Len(t, r.comp.State.Banks, 4, "a rejected checkpoint must leave State unchanged")
}

func TestBanksRespondInBankOrderWithinACycle(t *testing.T) {
	r := newWakeRig(t, wakeSpec(5), 8)
	var ids []uint64
	r.at(5, func() {
		for _, index := range []int{3, 2, 1, 0} {
			msg := r.read(index)
			ids = append([]uint64{msg.ID}, ids...)
			r.top.Deliver(msg)
		}
	})

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{11, 11, 11, 11}, r.sends)
	require.Equal(t, ids, r.rspTo)
}

func TestBlockedResponseKeepsCommittedStateAndSleeps(t *testing.T) {
	for name, isWrite := range map[string]bool{"read": false, "write": true} {
		t.Run(name, func(t *testing.T) {
			spec := wakeSpec(4)
			spec.NumBanks = 2
			r := newWakeRig(t, spec, 1)

			const blockedAddr = uint64(64)
			r.storage.Write(blockedAddr, []byte{1, 2, 3, 4})
			r.at(5, func() {
				r.top.Deliver(r.read(0))
				if isWrite {
					r.top.Deliver(messaging.Msg{
						Payload: memprotocol.WriteReq{Address: blockedAddr, Data: []byte{9, 9, 9, 9}},
						ID:      r.sim.NewID(), Src: "Agent", Dst: r.top.AsRemote(),
					})
					return
				}
				r.top.Deliver(messaging.Msg{
					Payload: memprotocol.ReadReq{Address: blockedAddr, AccessByteSize: 4},
					ID:      r.sim.NewID(), Src: "Agent", Dst: r.top.AsRemote(),
				})
			})

			// The commit happened at cycle 10. A later store must not be
			// observed by the read or overwritten by a repeated write.
			r.at(50, func() {
				stages := r.comp.State.Banks[1].Pipeline.Stages()
				require.Len(t, stages, 1)
				require.True(t, stages[0].Item.Committed)
				r.storage.Write(blockedAddr, []byte{7, 7, 7, 7})
			})
			r.at(100, func() { r.top.RetrieveOutgoing() })

			require.NoError(t, r.engine.Run())

			require.Equal(t, []uint64{10, 101}, r.sends)
			require.Equal(t, []uint64{6, 7, 10, 11, 101, 102}, r.ticks)
			require.Equal(t, []byte{7, 7, 7, 7}, r.storage.Read(blockedAddr, 4))

			rsp, ok := r.top.RetrieveOutgoing()
			require.True(t, ok)
			if !isWrite {
				require.Equal(t, []byte{1, 2, 3, 4}, rsp.Payload.(memprotocol.DataReadyRsp).Data)
			}
		})
	}
}

// newWideBlockedRig builds one bank of two 4-stage lanes whose Top output is
// already full, with distinct data at the first three request addresses.
func newWideBlockedRig(t *testing.T) *wakeRig {
	t.Helper()

	spec := wakeSpec(4)
	spec.NumBanks = 1
	spec.BankPipelineWidth = 2
	r := newWakeRig(t, spec, 1)
	for i, value := range []byte{0x11, 0x22, 0x33} {
		r.storage.Write(uint64(i*readSize), bytes.Repeat([]byte{value}, readSize))
	}
	r.top.Send(messaging.Msg{
		Payload: memprotocol.WriteDoneRsp{},
		ID:      r.sim.NewID(), Src: r.top.AsRemote(), Dst: "Agent",
	})
	r.sends, r.rspTo = nil, nil

	return r
}

// deliverStaggered makes lane heads ready at different cycles: request 0 enters
// lane 0 at 6, then request 1 follows it and request 2 enters lane 1 at 7. The
// heads are ready at 10 and 11.
func (r *wakeRig) deliverStaggered() []uint64 {
	first, second, third := r.read(0), r.read(1), r.read(2)
	r.at(5, func() { r.top.Deliver(first) })
	r.at(7, func() {
		r.top.Deliver(second)
		r.top.Deliver(third)
	})

	return []uint64{first.ID, second.ID, third.ID}
}

func committedFlags(b *bankState) []bool {
	stages := b.Pipeline.Stages()
	flags := make([]bool, len(stages))
	for i, s := range stages {
		flags[i] = s.Item.Committed
	}

	return flags
}

func (r *wakeRig) overwriteRequestData(value byte) {
	r.storage.Write(0, bytes.Repeat([]byte{value}, 3*readSize))
}

func requireReadData(t *testing.T, msg messaging.Msg, ok bool, value byte) {
	t.Helper()

	require.True(t, ok)
	require.Equal(t, bytes.Repeat([]byte{value}, readSize), msg.Payload.(memprotocol.DataReadyRsp).Data)
}

func TestWideBankCommitsSimultaneousHeadsWhileBlocked(t *testing.T) {
	r := newWideBlockedRig(t)
	first, second := r.read(0), r.read(1)
	r.at(5, func() {
		r.top.Deliver(first)
		r.top.Deliver(second)
	})
	r.at(50, func() {
		require.Equal(t, []bool{true, true}, committedFlags(&r.comp.State.Banks[0]))
		r.overwriteRequestData(0xEE)
	})
	r.at(100, func() { r.top.RetrieveOutgoing() }) // the prefilled message
	r.at(110, func() {
		msg, ok := r.top.RetrieveOutgoing()
		requireReadData(t, msg, ok, 0x11)
	})

	require.NoError(t, r.engine.Run())

	msg, ok := r.top.RetrieveOutgoing()
	requireReadData(t, msg, ok, 0x22)
	require.Equal(t, []uint64{101, 111}, r.sends)
	require.Equal(t, []uint64{first.ID, second.ID}, r.rspTo)
	require.Equal(t, []uint64{6, 7, 10, 101, 102, 111, 112}, r.ticks)
}

func TestWideBankCommitsStaggeredHeadsWhileBlocked(t *testing.T) {
	r := newWideBlockedRig(t)
	ids := r.deliverStaggered()
	r.at(50, func() {
		// Both lane heads committed; the item behind the first head did not.
		require.Equal(t, []bool{true, false, true}, committedFlags(&r.comp.State.Banks[0]))
		r.overwriteRequestData(0xEE)
	})
	r.at(100, func() { r.top.RetrieveOutgoing() }) // the prefilled message
	r.at(110, func() {
		msg, ok := r.top.RetrieveOutgoing()
		requireReadData(t, msg, ok, 0x11)
	})
	r.at(120, func() {
		// Request 1 became its lane's head after the overwrite.
		msg, ok := r.top.RetrieveOutgoing()
		requireReadData(t, msg, ok, 0xEE)
	})

	require.NoError(t, r.engine.Run())

	msg, ok := r.top.RetrieveOutgoing()
	requireReadData(t, msg, ok, 0x33)
	require.Equal(t, []uint64{101, 111, 121}, r.sends)
	require.Equal(t, ids, r.rspTo)
	require.Equal(t, []uint64{6, 7, 8, 10, 11, 101, 102, 111, 112, 121, 122}, r.ticks)
}

func TestWideBankKeepsReadyWriteThroughReset(t *testing.T) {
	r := newWideBlockedRig(t)
	written := bytes.Repeat([]byte{0x77}, readSize)
	first, second := r.read(0), r.read(1)
	write := messaging.Msg{
		Payload: memprotocol.WriteReq{Address: 2 * readSize, Data: written},
		ID:      r.sim.NewID(), Src: "Agent", Dst: r.top.AsRemote(),
	}
	r.at(5, func() { r.top.Deliver(first) })
	r.at(7, func() {
		r.top.Deliver(second)
		r.top.Deliver(write)
	})
	r.control(20, memcontrolprotocol.CmdReset)

	require.NoError(t, r.engine.Run())

	require.Equal(t, written, r.storage.Read(2*readSize, readSize))
	require.Empty(t, r.sends)
	require.True(t, r.quiescent())
}

func TestCheckpointKeepsFutureLaneWakeWhileHeadIsBlocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	src := newWideBlockedRig(t)
	src.deliverStaggered()
	require.NoError(t, src.engine.RunUntil(cycleEdge(src.comp.Spec, 10)))
	require.Equal(t, []bool{true, false, false}, committedFlags(&src.comp.State.Banks[0]))
	require.NoError(t, src.sim.SaveCheckpoint(path))

	resumed := newWideBlockedRig(t)
	require.NoError(t, resumed.sim.LoadCheckpoint(path))
	require.NoError(t, resumed.engine.Run())

	require.Equal(t, []uint64{11}, resumed.ticks)
	require.Equal(t, []bool{true, false, true}, committedFlags(&resumed.comp.State.Banks[0]))
	require.Empty(t, resumed.sends)
}

func TestPauseFreezesPipelineTime(t *testing.T) {
	r := newWakeRig(t, wakeSpec(20), 4)
	r.deliverReads(5, 0)
	r.control(9, memcontrolprotocol.CmdPause)    // handled at tick 10
	r.control(109, memcontrolprotocol.CmdEnable) // handled at tick 110

	require.NoError(t, r.engine.Run())

	// Accepted at 6 and due at 26, shifted by the 100 paused cycles. The tick
	// requested for 26 before the Pause fires once and does nothing.
	require.Equal(t, []uint64{126}, r.sends)
	require.Equal(t, []uint64{6, 7, 10, 11, 26, 110, 111, 126, 127}, r.ticks)
}

func TestRepeatedPauseAndEnableDoNotShiftTiming(t *testing.T) {
	r := newWakeRig(t, wakeSpec(20), 4)
	r.deliverReads(5, 0)
	r.control(9, memcontrolprotocol.CmdPause)
	r.control(19, memcontrolprotocol.CmdPause)
	r.control(29, memcontrolprotocol.CmdEnable)
	r.control(39, memcontrolprotocol.CmdEnable)

	require.NoError(t, r.engine.Run())

	// Paused from tick 10 to tick 30; the second Enable changes nothing.
	require.Equal(t, []uint64{46}, r.sends)
}

func TestDrainFromPauseDoesNotCountPausedCycles(t *testing.T) {
	r := newWakeRig(t, wakeSpec(20), 4)
	r.deliverReads(5, 0)
	r.control(9, memcontrolprotocol.CmdPause)   // handled at tick 10
	r.control(109, memcontrolprotocol.CmdDrain) // handled at tick 110

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{126}, r.sends)
	require.Equal(t, controlStatePaused, r.comp.State.ControlState)
	require.True(t, r.quiescent())
}

func TestBlockedResponseWaitsThroughPause(t *testing.T) {
	spec := wakeSpec(4)
	spec.NumBanks = 2
	r := newWakeRig(t, spec, 1)
	r.deliverReads(5, 0, 1)
	r.control(20, memcontrolprotocol.CmdPause)
	r.at(50, func() { r.top.RetrieveOutgoing() }) // space frees while paused
	r.control(99, memcontrolprotocol.CmdEnable)

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{10, 100}, r.sends)
	require.True(t, r.quiescent())
}

func TestResetClearsBlockedResponseAndItsTasks(t *testing.T) {
	spec := wakeSpec(4)
	spec.NumBanks = 2
	r := newWakeRig(t, spec, 1)
	rec := &tracingtest.LeakRecorder{}
	r.comp.TaskTracker().Subscribe(rec, r.comp)
	r.deliverReads(5, 0, 1)
	r.control(20, memcontrolprotocol.CmdReset)
	r.at(50, func() { r.top.RetrieveOutgoing() })

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{10}, r.sends)
	require.True(t, r.quiescent())
	require.Empty(t, rec.OpenTasks(), rec.OpenSummary())
}

func TestDrainWaitsForBlockedResponse(t *testing.T) {
	spec := wakeSpec(4)
	spec.NumBanks = 2
	r := newWakeRig(t, spec, 1)
	r.deliverReads(5, 0, 1)
	r.control(20, memcontrolprotocol.CmdDrain)
	r.at(100, func() {
		_, acked := r.ctrl.PeekOutgoing()
		require.False(t, acked, "Drain must not ack while a response is blocked")
		require.Equal(t, controlStateDraining, r.comp.State.ControlState)
		r.top.RetrieveOutgoing()
	})

	require.NoError(t, r.engine.Run())

	require.Equal(t, []uint64{10, 101}, r.sends)
	require.Equal(t, controlStatePaused, r.comp.State.ControlState)
	require.True(t, r.quiescent())
	ack, ok := r.ctrl.RetrieveOutgoing()
	require.True(t, ok)
	require.Equal(t, memcontrolprotocol.CmdDrain, ack.Payload.(memcontrolprotocol.Rsp).Command)
}

func TestResetDropsInflightWorkAndItsPendingWake(t *testing.T) {
	r := newWakeRig(t, wakeSpec(1000), 4)
	r.deliverReads(5, 0)
	r.control(20, memcontrolprotocol.CmdReset)
	r.deliverReads(40, 1)

	require.NoError(t, r.engine.Run())

	// Only the request after the Reset responds; the wake left for the
	// discarded request at 1006 is a harmless no-op.
	require.Equal(t, []uint64{1041}, r.sends)
	require.True(t, r.quiescent())
	require.Contains(t, r.ticks, uint64(1006))
}

func TestZeroLatencySpecsRespondInOneCycle(t *testing.T) {
	cases := map[string]func(*Spec){
		"zero depth":              func(s *Spec) { s.BankPipelineDepth = 0 },
		"zero stage latency":      func(s *Spec) { s.BankPipelineDepth = 3; s.StageLatency = 0 },
		"zero depth and latency":  func(s *Spec) { s.BankPipelineDepth = 0; s.StageLatency = 0 },
		"zero depth, zero width":  func(s *Spec) { s.BankPipelineDepth = 0; s.BankPipelineWidth = 0 },
		"zero depth, zero buffer": func(s *Spec) { s.BankPipelineDepth = 0; s.PostPipelineBufSize = 0 },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			spec := DefaultSpec()
			modify(&spec)
			r := newWakeRig(t, spec, 4)
			r.deliverReads(5, 0)

			require.NoError(t, r.engine.Run())

			require.Equal(t, []uint64{7}, r.sends)
			require.True(t, r.quiescent())
		})
	}
}

func TestZeroWidthPipelineWaitsWithoutSpinning(t *testing.T) {
	spec := DefaultSpec()
	spec.BankPipelineWidth = 0
	r := newWakeRig(t, spec, 4)
	r.deliverReads(5, 0)

	require.NoError(t, r.engine.Run())

	require.Empty(t, r.sends)
	require.Equal(t, []uint64{6}, r.ticks)
}

func TestDeprecatedPostPipelineBufSizeIsIgnored(t *testing.T) {
	var want []uint64
	for _, size := range []int{1, 0, -5, 64} {
		spec := wakeSpec(3)
		spec.PostPipelineBufSize = size
		r := newWakeRig(t, spec, 2)
		r.deliverReads(5, 0, 4, 8, 12, 1, 5)
		r.at(40, func() { r.top.RetrieveOutgoing(); r.top.RetrieveOutgoing() })
		r.at(60, func() { r.top.RetrieveOutgoing(); r.top.RetrieveOutgoing() })

		require.NoError(t, r.engine.Run())

		if want == nil {
			want = r.sends
		}
		require.Len(t, r.sends, 6)
		require.Equal(t, want, r.sends, "PostPipelineBufSize=%d", size)
	}
}

func TestDeprecatedFieldKeepsJSONCompatibility(t *testing.T) {
	var spec Spec
	require.NoError(t, json.Unmarshal([]byte(`{"post_pipeline_buf_size":-3}`), &spec))
	require.Equal(t, -3, spec.PostPipelineBufSize)

	data, err := json.Marshal(DefaultSpec())
	require.NoError(t, err)
	require.Contains(t, string(data), `"post_pipeline_buf_size":1`)
}

func TestCheckpointResumesInFlightWork(t *testing.T) {
	setup := func(t *testing.T) *wakeRig {
		t.Helper()
		spec := wakeSpec(40)
		r := newWakeRig(t, spec, 8)
		r.deliverReads(5, 0, 1, 4)

		return r
	}

	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	ref := setup(t)
	require.NoError(t, ref.engine.RunUntil(cycleEdge(ref.comp.Spec, 20)))
	require.Empty(t, ref.sends)
	require.NoError(t, ref.sim.SaveCheckpoint(path))
	ref.ticks = nil
	require.NoError(t, ref.engine.Run())

	resumed := newWakeRig(t, wakeSpec(40), 8)
	require.NoError(t, resumed.sim.LoadCheckpoint(path))
	require.NoError(t, resumed.engine.Run())

	require.Equal(t, []uint64{46, 46, 47}, ref.sends[:3])
	require.Equal(t, ref.sends, resumed.sends)
	require.Equal(t, ref.ticks, resumed.ticks)
	require.Equal(t, ref.engine.CurrentTime(), resumed.engine.CurrentTime())
	require.True(t, resumed.quiescent())
}

func TestCheckpointKeepsBlockedCommittedResponse(t *testing.T) {
	spec := wakeSpec(4)
	spec.NumBanks = 2
	const blockedAddr = uint64(64)

	path := filepath.Join(t.TempDir(), "ck.tar.gz")
	ref := newWakeRig(t, spec, 1)
	ref.storage.Write(blockedAddr, []byte{1, 2, 3, 4})
	ref.deliverReads(5, 0, 1)
	require.NoError(t, ref.engine.RunUntil(cycleEdge(spec, 50)))
	require.Equal(t, []uint64{10}, ref.sends)
	require.NoError(t, ref.sim.SaveCheckpoint(path))

	resumed := newWakeRig(t, spec, 1)
	require.NoError(t, resumed.sim.LoadCheckpoint(path))
	item := resumed.comp.State.Banks[1].Pipeline.Stages()[0].Item
	require.True(t, item.Committed)
	require.Equal(t, []byte{1, 2, 3, 4}, item.ReadData[:4])

	resumed.at(100, func() { resumed.top.RetrieveOutgoing() })
	require.NoError(t, resumed.engine.Run())

	require.Equal(t, []uint64{101}, resumed.sends)
	require.Equal(t, []uint64{101, 102}, resumed.ticks)
	require.True(t, resumed.quiescent())
}

func TestParallelEngineCompletesTraffic(t *testing.T) {
	sim := akitasim.MakeBuilder().WithParallelEngine().Build()
	spec := wakeSpec(30)
	spec.NumBanks = 8
	comp := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{Storage: mem.NewStorage(1 * mem.MB)}).
		Build("Mem")
	ports := makePorts(sim, "Mem", 64, 4)
	comp.BindPort("Top", ports.Top)
	comp.BindPort("Control", ports.Control)
	agent := newBandwidthAgent(sim, "Agent")
	conn := direct.NewConnection("Conn", sim, timing.GHz)
	conn.BindPort(comp.Ports.Top)
	conn.BindPort(agent.port)
	(&noopConn{}).BindPort(comp.Ports.Control)
	require.NoError(t, sim.Initialize())

	const requests = 64
	for i := range requests {
		comp.Ports.Top.Deliver(makeReadReq(sim, agent.port.AsRemote(), comp.Ports.Top.AsRemote(), i))
	}

	require.NoError(t, sim.Engine().Run())

	require.Equal(t, requests, agent.completed)
	for i := range comp.State.Banks {
		require.True(t, bankIsQuiescent(&comp.State.Banks[i]))
	}
}
