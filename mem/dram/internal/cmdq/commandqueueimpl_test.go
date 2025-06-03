package cmdq

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/dram/internal/addressmapping"
	"github.com/sarchlab/akita/v4/mem/dram/internal/signal"
	"go.uber.org/mock/gomock"
)

var _ = Describe("CommandQueueImpl", func() {
	var (
		mockCtrl *gomock.Controller
		channel  *MockChannel
		q        CommandQueueImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		channel = NewMockChannel(mockCtrl)
		q = CommandQueueImpl{
			Queues:           make([]Queue, 8),
			CapacityPerQueue: 8,
			nextQueueIndex:   0,
			Channel:          channel,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should get the next command to issue", func() {
		cmd1 := &signal.Command{
			ID:   "1",
			Kind: signal.CmdKindRead,
			Location: addressmapping.Location{
				Rank: 0,
				Bank: 0,
			},
		}
		q.Queues[0] = append(q.Queues[0], cmd1)

		cmd2 := &signal.Command{
			ID:   "2",
			Kind: signal.CmdKindRead,
			Location: addressmapping.Location{
				Rank: 0,
				Bank: 0,
			},
		}
		q.Queues[0] = append(q.Queues[0], cmd2)

		cmd3 := &signal.Command{
			ID:   "3",
			Kind: signal.CmdKindRead,
			Location: addressmapping.Location{
				Rank: 0,
				Bank: 1,
			},
		}
		q.Queues[1] = append(q.Queues[1], cmd3)

		channel.EXPECT().
			GetReadyCommand(cmd1).
			Return(nil)
		channel.EXPECT().
			GetReadyCommand(cmd2).
			Return(cmd2)

		readyCmd := q.GetCommandToIssue()

		Expect(readyCmd).To(BeIdenticalTo(cmd2))
		Expect(q.Queues[0]).NotTo(ContainElement(cmd2))
	})

	It("should accept new commands", func() {
		cmd := &signal.Command{}

		Expect(q.CanAccept(cmd)).To(BeTrue())

		q.Accept(cmd)

		Expect(q.Queues[0]).To(ContainElement(cmd))
	})
})
