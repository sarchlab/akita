package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests cover bank selection when the memory is one of several
// interleaved controllers over a shared, globally-addressed storage. Bank
// selection runs on the global request address (there is no per-controller
// conversion), so the bank-select bits must be chosen to sit above the
// upstream controller-select bits; otherwise the two interleavings overlap and
// accesses collapse onto a fraction of the banks.
var _ = Describe("Bank selection across interleaved controllers", func() {
	// Four controllers interleaved at 128 B; this memory is element 1. Within
	// element 1 the visible addresses are 64-B lines inside every 4th 128-B
	// block: 128, 192, 640, 704, 1152, 1216, ... The controller-select bits are
	// [7:9) (128 B stride, 4 controllers).
	const (
		numBanks      = 4
		numElements   = 4
		elementIndex  = 1
		interleave    = 128 // upstream inter-controller stride (bytes)
		roundSize     = interleave * numElements
		numLinesToHit = 16
	)

	element1Addresses := func() []uint64 {
		addrs := make([]uint64, 0, numLinesToHit)
		for len(addrs) < numLinesToHit {
			block := uint64(len(addrs)/2) * roundSize
			base := block + elementIndex*interleave
			addrs = append(addrs, base, base+64) // two 64-B lines per 128-B block
		}
		return addrs
	}

	distinctBanks := func(spec Spec, addrs []uint64) map[int]struct{} {
		banks := map[int]struct{}{}
		for _, a := range addrs {
			banks[selectBank(spec, a)] = struct{}{}
		}
		return banks
	}

	It("collapses onto a fraction of banks when bank bits overlap controller bits",
		func() {
			spec := DefaultSpec()
			spec.NumBanks = numBanks
			// 64 B bank stride -> bank bits [6:8) overlap the controller bits
			// [7:9): the hazard.
			spec.BankSelectorLog2InterleaveSize = 6

			banks := distinctBanks(spec, element1Addresses())

			Expect(banks).To(HaveLen(2))
		})

	It("spreads across all banks when bank bits sit above controller bits",
		func() {
			spec := DefaultSpec()
			spec.NumBanks = numBanks
			// Place bank bits above the controller bits:
			// log2(interleave * numElements) = log2(512) = 9.
			spec.BankSelectorLog2InterleaveSize = 9

			banks := distinctBanks(spec, element1Addresses())

			Expect(banks).To(HaveLen(numBanks))
		})
})

// This integration test confirms that, with a global identity-addressed
// storage (no per-controller conversion) and bank bits chosen above the
// controller bits, reads and writes hit the correct global offset and exercise
// more than one bank.
var _ = Describe("Bank selection data correctness with global storage", func() {
	var (
		engine  timing.Engine
		memComp *Comp
		agent   *testAgent
		conn    *loopbackConnection
	)

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		spec := DefaultSpec()
		spec.NumBanks = 4
		spec.StageLatency = 2
		spec.BankSelectorLog2InterleaveSize = 9 // above the 128 B/4-way controllers

		memComp = MakeBuilder().WithRegistrar(reg).WithSpec(spec).Build("MemBank")
		assignPort(reg, memComp, "Top", 8)
		assignPort(reg, memComp, "Control", 16)

		agent = newTestAgent("AgentBank")
		conn = newLoopbackConnection("ConnBank")
		conn.PlugIn(memComp.GetPortByName("Top"))
		conn.PlugIn(agent.port)
	})

	It("writes and reads back at the global address across banks", func() {
		tp := memComp.GetPortByName("Top")

		// Two element-1 addresses that select different banks under the
		// above-controller-bit interleave (128 -> bank 0, 640 -> bank 1).
		addrs := []uint64{128, 640}

		spec := memComp.Spec()
		Expect(selectBank(spec, addrs[0])).NotTo(Equal(selectBank(spec, addrs[1])))

		for i, a := range addrs {
			w := memprotocol.WriteReq{}
			w.ID = timing.GetIDGenerator().Generate()
			w.Src = agent.port.AsRemote()
			w.Dst = tp.AsRemote()
			w.Address = a
			w.Data = []byte{byte(i + 1), byte(i + 2), byte(i + 3), byte(i + 4)}
			w.TrafficClass = "memprotocol.WriteReq"
			agent.send(w)
		}

		for i := 0; i < 20; i++ {
			memComp.Tick()
		}

		for i, a := range addrs {
			r := memprotocol.ReadReq{}
			r.ID = timing.GetIDGenerator().Generate()
			r.Src = agent.port.AsRemote()
			r.Dst = tp.AsRemote()
			r.Address = a
			r.AccessByteSize = 4
			r.TrafficClass = "memprotocol.ReadReq"
			agent.send(r)

			for j := 0; j < 20; j++ {
				memComp.Tick()
			}

			rsp, ok := agent.received[len(agent.received)-1].(memprotocol.DataReadyRsp)
			Expect(ok).To(BeTrue())
			Expect(rsp.Data).To(Equal(
				[]byte{byte(i + 1), byte(i + 2), byte(i + 3), byte(i + 4)}))
		}
	})
})
