package dram

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"

	"go.uber.org/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port
//go:generate mockgen -destination "mock_trans_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/dram/internal/trans SubTransactionQueue,SubTransSplitter
//go:generate mockgen -destination "mock_addressmapping_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/dram/internal/addressmapping Mapper
//go:generate mockgen -destination "mock_cmdq_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/dram/internal/cmdq CommandQueue
//go:generate mockgen -destination "mock_org_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/dram/internal/org Channel
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/mem/mem AddressConverter

func TestDram(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dram Suite")
}

var _ = Describe("DRAM Integration", func() {
	var (
		mockCtrl *gomock.Controller
		engine   sim.Engine
		srcPort  *MockPort
		memCtrl  *Comp
		conn     *directconnection.Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()
		memCtrl = MakeBuilder().
			WithEngine(engine).
			Build("MemCtrl")
		srcPort = NewMockPort(mockCtrl)
		srcPort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		srcPort.EXPECT().AsRemote().Return(sim.RemotePort("SrcPort")).AnyTimes()

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * sim.GHz).
			Build("Conn")
		srcPort.EXPECT().SetConnection(conn)
		conn.PlugIn(memCtrl.topPort)
		conn.PlugIn(srcPort)
	})

	It("should read and write", func() {
		write := mem.WriteReqBuilder{}.
			WithAddress(0x40).
			WithData([]byte{1, 2, 3, 4}).
			WithSrc(srcPort.AsRemote()).
			WithDst(memCtrl.topPort.AsRemote()).
			Build()

		read := mem.ReadReqBuilder{}.
			WithAddress(0x40).
			WithByteSize(4).
			WithSrc(srcPort.AsRemote()).
			WithDst(memCtrl.topPort.AsRemote()).
			Build()

		memCtrl.topPort.Deliver(write)
		memCtrl.topPort.Deliver(read)

		ret1 := srcPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(wd *mem.WriteDoneRsp) {
				Expect(wd.RespondTo).To(Equal(write.ID))
			})
		srcPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.RespondTo).To(Equal(read.ID))
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			}).After(ret1)

		engine.Run()
	})
})
