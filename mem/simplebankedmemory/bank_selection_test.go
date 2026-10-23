package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// These tests cover bank selection when the memory is one of several
// interleaved controllers over a shared, globally-addressed storage. Storage is
// always global; the optional bank-selection conversion strips the upstream
// inter-controller interleaving so a contiguous bank selector can stripe finely
// across all banks instead of aliasing on the strided global address.
func TestBankSelectionAcrossInterleavedControllers(t *testing.T) {
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
		spec.BankAddressConversion.Kind = ConversionInterleaving
		spec.BankAddressConversion.InterleavingSize = interleave
		spec.BankAddressConversion.NumElements = numElements
		spec.BankAddressConversion.ElementIndex = elementIndex
		return spec
	}

	t.Run("collapses onto a fraction of banks without the bank conversion", func(t *testing.T) {
		spec := DefaultSpec()
		spec.NumBanks = numBanks
		spec.BankSelectorLog2InterleaveSize = bankLog2
		// No bank conversion: selection runs on the strided global address.

		banks := distinctBanks(spec, element1Addresses())

		require.Len(t, banks, 2)
	})

	t.Run("spreads finely across all banks with the bank conversion", func(t *testing.T) {
		banks := distinctBanks(withBankConv(), element1Addresses())

		require.Len(t, banks, numBanks)
	})

	t.Run("places adjacent controller-local lines on different banks", func(t *testing.T) {
		spec := withBankConv()

		// The two 64-B lines of element 1's first block (128 and 192) become
		// controller-local 0 and 64, landing on different banks — the
		// fine-grained striping that bit-placement alone cannot achieve.
		require.Equal(t, uint64(0), bankSelectionAddress(spec, 128))
		require.Equal(t, uint64(64), bankSelectionAddress(spec, 192))
		require.NotEqual(
			t,
			selectBank(spec, bankSelectionAddress(spec, 192)),
			selectBank(spec, bankSelectionAddress(spec, 128)),
		)
	})

	t.Run("leaves storage addressing untouched (storage is global)", func(t *testing.T) {
		// The bank conversion does not affect storage: bankSelectionAddress is
		// only ever fed to selectBank, never to the storage access (see
		// tickFinalizeMW, which reads/writes at the request's raw address).
		spec := withBankConv()
		require.Equal(t, uint64(128), bankSelectionAddress(spec, 640))
	})
}

// This integration test confirms that, with a global identity-addressed storage
// and the bank conversion enabled, reads and writes hit the correct global
// offset and exercise more than one bank.
func TestBankSelectionDataCorrectnessWithGlobalStorage(t *testing.T) {
	var (
		engine  timing.Engine
		sim     *testsim.Simulation
		memComp *Comp
		agent   *testAgent
		conn    *loopbackConnection
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		engine = timing.NewSerialEngine()
		sim = testsim.New(engine, false)

		spec := DefaultSpec()
		spec.NumBanks = 4
		spec.StageLatency = 2
		spec.BankSelectorLog2InterleaveSize = 6
		// Strip the 128 B/4-way controller interleave for bank selection.
		spec.BankAddressConversion.Kind = ConversionInterleaving
		spec.BankAddressConversion.InterleavingSize = 128
		spec.BankAddressConversion.NumElements = 4
		spec.BankAddressConversion.ElementIndex = 1

		memComp = modeling.MakeBuilder[*Comp](spec).
			WithSimulation(sim).
			WithResources(Resources{Storage: mem.NewStorage(4 * mem.GB)}).
			Build("MemBank")
		memCompPorts := makePorts(sim, "MemBank", 8, 16)
		memComp.BindPort("Top", memCompPorts.Top)
		memComp.BindPort("Control", memCompPorts.Control)

		agent = newTestAgent(sim, "AgentBank")
		conn = newLoopbackConnection("ConnBank")
		conn.BindPort(memComp.Ports.Top)
		conn.BindPort(agent.port)
		if err := sim.Initialize(); err != nil {
			panic(err)
		}
	}

	t.Run("writes and reads back at the global address across banks", func(t *testing.T) {
		setup1(t)

		tp := memComp.Ports.Top

		// Two element-1 lines that select different banks (local 0 and 64).
		addrs := []uint64{128, 192}

		spec := memComp.Spec
		require.NotEqual(
			t,
			selectBank(spec, bankSelectionAddress(spec, addrs[1])),
			selectBank(spec, bankSelectionAddress(spec, addrs[0])),
		)

		for i, a := range addrs {
			w := messaging.Msg{Payload: memprotocol.WriteReq{
				Address: a,
				Data:    []byte{byte(i + 1), byte(i + 2), byte(i + 3), byte(i + 4)}},
				ID:  sim.NewID(),
				Src: agent.port.AsRemote(),
				Dst: tp.AsRemote(),
			}

			agent.send(t, w)
		}

		for i := 0; i < 20; i++ {
			modelingtest.Tick(memComp)
		}

		for i, a := range addrs {
			r := messaging.Msg{Payload: memprotocol.ReadReq{
				Address:        a,
				AccessByteSize: 4},
				ID:  sim.NewID(),
				Src: agent.port.AsRemote(),
				Dst: tp.AsRemote(),
			}

			agent.send(t, r)

			for j := 0; j < 20; j++ {
				modelingtest.Tick(memComp)
			}
			_, ok := agent.received[len(agent.received)-1].Payload.(memprotocol.DataReadyRsp)
			rsp := agent.received[len(agent.received)-1]
			require.True(t, ok)
			require.Equal(
				t,
				[]byte{byte(i + 1), byte(i + 2), byte(i + 3), byte(i + 4)},
				rsp.Payload.(memprotocol.DataReadyRsp).Data,
			)
		}
	})
}
