package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests cover bank selection when the memory is one of several
// interleaved controllers over a shared, globally-addressed storage. Storage is
// always global; the optional bank-selection conversion strips the upstream
// inter-controller interleaving so a contiguous bank selector can stripe finely
// across all banks instead of aliasing on the strided global address.
var _ = Describe("Bank selection across interleaved controllers", func() {
	// Four controllers interleaved at 128 B; this memory is element 1. Within
	// element 1 the visible addresses are 64-B lines inside every 4th 128-B
	// block: 128, 192, 640, 704, 1152, 1216, ...
	const (
		numBanks      = 4
		numElements   = 4
		elementIndex  = 1
		interleave    = 128 // upstream inter-controller stride (bytes)
		bankLog2      = 6   // 64 B bank stride
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
			banks[selectBank(spec, bankSelectionAddress(spec, a))] = struct{}{}
		}
		return banks
	}

	// withBankConv returns a spec whose bank selection strips the 128 B/4-way
	// controller interleaving for element 1.
	withBankConv := func() Spec {
		spec := DefaultSpec()
		spec.NumBanks = numBanks
		spec.BankSelectorLog2InterleaveSize = bankLog2
		spec.BankAddrConvKind = "interleaving"
		spec.BankAddrInterleavingSize = interleave
		spec.BankAddrTotalNumOfElements = numElements
		spec.BankAddrCurrentElementIndex = elementIndex
		return spec
	}

	It("collapses onto a fraction of banks without the bank conversion", func() {
		spec := DefaultSpec()
		spec.NumBanks = numBanks
		spec.BankSelectorLog2InterleaveSize = bankLog2
		// No bank conversion: selection runs on the strided global address.

		banks := distinctBanks(spec, element1Addresses())

		Expect(banks).To(HaveLen(2))
	})

	It("spreads finely across all banks with the bank conversion", func() {
		banks := distinctBanks(withBankConv(), element1Addresses())

		Expect(banks).To(HaveLen(numBanks))
	})

	It("places adjacent controller-local lines on different banks", func() {
		spec := withBankConv()

		// The two 64-B lines of element 1's first block (128 and 192) become
		// controller-local 0 and 64, landing on different banks — the
		// fine-grained striping that bit-placement alone cannot achieve.
		Expect(bankSelectionAddress(spec, 128)).To(Equal(uint64(0)))
		Expect(bankSelectionAddress(spec, 192)).To(Equal(uint64(64)))
		Expect(selectBank(spec, bankSelectionAddress(spec, 128))).
			NotTo(Equal(selectBank(spec, bankSelectionAddress(spec, 192))))
	})

	It("leaves storage addressing untouched (storage is global)", func() {
		// The bank conversion does not affect storage: bankSelectionAddress is
		// only ever fed to selectBank, never to the storage access (see
		// tickFinalizeMW, which reads/writes at the request's raw address).
		spec := withBankConv()
		Expect(bankSelectionAddress(spec, 640)).To(Equal(uint64(128)))
	})
})

// This integration test confirms that, with a global identity-addressed storage
// and the bank conversion enabled, reads and writes hit the correct global
// offset and exercise more than one bank.
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
		spec.BankSelectorLog2InterleaveSize = 6
		// Strip the 128 B/4-way controller interleave for bank selection.
		spec.BankAddrConvKind = "interleaving"
		spec.BankAddrInterleavingSize = 128
		spec.BankAddrTotalNumOfElements = 4
		spec.BankAddrCurrentElementIndex = 1

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

		// Two element-1 lines that select different banks (local 0 and 64).
		addrs := []uint64{128, 192}

		spec := memComp.Spec()
		Expect(selectBank(spec, bankSelectionAddress(spec, addrs[0]))).
			NotTo(Equal(selectBank(spec, bankSelectionAddress(spec, addrs[1]))))

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
