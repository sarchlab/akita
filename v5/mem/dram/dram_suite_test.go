package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/directconnection"

	"go.uber.org/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port
//go:generate mockgen -destination "mock_trans_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/dram/internal/trans SubTransactionQueue,SubTransSplitter
//go:generate mockgen -destination "mock_addressmapping_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/dram/internal/addressmapping Mapper
//go:generate mockgen -destination "mock_cmdq_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/dram/internal/cmdq CommandQueue
//go:generate mockgen -destination "mock_org_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/dram/internal/org Channel
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/mem/mem AddressConverter

func TestDram(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dram Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
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
			WithTopPort(sim.NewPort(nil, 1024, 1024, "MemCtrl.TopPort")).
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
		writeData := []byte{1, 2, 3, 4}
		write := &mem.WriteReq{}
		write.ID = sim.GetIDGenerator().Generate()
		write.Address = 0x40
		write.Data = writeData
		write.Src = srcPort.AsRemote()
		write.Dst = memCtrl.topPort.AsRemote()
		write.TrafficBytes = len(writeData) + 12
		write.TrafficClass = "mem.WriteReq"

		read := &mem.ReadReq{}
		read.ID = sim.GetIDGenerator().Generate()
		read.Address = 0x40
		read.AccessByteSize = 4
		read.Src = srcPort.AsRemote()
		read.Dst = memCtrl.topPort.AsRemote()
		read.TrafficBytes = 12
		read.TrafficClass = "mem.ReadReq"

		memCtrl.topPort.Deliver(write)
		memCtrl.topPort.Deliver(read)

		ret1 := srcPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				wdRsp := msg.(*mem.WriteDoneRsp)
				Expect(wdRsp.RspTo).To(Equal(write.ID))
			})
		srcPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(msg sim.Msg) {
				dr := msg.(*mem.DataReadyRsp)
				Expect(dr.RspTo).To(Equal(read.ID))
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			}).After(ret1)

		engine.Run()
	})
})
