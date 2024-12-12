package dram

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"

	"github.com/golang/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_modeling_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Port
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
		engine   timing.Engine
		srcPort  *MockPort
		memCtrl  *Comp
		conn     *directconnection.Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()
		memCtrl = MakeBuilder().
			WithEngine(engine).
			Build("MemCtrl")
		srcPort = NewMockPort(mockCtrl)
		srcPort.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		srcPort.EXPECT().
			AsRemote().
			Return(modeling.RemotePort("SrcPort")).
			AnyTimes()

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			WithFreq(1 * timing.GHz).
			Build("Conn")
		srcPort.EXPECT().SetConnection(conn)
		conn.PlugIn(memCtrl.topPort)
		conn.PlugIn(srcPort)
	})

	It("should read and write", func() {
		write := mem.WriteReq{
			MsgMeta: modeling.MsgMeta{
				Src: srcPort.AsRemote(),
				Dst: memCtrl.topPort.AsRemote(),
			},
			Address: 0x40,
			Data:    []byte{1, 2, 3, 4},
		}

		read := mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				Src: srcPort.AsRemote(),
				Dst: memCtrl.topPort.AsRemote(),
			},
			Address:        0x40,
			AccessByteSize: 4,
		}

		memCtrl.topPort.Deliver(write)
		memCtrl.topPort.Deliver(read)

		ret1 := srcPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(wd mem.WriteDoneRsp) {
				Expect(wd.RespondTo).To(Equal(write.ID))
			})
		srcPort.EXPECT().
			Deliver(gomock.Any()).
			Do(func(dr mem.DataReadyRsp) {
				Expect(dr.RespondTo).To(Equal(read.ID))
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			}).After(ret1)

		engine.Run()
	})
})
