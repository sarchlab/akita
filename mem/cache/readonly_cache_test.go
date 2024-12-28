package cache_test

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/noc/directconnection"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

var _ = Describe("ReadOnlyCache", func() {
	var (
		mockCtrl *gomock.Controller
		engine   timing.Engine
		sim      simulation.Simulation
		port     *cache.MockPort
		conn     *directconnection.Comp
		m        *idealmemcontroller.Comp
		c        *cache.Comp
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = timing.NewSerialEngine()

		sim = simulation.NewSimulation()
		sim.RegisterEngine(engine)

		port = cache.NewMockPort(mockCtrl)
		port.EXPECT().AsRemote().Return(modeling.RemotePort("Agent")).AnyTimes()
		port.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		port.EXPECT().NotifyAvailable().AnyTimes()
		port.EXPECT().PeekOutgoing().Return(nil).AnyTimes()
		port.EXPECT().RetrieveOutgoing().Return(nil).AnyTimes()

		m = idealmemcontroller.MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).
			Build("Mem")
		addrToDstTable := mem.SinglePortMapper{
			Port: m.GetPortByName("Top").AsRemote(),
		}

		c = cache.MakeBuilder().
			WithSimulation(sim).
			WithFreq(1 * timing.GHz).
			WithNumReqPerCycle(1).
			WithLog2CacheLineSize(6).
			WithWayAssociativity(1).
			WithMSHRCapacity(4).
			WithWriteStrategy("readOnly").
			WithAddressToDstTable(addrToDstTable).
			Build("Cache")

		conn = directconnection.MakeBuilder().
			WithEngine(engine).
			Build("Conn")
		conn.PlugIn(port)
		conn.PlugIn(c.GetPortByName("Top"))
		conn.PlugIn(c.GetPortByName("Bottom"))
		conn.PlugIn(m.GetPortByName("Top"))
	})

	FIt("should be able to read from the memory", func() {
		readReq := mem.ReadReq{
			MsgMeta: modeling.MsgMeta{
				ID:  id.Generate(),
				Src: port.AsRemote(),
				Dst: c.GetPortByName("Top").AsRemote(),
			},
			Address:        0,
			AccessByteSize: 4,
		}

		c.GetPortByName("Top").Deliver(readReq)

		port.EXPECT().Deliver(gomock.Any()).Do(func(msg modeling.Msg) {
			dr := msg.(mem.DataReadyRsp)
			Expect(dr.Data).To(Equal([]byte{0, 0, 0, 0}))
			Expect(dr.RespondTo).To(Equal(readReq.ID))
		})

		engine.Run()

	})

})
