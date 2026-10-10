package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

// p0Harness wires a DRAM controller to a source port through a direct
// connection so a test can drive requests and collect responses end-to-end.
type p0Harness struct {
	engine timing.Engine
	dram   *Comp
	src    messaging.Port
	top    messaging.Port
}

func newP0Harness(spec Spec, tracers ...tracing.Tracer) *p0Harness {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	dramComp := buildDRAM(sim, spec, "P0DRAM", 1024)
	for _, t := range tracers {
		tracing.CollectTrace(dramComp, t)
	}

	top := dramComp.Ports.Top
	src := newDriverPort("P0Src.Top", 1024)

	conn := direct.NewConnection("P0Conn", sim, timing.GHz)
	conn.BindPort(top)
	conn.BindPort(src)
	Expect(sim.Initialize()).To(Succeed())

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

var _ = Describe("P0: open-page panic regression", func() {
	openPageSpec := func() Spec {
		spec := Definition.DefaultSpec
		spec.PagePolicy = PagePolicyOpen
		return spec
	}

	It("should not panic on back-to-back same-row reads (the original bug)", func() {
		h := newP0Harness(openPageSpec())

		// Two reads to the same bank+row (columns 0x40 and 0x80). The second is
		// a row-buffer hit issued while the first read's data is still in
		// flight — this used to panic at startCommand("previous cmd is not
		// completed") because bank occupancy was conflated with data latency.
		h.src.Send(h.read(0x40))
		h.src.Send(h.read(0x80))

		Expect(h.engine.Run()).To(Succeed())

		reads, _ := h.collect()
		Expect(reads).To(HaveLen(2))
	})

	It("should handle same-bank row conflicts in open-page mode", func() {
		h := newP0Harness(openPageSpec())

		// Same bank and rank, different rows (bit 17) → the controller must
		// precharge and re-activate between the two reads.
		h.src.Send(h.read(0x0))
		h.src.Send(h.read(0x20000))

		Expect(h.engine.Run()).To(Succeed())

		reads, _ := h.collect()
		Expect(reads).To(HaveLen(2))
	})

	It("should pipeline many same-row reads without panic", func() {
		h := newP0Harness(openPageSpec())

		const n = 8
		for i := range n {
			// Vary only the column bits → all same bank+row, all row-buffer
			// hits issued faster than readDelay. Exercises the decoupled
			// pending-completion timeline (multiple reads in flight at once).
			h.src.Send(h.read(uint64(i) * 64))
		}

		Expect(h.engine.Run()).To(Succeed())

		reads, _ := h.collect()
		Expect(reads).To(HaveLen(n))
	})

	It("should preserve write-then-read data in open-page mode", func() {
		h := newP0Harness(openPageSpec())

		data := []byte{9, 8, 7, 6}

		// Drive the write to completion first so the read observes it.
		h.src.Send(h.write(0x40, data))
		Expect(h.engine.Run()).To(Succeed())
		_, writes := h.collect()
		Expect(writes).To(HaveLen(1))

		h.src.Send(h.read(0x40))
		Expect(h.engine.Run()).To(Succeed())
		reads, _ := h.collect()
		Expect(reads).To(HaveLen(1))
		Expect(reads[0].Payload.(memprotocol.DataReadyRsp).Data[:len(data)]).To(Equal(data))
	})
})

var _ = Describe("P0: close-page completion latency", func() {
	// Regression for the auto-precharge completion bug: under the default
	// close-page policy, reads/writes are emitted as ReadPrecharge/WritePrecharge.
	// Their data/response must become ready after ReadDelay/WriteDelay (the data
	// return latency), NOT after TRP — the trailing precharge is enforced by the
	// bank timing table, not by the completion timeline.
	var cmdCycles map[commandKind]int

	BeforeEach(func() {
		_, cmdCycles = buildDDR4TimingAndCycles()
	})

	It("maps the read variants to ReadDelay, not TRP", func() {
		// Precondition: the bug is only observable when the two differ.
		Expect(DDR4Spec.TRP).NotTo(Equal(cmdCycles[cmdKindRead]))

		Expect(cmdCycles[cmdKindReadPrecharge]).
			To(Equal(cmdCycles[cmdKindRead]))
	})

	It("maps the write variants to WriteDelay, not TRP", func() {
		Expect(cmdCycles[cmdKindWritePrecharge]).
			To(Equal(cmdCycles[cmdKindWrite]))
	})

	It("schedules a ReadPrecharge completion at ReadDelay", func() {
		st := newDDR4State()
		bs := findBankState(&st.BankStates, 0, 0, 0)
		bs.State = int(bankStateOpen)
		bs.OpenRow = 0

		cmd := &commandState{
			Kind:     int(cmdKindReadPrecharge),
			Location: location{Rank: 0, BankGroup: 0, Bank: 0, Row: 0},
		}
		startCommand(cmdCycles, st, bs, cmd)

		Expect(st.PendingCompletions).To(HaveLen(1))
		Expect(st.PendingCompletions[0].CompletionTick).
			To(Equal(st.TickCount + uint64(cmdCycles[cmdKindRead])))
	})
})

var _ = Describe("P0: channel guard", func() {
	build := func(numChannel int) func() {
		return func() {
			engine := timing.NewSerialEngine()
			sim := modeling.NewStandaloneSimulation(engine)

			spec := Definition.DefaultSpec
			spec.NumChannel = numChannel
			buildDRAM(sim, spec, "ChannelGuard", 16)
			Expect(sim.Initialize()).To(Succeed())
		}
	}

	It("should reject NumChannel > 1 at build time", func() {
		Expect(build(2)).To(PanicWith(ContainSubstring("NumChannel > 1")))
	})

	It("should still build with a single channel", func() {
		Expect(build(1)).NotTo(Panic())
	})
})
