package trans

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/dram/internal/addressmapping"
	"github.com/sarchlab/akita/v5/mem/dram/internal/signal"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("ClosePageCommandCreator", func() {
	var (
		mockCtrl   *gomock.Controller
		mapper     *MockMapper
		cmdCreator *ClosePageCommandCreator
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mapper = NewMockMapper(mockCtrl)
		cmdCreator = &ClosePageCommandCreator{
			AddrMapper: mapper,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should create read precharge commands", func() {
		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"
		trans := &signal.Transaction{Read: read}
		subTrans := &signal.SubTransaction{
			Transaction: trans,
			Address:     0x40,
		}

		mapper.EXPECT().Map(uint64(0x40)).Return(addressmapping.Location{
			Channel:   1,
			Rank:      2,
			BankGroup: 3,
			Bank:      4,
			Row:       5,
			Column:    6,
		})

		cmd := cmdCreator.Create(subTrans)

		Expect(cmd.Kind).To(Equal(signal.CmdKindReadPrecharge))
		Expect(cmd.SubTrans).To(BeIdenticalTo(subTrans))
	})
})
