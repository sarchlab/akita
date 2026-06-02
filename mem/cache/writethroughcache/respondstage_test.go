package writethroughcache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Respond Stage", func() {
	var (
		mw      *pipelineMW
		topPort messaging.Port
		s       *respondStage
	)

	BeforeEach(func() {
		mw = &pipelineMW{}
		mw.comp = modeling.NewBuilder[Spec, State, Resources]().
			WithEngine(timing.NewSerialEngine()).
			WithFreq(1 * timing.GHz).
			WithSpec(Spec{}).
			Build("Cache")

		// topPort is a real, single-slot port (owned by the component) so the
		// "cannot send" cases can be forced by pre-filling its outgoing buffer.
		topPort = messaging.NewPort(mw.comp, 1, 1, "Cache.Top")
		(&noopConn{}).PlugIn(topPort)
		mw.topPort = topPort
		mw.comp.AddPort("Top", topPort)

		s = &respondStage{cache: mw}
	})

	// fillOutgoing pre-fills topPort's single outgoing slot so the next Send
	// fails, simulating a busy port.
	fillOutgoing := func() {
		dummy := &mem.DataReadyRsp{}
		dummy.Src = topPort.AsRemote()
		dummy.Dst = messaging.RemotePort("SomeSrc")
		dummy.TrafficClass = "rsp"
		Expect(topPort.Send(dummy)).To(BeNil())
	}

	Context("read", func() {
		var readMeta messaging.MsgMeta

		BeforeEach(func() {
			next := &mw.comp.State

			readMeta = messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          "SomeSrc",
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasRead:            true,
					ReadMeta:           readMeta,
					ReadAddress:        0x100,
					ReadAccessByteSize: 4,
					ReadPID:            1,
				},
			)
		})

		It("should stall if cannot send to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true
			fillOutgoing()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeTrue())

			out := topPort.RetrieveOutgoing()
			dr := out.(*mem.DataReadyRsp)
			Expect(dr.RspTo).To(Equal(readMeta.ID))
			Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
		})
	})

	Context("write", func() {
		var writeMeta messaging.MsgMeta

		BeforeEach(func() {
			next := &mw.comp.State

			writeMeta = messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          "SomeSrc",
				TrafficBytes: 12,
				TrafficClass: "req",
			}

			next.Transactions = append(next.Transactions,
				transactionState{
					HasWrite:     true,
					WriteMeta:    writeMeta,
					WriteAddress: 0x100,
					WritePID:     1,
				},
			)
		})

		It("should stall if cannot send to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Done = true
			fillOutgoing()

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			next := &mw.comp.State
			next.Transactions[0].Data = []byte{1, 2, 3, 4}
			next.Transactions[0].Done = true

			madeProgress := s.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(next.Transactions[0].Removed).To(BeTrue())

			out := topPort.RetrieveOutgoing()
			Expect(out.Meta().RspTo).To(Equal(writeMeta.ID))
		})
	})

})
