package mmuCache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("MMUCacheCtrlMiddleware", func() {
	var (
		mockCtrl    *gomock.Controller
		comp        *modeling.Component[Spec, State]
		ctrl        *ctrlMiddleware
		topPort     *MockPort
		bottomPort  *MockPort
		controlPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		topPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		bottomPort = NewMockPort(mockCtrl)
		bottomPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()
		controlPort = NewMockPort(mockCtrl)
		controlPort.EXPECT().AsRemote().Return(sim.RemotePort("ControlPort")).AnyTimes()
		controlPort.EXPECT().Name().Return("ControlPort").AnyTimes()
		controlPort.EXPECT().SetComponent(gomock.Any()).AnyTimes()

		spec := Spec{
			NumBlocks:       1,
			NumLevels:       5,
			PageSize:        4096,
			Log2PageSize:    12,
			NumReqPerCycle:  4,
			LatencyPerLevel: 100,
		}

		initialState := State{
			CurrentState: mmuCacheStatePause,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		}

		comp = modeling.NewBuilder[Spec, State]().
			WithSpec(spec).
			Build("MMUCache")
		comp.SetState(initialState)

		comp.AddPort("Top", topPort)
		comp.AddPort("Bottom", bottomPort)
		comp.AddPort("Control", controlPort)

		ctrl = &ctrlMiddleware{comp: comp}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing when no control message", func() {
		controlPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := ctrl.Tick()

		Expect(madeProgress).To(BeFalse())
	})

	It("should restart and drain ports", func() {
		req := &mem.ControlReq{Command: mem.CmdReset}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = sim.RemotePort("Requester")
		req.TrafficClass = "mem.ControlReq"

		topMsg := &vm.TranslationReq{}
		topMsg.ID = sim.GetIDGenerator().Generate()
		topMsg.PID = 1
		topMsg.VAddr = 0x1000
		topMsg.DeviceID = 1
		topMsg.TrafficClass = "vm.TranslationReq"
		bottomMsg := &vm.TranslationRsp{
			Page: vm.Page{},
		}
		bottomMsg.ID = sim.GetIDGenerator().Generate()
		bottomMsg.RspTo = sim.GetIDGenerator().Generate()
		bottomMsg.TrafficClass = "vm.TranslationRsp"

		controlPort.EXPECT().Send(gomock.Any()).Do(func(sent sim.Msg) {
			rsp := sent.(*mem.ControlRsp)
			Expect(rsp.Command).To(Equal(mem.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.Dst).To(Equal(sim.RemotePort("Requester")))
			Expect(rsp.Src).To(Equal(sim.RemotePort("ControlPort")))
		}).Return(nil)
		controlPort.EXPECT().RetrieveIncoming()

		topPort.EXPECT().PeekIncoming().Return(topMsg)
		topPort.EXPECT().RetrieveIncoming()
		topPort.EXPECT().PeekIncoming().Return(nil)

		bottomPort.EXPECT().PeekIncoming().Return(bottomMsg)
		bottomPort.EXPECT().RetrieveIncoming()
		bottomPort.EXPECT().PeekIncoming().Return(nil)

		madeProgress := ctrl.handleMMUCacheRestart(req)

		next := comp.GetNextState()
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStateEnable))
	})

	It("should accept flush request in enable state", func() {
		// Set state to enable (both base and next)
		spec := comp.GetSpec()
		comp.SetState(State{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		})

		req := &mem.ControlReq{Command: mem.CmdFlush}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = sim.RemotePort("Requester")
		req.TrafficClass = "mem.ControlReq"
		controlPort.EXPECT().RetrieveIncoming()

		madeProgress := ctrl.handleMMUCacheFlush(req)

		next := comp.GetNextState()
		Expect(madeProgress).To(BeTrue())
		Expect(next.InflightFlushReqActive).To(BeTrue())
		Expect(next.InflightFlushReqID).To(Equal(req.ID))
		Expect(next.InflightFlushReqSrc).To(Equal(req.Src))
		Expect(next.CurrentState).To(Equal(mmuCacheStateFlush))
	})

	It("should handle control pause", func() {
		// Set state to enable (both base and next)
		spec := comp.GetSpec()
		comp.SetState(State{
			CurrentState: mmuCacheStateEnable,
			Table:        initSets(spec.NumLevels, spec.NumBlocks),
		})

		msg := &mem.ControlReq{
			Command: mem.CmdPause,
		}
		msg.ID = sim.GetIDGenerator().Generate()
		msg.Dst = sim.RemotePort("ControlPort")
		msg.TrafficBytes = 4
		msg.TrafficClass = "mem.ControlReq"

		controlPort.EXPECT().PeekIncoming().Return(msg)
		controlPort.EXPECT().RetrieveIncoming().Return(msg)

		madeProgress := ctrl.handleIncomingCommands()

		next := comp.GetNextState()
		Expect(madeProgress).To(BeTrue())
		Expect(next.CurrentState).To(Equal(mmuCacheStatePause))
	})
})
