package dram

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// p0Harness wires a DRAM controller to a source port through a direct
// connection so a test can drive requests and collect responses end-to-end.
type p0Harness struct {
	engine timing.Engine
	dram   *Comp
	src    messaging.Port
	top    messaging.Port
}

func newP0Harness(t *testing.T, spec Spec, tracers ...tasks.Observer) *p0Harness {
	t.Helper()

	sim := akitasim.MakeBuilder().WithTaskTracking().Build()
	engine := sim.Engine().(*timing.SerialEngine)

	dramComp := buildDRAM(sim, spec, "P0DRAM", 1024)
	for _, subject := range tracers {
		dramComp.TaskTracker().Subscribe(subject, dramComp)
	}

	top := dramComp.Ports.Top
	src := newDriverPort("P0Src.Top", 1024)

	conn := direct.NewConnection("P0Conn", sim, timing.GHz)
	conn.BindPort(top)
	conn.BindPort(src)
	require.NoError(t, sim.Initialize())

	return &p0Harness{engine: engine, dram: dramComp, src: src, top: top}
}

func (h *p0Harness) read(addr uint64) messaging.Msg {
	r := messaging.Msg{Payload: memprotocol.ReadReq{
		Address:        addr,
		AccessByteSize: 64},
		ID: h.dram.NewID(),

		Src:          h.src.AsRemote(),
		Dst:          h.top.AsRemote(),
		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	return r
}

func (h *p0Harness) write(addr uint64, data []byte) messaging.Msg {
	w := messaging.Msg{Payload: memprotocol.WriteReq{
		Address: addr,
		Data:    data},
		ID: h.dram.NewID(),

		Src:          h.src.AsRemote(),
		Dst:          h.top.AsRemote(),
		TrafficBytes: len(data) + 12,
		TrafficClass: "memprotocol.WriteReq"}

	return w
}

func (h *p0Harness) collect() (
	reads []messaging.Msg,
	writes []messaging.Msg,
) {
	for {
		msg, ok := h.src.RetrieveIncoming()
		if !ok {
			break
		}
		switch msg.Payload.(type) {
		case memprotocol.DataReadyRsp:
			m := msg

			reads = append(reads, m)
		case memprotocol.WriteDoneRsp:
			m := msg

			writes = append(writes, m)
		}
	}
	return reads, writes
}

// Addressing note for DefaultSpec (single channel, 2 ranks, 1 bank-group,
// 8 banks): the 64-byte access unit occupies bits [0,5]; column bits are
// [6,12]; bank bits are [13,15]; rank bit is [16]; row starts at [17]. So
// addresses that differ only in bits [6,12] hit the same bank+row, while a
// change in bit [17] is a same-bank row conflict.

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestP0OpenPagePanicRegression(t *testing.T) {
	openPageSpec := func() Spec {
		spec := DefaultSpec()
		spec.PagePolicy = PagePolicyOpen
		return spec
	}

	t.Run("should not panic on back-to-back same-row reads (the original bug)", func(t *testing.T) {
		h := newP0Harness(t, openPageSpec())

		// Two reads to the same bank+row (columns 0x40 and 0x80). The second is
		// a row-buffer hit issued while the first read's data is still in
		// flight — this used to panic at startCommand("previous cmd is not
		// completed") because bank occupancy was conflated with data latency.
		h.src.Send(h.read(0x40))
		h.src.Send(h.read(0x80))

		require.NoError(t, h.engine.Run())

		reads, _ := h.collect()
		require.Len(t, reads, 2)
	})

	t.Run("should handle same-bank row conflicts in open-page mode", func(t *testing.T) {
		h := newP0Harness(t, openPageSpec())

		// Same bank and rank, different rows (bit 17) → the controller must
		// precharge and re-activate between the two reads.
		h.src.Send(h.read(0x0))
		h.src.Send(h.read(0x20000))

		require.NoError(t, h.engine.Run())

		reads, _ := h.collect()
		require.Len(t, reads, 2)
	})

	t.Run("should pipeline many same-row reads without panic", func(t *testing.T) {
		h := newP0Harness(t, openPageSpec())

		const n = 8
		for i := range n {
			// Vary only the column bits → all same bank+row, all row-buffer
			// hits issued faster than readDelay. Exercises the decoupled
			// pending-completion timeline (multiple reads in flight at once).
			h.src.Send(h.read(uint64(i) * 64))
		}

		require.NoError(t, h.engine.Run())

		reads, _ := h.collect()
		require.Len(t, reads, n)
	})

	t.Run("should preserve write-then-read data in open-page mode", func(t *testing.T) {
		h := newP0Harness(t, openPageSpec())

		data := []byte{9, 8, 7, 6}

		// Drive the write to completion first so the read observes it.
		h.src.Send(h.write(0x40, data))
		require.NoError(t, h.engine.Run())
		_, writes := h.collect()
		require.Len(t, writes, 1)

		h.src.Send(h.read(0x40))
		require.NoError(t, h.engine.Run())
		reads, _ := h.collect()
		require.Len(t, reads, 1)
		require.Equal(t, data, reads[0].Payload.(memprotocol.DataReadyRsp).Data[:len(data)])
	})
}

func TestP0ClosePageCompletionLatency(t *testing.T) {
	// Regression for the auto-precharge completion bug: under the default
	// close-page policy, reads/writes are emitted as ReadPrecharge/WritePrecharge.
	// Their data/response must become ready after ReadDelay/WriteDelay (the data
	// return latency), NOT after TRP — the trailing precharge is enforced by the
	// bank timing table, not by the completion timeline.
	var cmdCycles map[commandKind]int

	setup1 := func(t *testing.T) {
		t.Helper()

		_, cmdCycles = buildDDR4TimingAndCycles()
	}

	t.Run("maps the read variants to ReadDelay, not TRP", func(t *testing.T) {
		setup1(t)

		// Precondition: the bug is only observable when the two differ.
		require.NotEqual(t, cmdCycles[cmdKindRead], DDR4Spec().TRP)

		require.Equal(t, cmdCycles[cmdKindRead], cmdCycles[cmdKindReadPrecharge])
	})

	t.Run("maps the write variants to WriteDelay, not TRP", func(t *testing.T) {
		setup1(t)

		require.Equal(t, cmdCycles[cmdKindWrite], cmdCycles[cmdKindWritePrecharge])
	})

	t.Run("schedules a ReadPrecharge completion at ReadDelay", func(t *testing.T) {
		setup1(t)

		st := newDDR4State()
		bs := findBankState(&st.BankStates, 0, 0, 0)
		bs.State = int(bankStateOpen)
		bs.OpenRow = 0

		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 0},
		}
		startCommand(cmdCycles, st, bs, cmd)

		require.Len(t, st.PendingCompletions, 1)
		require.Equal(t, st.TickCount+uint64(cmdCycles[cmdKindRead]), st.PendingCompletions[0].CompletionTick)
	})
}

func TestP0ChannelGuard(t *testing.T) {
	build := func(t *testing.T, numChannel int) func() {
		t.Helper()

		return func() {
			sim := akitasim.MakeBuilder().WithTaskTracking().Build()

			spec := DefaultSpec()
			spec.NumChannel = numChannel
			buildDRAM(sim, spec, "ChannelGuard", 16)
			require.NoError(t, sim.Initialize())
		}
	}

	t.Run("should reject NumChannel > 1 at build time", func(t *testing.T) {
		func() {
			defer func() {
				recovered := recover()
				require.NotNil(t, recovered)
				require.Contains(t, fmt.Sprint(recovered), "NumChannel > 1")
			}()
			(build(t, 2))()
		}()
	})

	t.Run("should still build with a single channel", func(t *testing.T) {
		require.NotPanics(t, build(t, 1))
	})
}
